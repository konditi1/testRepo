-- Drop triggers in reverse order
DROP TRIGGER IF EXISTS trigger_job_applications_count ON job_applications;
DROP TRIGGER IF EXISTS trigger_comments_count ON comments;
DROP TRIGGER IF EXISTS trigger_comment_reactions_count ON comment_reactions;
DROP TRIGGER IF EXISTS trigger_question_reactions_count ON question_reactions;
DROP TRIGGER IF EXISTS trigger_post_reactions_count ON post_reactions;
DROP TRIGGER IF EXISTS trigger_users_last_seen ON users;
DROP TRIGGER IF EXISTS trigger_notifications_updated_at ON notifications;
DROP TRIGGER IF EXISTS trigger_messages_updated_at ON messages;
DROP TRIGGER IF EXISTS trigger_job_applications_updated_at ON job_applications;
DROP TRIGGER IF EXISTS trigger_jobs_updated_at ON jobs;
DROP TRIGGER IF EXISTS trigger_comments_updated_at ON comments;
DROP TRIGGER IF EXISTS trigger_questions_updated_at ON questions;
DROP TRIGGER IF EXISTS trigger_posts_updated_at ON posts;
DROP TRIGGER IF EXISTS trigger_users_updated_at ON users;

-- Drop functions
DROP FUNCTION IF EXISTS update_job_application_counts();
DROP FUNCTION IF EXISTS update_comment_counts();
DROP FUNCTION IF EXISTS update_engagement_counts();
DROP FUNCTION IF EXISTS update_last_seen();
DROP FUNCTION IF EXISTS update_updated_at_column();
