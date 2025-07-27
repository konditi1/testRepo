
-- Down migration file: 000011_create_stats_tables.down.sql

-- Drop triggers
DROP TRIGGER IF EXISTS trigger_posts_user_stats ON posts;
DROP TRIGGER IF EXISTS trigger_questions_user_stats ON questions;
DROP TRIGGER IF EXISTS trigger_comments_user_stats ON comments;

-- Drop function
DROP FUNCTION IF EXISTS update_user_stats();

-- Drop tables
DROP TABLE IF EXISTS job_stats;
DROP TABLE IF EXISTS comment_stats;
DROP TABLE IF EXISTS question_stats;
DROP TABLE IF EXISTS post_stats;
DROP TABLE IF EXISTS user_stats;
