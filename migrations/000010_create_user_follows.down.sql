
-- Down migration file (e.g., 000010_create_user_follows.down.sql)

-- Drop trigger and function
DROP TRIGGER IF EXISTS update_follow_counts_trigger ON user_follows;
DROP FUNCTION IF EXISTS update_follow_counts();

-- Drop table
DROP TABLE IF EXISTS user_follows;

-- Remove columns from user_stats (optional - might want to keep data)
-- ALTER TABLE user_stats DROP COLUMN IF EXISTS followers_count;
-- ALTER TABLE user_stats DROP COLUMN IF EXISTS following_count;