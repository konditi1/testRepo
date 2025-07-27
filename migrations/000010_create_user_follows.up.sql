-- 000010_create_user_follows.up.sql

-- User follows table for social relationships
CREATE TABLE IF NOT EXISTS user_follows (
    id BIGSERIAL PRIMARY KEY,
    follower_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    -- Constraints
    CONSTRAINT user_follows_no_self_follow CHECK (follower_id != followee_id),
    CONSTRAINT user_follows_unique UNIQUE (follower_id, followee_id)
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_user_follows_follower_id ON user_follows(follower_id);
CREATE INDEX IF NOT EXISTS idx_user_follows_followee_id ON user_follows(followee_id);
CREATE INDEX IF NOT EXISTS idx_user_follows_created_at ON user_follows(created_at);

-- Ensure follower_count and following_count columns exist in user_stats
DO $$ 
BEGIN
    -- Check if followers_count column exists, if not add it
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_stats' 
        AND column_name = 'followers_count'
    ) THEN
        ALTER TABLE user_stats ADD COLUMN followers_count INTEGER DEFAULT 0;
    END IF;
    
    -- Check if following_count column exists, if not add it
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'user_stats' 
        AND column_name = 'following_count'
    ) THEN
        ALTER TABLE user_stats ADD COLUMN following_count INTEGER DEFAULT 0;
    END IF;
END $$;

-- Function to update follow counts
CREATE OR REPLACE FUNCTION update_follow_counts()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        -- Update follower count for followee
        UPDATE user_stats 
        SET followers_count = (
            SELECT COUNT(*) FROM user_follows WHERE followee_id = NEW.followee_id
        )
        WHERE user_id = NEW.followee_id;
        
        -- Update following count for follower
        UPDATE user_stats 
        SET following_count = (
            SELECT COUNT(*) FROM user_follows WHERE follower_id = NEW.follower_id
        )
        WHERE user_id = NEW.follower_id;
        
        RETURN NEW;
    ELSIF TG_OP = 'DELETE' THEN
        -- Update follower count for followee
        UPDATE user_stats 
        SET followers_count = (
            SELECT COUNT(*) FROM user_follows WHERE followee_id = OLD.followee_id
        )
        WHERE user_id = OLD.followee_id;
        
        -- Update following count for follower
        UPDATE user_stats 
        SET following_count = (
            SELECT COUNT(*) FROM user_follows WHERE follower_id = OLD.follower_id
        )
        WHERE user_id = OLD.follower_id;
        
        RETURN OLD;
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Trigger to automatically update follow counts
DROP TRIGGER IF EXISTS update_follow_counts_trigger ON user_follows;
CREATE TRIGGER update_follow_counts_trigger
    AFTER INSERT OR DELETE ON user_follows
    FOR EACH ROW EXECUTE FUNCTION update_follow_counts();

-- =======================================
-- NOW UPDATE USER STATS WITH FOLLOW COUNTS
-- (Moved from migration 9 since user_follows table now exists)
-- =======================================

-- Initialize follower/following counts for existing users AND complete the user_stats calculations
UPDATE user_stats SET 
    followers_count = (
        SELECT COUNT(*) FROM user_follows 
        WHERE followee_id = user_stats.user_id
    ),
    following_count = (
        SELECT COUNT(*) FROM user_follows 
        WHERE follower_id = user_stats.user_id
    ),
    -- Recalculate total_contributions now that we have all the data
    total_contributions = (
        posts_count + questions_count + comments_count
    ),
    updated_at = CURRENT_TIMESTAMP;

-- Table comments
COMMENT ON TABLE user_follows IS 'User follow relationships for social features';
COMMENT ON COLUMN user_follows.follower_id IS 'User who is following';
COMMENT ON COLUMN user_follows.followee_id IS 'User being followed';
