-- 000009_create_stats_tables.up.sql
-- User statistics table (CRITICAL - referenced in user_repository.go)
CREATE TABLE IF NOT EXISTS user_stats (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    
    -- Reputation and engagement
    reputation_points INTEGER DEFAULT 0 NOT NULL,
    total_contributions INTEGER DEFAULT 0 NOT NULL,
    
    -- Content counts
    posts_count INTEGER DEFAULT 0 NOT NULL,
    questions_count INTEGER DEFAULT 0 NOT NULL,
    comments_count INTEGER DEFAULT 0 NOT NULL,
    
    -- Social engagement
    likes_given INTEGER DEFAULT 0 NOT NULL,
    likes_received INTEGER DEFAULT 0 NOT NULL,
    followers_count INTEGER DEFAULT 0 NOT NULL,
    following_count INTEGER DEFAULT 0 NOT NULL,
    
    -- Activity tracking
    last_activity TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Post statistics table (for analytics)
CREATE TABLE IF NOT EXISTS post_stats (
    post_id BIGINT PRIMARY KEY REFERENCES posts(id) ON DELETE CASCADE,
    views_count INTEGER DEFAULT 0 NOT NULL,
    likes_count INTEGER DEFAULT 0 NOT NULL,
    dislikes_count INTEGER DEFAULT 0 NOT NULL,
    comments_count INTEGER DEFAULT 0 NOT NULL,
    shares_count INTEGER DEFAULT 0 NOT NULL,
    bookmarks_count INTEGER DEFAULT 0 NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Question statistics table (for analytics)
CREATE TABLE IF NOT EXISTS question_stats (
    question_id BIGINT PRIMARY KEY REFERENCES questions(id) ON DELETE CASCADE,
    views_count INTEGER DEFAULT 0 NOT NULL,
    likes_count INTEGER DEFAULT 0 NOT NULL,
    dislikes_count INTEGER DEFAULT 0 NOT NULL,
    comments_count INTEGER DEFAULT 0 NOT NULL,
    is_answered BOOLEAN DEFAULT FALSE NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Comment statistics table (for analytics)
CREATE TABLE IF NOT EXISTS comment_stats (
    comment_id BIGINT PRIMARY KEY REFERENCES comments(id) ON DELETE CASCADE,
    likes_count INTEGER DEFAULT 0 NOT NULL,
    dislikes_count INTEGER DEFAULT 0 NOT NULL,
    replies_count INTEGER DEFAULT 0 NOT NULL,
    is_accepted BOOLEAN DEFAULT FALSE NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Job statistics table (for analytics)
CREATE TABLE IF NOT EXISTS job_stats (
    job_id BIGINT PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    views_count INTEGER DEFAULT 0 NOT NULL,
    applications_count INTEGER DEFAULT 0 NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- =======================================
-- INITIALIZE STATS FOR EXISTING USERS
-- =======================================

-- Create user_stats entries for all existing users
INSERT INTO user_stats (user_id, created_at, updated_at)
SELECT id, created_at, updated_at 
FROM users 
ON CONFLICT (user_id) DO NOTHING;

-- Initialize post stats from existing posts
INSERT INTO post_stats (post_id, views_count, likes_count, dislikes_count, comments_count)
SELECT 
    id, 
    COALESCE(views_count, 0),
    COALESCE(likes_count, 0), 
    COALESCE(dislikes_count, 0),
    COALESCE(comments_count, 0)
FROM posts
ON CONFLICT (post_id) DO UPDATE SET
    views_count = EXCLUDED.views_count,
    likes_count = EXCLUDED.likes_count,
    dislikes_count = EXCLUDED.dislikes_count,
    comments_count = EXCLUDED.comments_count;

-- Initialize question stats from existing questions
INSERT INTO question_stats (question_id, views_count, likes_count, dislikes_count, comments_count, is_answered)
SELECT 
    id,
    COALESCE(views_count, 0),
    COALESCE(likes_count, 0),
    COALESCE(dislikes_count, 0), 
    COALESCE(comments_count, 0),
    COALESCE(is_answered, FALSE)
FROM questions
ON CONFLICT (question_id) DO UPDATE SET
    views_count = EXCLUDED.views_count,
    likes_count = EXCLUDED.likes_count,
    dislikes_count = EXCLUDED.dislikes_count,
    comments_count = EXCLUDED.comments_count,
    is_answered = EXCLUDED.is_answered;

-- Initialize comment stats from existing comments
INSERT INTO comment_stats (comment_id, likes_count, dislikes_count, replies_count)
SELECT 
    c.id,
    COALESCE(c.likes_count, 0),
    COALESCE(c.dislikes_count, 0),
    COALESCE(reply_counts.replies_count, 0)
FROM comments c
LEFT JOIN (
    SELECT parent_comment_id, COUNT(*) as replies_count
    FROM comments 
    WHERE parent_comment_id IS NOT NULL
    GROUP BY parent_comment_id
) reply_counts ON c.id = reply_counts.parent_comment_id
ON CONFLICT (comment_id) DO UPDATE SET
    likes_count = EXCLUDED.likes_count,
    dislikes_count = EXCLUDED.dislikes_count,
    replies_count = EXCLUDED.replies_count;

-- Initialize job stats from existing jobs
INSERT INTO job_stats (job_id, views_count, applications_count)
SELECT 
    id,
    COALESCE(views_count, 0),
    COALESCE(applications_count, 0)
FROM jobs
ON CONFLICT (job_id) DO UPDATE SET
    views_count = EXCLUDED.views_count,
    applications_count = EXCLUDED.applications_count;

-- =======================================
-- UPDATE USER STATS WITH ACTUAL COUNTS
-- (Excluding followers/following - handled in migration 10)
-- =======================================

-- Calculate and update user statistics
UPDATE user_stats SET
    posts_count = (
        SELECT COUNT(*) FROM posts 
        WHERE user_id = user_stats.user_id AND status = 'published'
    ),
    questions_count = (
        SELECT COUNT(*) FROM questions 
        WHERE user_id = user_stats.user_id AND status = 'published'
    ),
    comments_count = (
        SELECT COUNT(*) FROM comments 
        WHERE user_id = user_stats.user_id
    ),
    likes_given = (
        SELECT COUNT(*) FROM (
            SELECT user_id FROM post_reactions WHERE user_id = user_stats.user_id AND reaction = 'like'
            UNION ALL
            SELECT user_id FROM question_reactions WHERE user_id = user_stats.user_id AND reaction = 'like'
            UNION ALL  
            SELECT user_id FROM comment_reactions WHERE user_id = user_stats.user_id AND reaction = 'like'
        ) all_likes
    ),
    likes_received = (
        SELECT COUNT(*) FROM (
            SELECT 1 FROM post_reactions pr 
            JOIN posts p ON pr.post_id = p.id 
            WHERE p.user_id = user_stats.user_id AND pr.reaction = 'like'
            UNION ALL
            SELECT 1 FROM question_reactions qr 
            JOIN questions q ON qr.question_id = q.id 
            WHERE q.user_id = user_stats.user_id AND qr.reaction = 'like'
            UNION ALL
            SELECT 1 FROM comment_reactions cr 
            JOIN comments c ON cr.comment_id = c.id 
            WHERE c.user_id = user_stats.user_id AND cr.reaction = 'like'
        ) all_received_likes
    ),
    total_contributions = (
        posts_count + questions_count + comments_count
    );

-- =======================================
-- TRIGGERS TO MAINTAIN STATS
-- =======================================

-- Function to update user stats when content is created/updated
CREATE OR REPLACE FUNCTION update_user_stats()
RETURNS TRIGGER AS $$
BEGIN
    -- Update user stats based on the table that changed
    IF TG_TABLE_NAME = 'posts' THEN
        UPDATE user_stats SET 
            posts_count = (SELECT COUNT(*) FROM posts WHERE user_id = NEW.user_id AND status = 'published'),
            total_contributions = posts_count + questions_count + comments_count,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = NEW.user_id;
        
    ELSIF TG_TABLE_NAME = 'questions' THEN
        UPDATE user_stats SET 
            questions_count = (SELECT COUNT(*) FROM questions WHERE user_id = NEW.user_id AND status = 'published'),
            total_contributions = posts_count + questions_count + comments_count,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = NEW.user_id;
        
    ELSIF TG_TABLE_NAME = 'comments' THEN
        UPDATE user_stats SET 
            comments_count = (SELECT COUNT(*) FROM comments WHERE user_id = NEW.user_id),
            total_contributions = posts_count + questions_count + comments_count,
            updated_at = CURRENT_TIMESTAMP
        WHERE user_id = NEW.user_id;
    END IF;
    
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply triggers to maintain user stats
CREATE TRIGGER trigger_posts_user_stats 
    AFTER INSERT OR UPDATE ON posts 
    FOR EACH ROW EXECUTE FUNCTION update_user_stats();

CREATE TRIGGER trigger_questions_user_stats 
    AFTER INSERT OR UPDATE ON questions 
    FOR EACH ROW EXECUTE FUNCTION update_user_stats();

CREATE TRIGGER trigger_comments_user_stats 
    AFTER INSERT OR UPDATE ON comments 
    FOR EACH ROW EXECUTE FUNCTION update_user_stats();

-- Comments
COMMENT ON TABLE user_stats IS 'User engagement and contribution statistics';
COMMENT ON TABLE post_stats IS 'Post performance statistics';
COMMENT ON TABLE question_stats IS 'Question performance statistics';
COMMENT ON TABLE comment_stats IS 'Comment engagement statistics';
COMMENT ON TABLE job_stats IS 'Job posting statistics';
