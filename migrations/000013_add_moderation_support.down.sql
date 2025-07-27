-- Drop indexes
DROP INDEX IF EXISTS idx_moderation_logs_post_id;
DROP INDEX IF EXISTS idx_moderation_logs_moderator_id;
DROP INDEX IF EXISTS idx_moderation_logs_created_at;

-- Drop moderation_logs table
DROP TABLE IF EXISTS moderation_logs;

-- Remove moderation columns from posts table
ALTER TABLE posts 
    DROP COLUMN IF EXISTS moderated_at,
    DROP COLUMN IF EXISTS moderated_by,
    DROP COLUMN IF EXISTS moderation_reason;
