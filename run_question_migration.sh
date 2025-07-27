-- Rollback question support migration

-- Drop question_reactions table
DROP TABLE IF EXISTS question_reactions;

-- Remove question_id column from comments table
ALTER TABLE comments DROP COLUMN IF EXISTS question_id;

-- Make post_id required again in comments table
UPDATE comments SET post_id = 0 WHERE post_id IS NULL;
ALTER TABLE comments ALTER COLUMN post_id SET NOT NULL;

-- Drop check constraint
ALTER TABLE comments DROP CONSTRAINT IF EXISTS check_post_or_question;

-- Drop questions table
DROP TABLE IF EXISTS questions;