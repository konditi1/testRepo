-- =======================================
-- DATABASE TRIGGERS (Automated Updates)
-- =======================================

-- Function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Function to update last_seen when user comes online
CREATE OR REPLACE FUNCTION update_last_seen()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.is_online = TRUE AND OLD.is_online = FALSE THEN
        NEW.last_seen = CURRENT_TIMESTAMP;
    END IF;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Function to update engagement counts
CREATE OR REPLACE FUNCTION update_engagement_counts()
RETURNS TRIGGER AS $$
BEGIN
    -- Update post reaction counts
    IF TG_TABLE_NAME = 'post_reactions' THEN
        UPDATE posts SET 
            likes_count = (SELECT COUNT(*) FROM post_reactions WHERE post_id = NEW.post_id AND reaction = 'like'),
            dislikes_count = (SELECT COUNT(*) FROM post_reactions WHERE post_id = NEW.post_id AND reaction = 'dislike')
        WHERE id = NEW.post_id;
    
    -- Update question reaction counts
    ELSIF TG_TABLE_NAME = 'question_reactions' THEN
        UPDATE questions SET 
            likes_count = (SELECT COUNT(*) FROM question_reactions WHERE question_id = NEW.question_id AND reaction = 'like'),
            dislikes_count = (SELECT COUNT(*) FROM question_reactions WHERE question_id = NEW.question_id AND reaction = 'dislike')
        WHERE id = NEW.question_id;
    
    -- Update comment reaction counts
    ELSIF TG_TABLE_NAME = 'comment_reactions' THEN
        UPDATE comments SET 
            likes_count = (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = NEW.comment_id AND reaction = 'like'),
            dislikes_count = (SELECT COUNT(*) FROM comment_reactions WHERE comment_id = NEW.comment_id AND reaction = 'dislike')
        WHERE id = NEW.comment_id;
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Function to update comment counts
CREATE OR REPLACE FUNCTION update_comment_counts()
RETURNS TRIGGER AS $$
BEGIN
    -- Update post comment count
    IF NEW.post_id IS NOT NULL THEN
        UPDATE posts SET 
            comments_count = (SELECT COUNT(*) FROM comments WHERE post_id = NEW.post_id AND is_approved = true)
        WHERE id = NEW.post_id;
    END IF;
    
    -- Update question comment count
    IF NEW.question_id IS NOT NULL THEN
        UPDATE questions SET 
            comments_count = (SELECT COUNT(*) FROM comments WHERE question_id = NEW.question_id AND is_approved = true)
        WHERE id = NEW.question_id;
    END IF;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Function to update job application counts
CREATE OR REPLACE FUNCTION update_job_application_counts()
RETURNS TRIGGER AS $$
BEGIN
    UPDATE jobs SET 
        applications_count = (SELECT COUNT(*) FROM job_applications WHERE job_id = NEW.job_id)
    WHERE id = NEW.job_id;
    
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply updated_at triggers
CREATE TRIGGER trigger_users_updated_at 
    BEFORE UPDATE ON users 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_posts_updated_at 
    BEFORE UPDATE ON posts 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_questions_updated_at 
    BEFORE UPDATE ON questions 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_comments_updated_at 
    BEFORE UPDATE ON comments 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_jobs_updated_at 
    BEFORE UPDATE ON jobs 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_job_applications_updated_at 
    BEFORE UPDATE ON job_applications 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_messages_updated_at 
    BEFORE UPDATE ON messages 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trigger_notifications_updated_at 
    BEFORE UPDATE ON notifications 
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Apply last_seen trigger
CREATE TRIGGER trigger_users_last_seen 
    BEFORE UPDATE ON users 
    FOR EACH ROW EXECUTE FUNCTION update_last_seen();

-- Apply engagement count triggers
CREATE TRIGGER trigger_post_reactions_count 
    AFTER INSERT OR UPDATE OR DELETE ON post_reactions 
    FOR EACH ROW EXECUTE FUNCTION update_engagement_counts();

CREATE TRIGGER trigger_question_reactions_count 
    AFTER INSERT OR UPDATE OR DELETE ON question_reactions 
    FOR EACH ROW EXECUTE FUNCTION update_engagement_counts();

CREATE TRIGGER trigger_comment_reactions_count 
    AFTER INSERT OR UPDATE OR DELETE ON comment_reactions 
    FOR EACH ROW EXECUTE FUNCTION update_engagement_counts();

-- Apply comment count triggers
CREATE TRIGGER trigger_comments_count 
    AFTER INSERT OR UPDATE OR DELETE ON comments 
    FOR EACH ROW EXECUTE FUNCTION update_comment_counts();

-- Apply job application count triggers
CREATE TRIGGER trigger_job_applications_count 
    AFTER INSERT OR UPDATE OR DELETE ON job_applications 
    FOR EACH ROW EXECUTE FUNCTION update_job_application_counts();

-- Comments for documentation
COMMENT ON FUNCTION update_updated_at_column() IS 'Automatically updates updated_at timestamp on row changes';
COMMENT ON FUNCTION update_last_seen() IS 'Updates last_seen when user comes online';
COMMENT ON FUNCTION update_engagement_counts() IS 'Maintains engagement counts for posts, questions, and comments';
COMMENT ON FUNCTION update_comment_counts() IS 'Maintains comment counts for posts and questions';
COMMENT ON FUNCTION update_job_application_counts() IS 'Maintains application counts for jobs';