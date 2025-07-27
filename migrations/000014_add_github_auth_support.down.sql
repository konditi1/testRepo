-- Remove the index first
DROP INDEX IF EXISTS idx_users_github_id;

-- Remove the github_id column
ALTER TABLE users 
DROP COLUMN IF EXISTS github_id;
