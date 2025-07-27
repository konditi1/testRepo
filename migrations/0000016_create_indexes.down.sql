-- Drop all indexes
DROP INDEX IF EXISTS idx_sessions_user_expires;
DROP INDEX IF EXISTS idx_users_active_role;
DROP INDEX IF EXISTS idx_jobs_status_created_at;
DROP INDEX IF EXISTS idx_questions_status_created_at;
DROP INDEX IF EXISTS idx_posts_status_created_at;

-- Drop individual indexes
DROP INDEX IF EXISTS idx_comment_reactions_user_id;
DROP INDEX IF EXISTS idx_comment_reactions_comment_id;
DROP INDEX IF EXISTS idx_question_reactions_user_id;
DROP INDEX IF EXISTS idx_question_reactions_question_id;
DROP INDEX IF EXISTS idx_post_reactions_user_id;
DROP INDEX IF EXISTS idx_post_reactions_post_id;

DROP INDEX IF EXISTS idx_job_applications_applied_at;
DROP INDEX IF EXISTS idx_job_applications_status;
DROP INDEX IF EXISTS idx_job_applications_applicant_id;
DROP INDEX IF EXISTS idx_job_applications_job_id;

DROP INDEX IF EXISTS idx_jobs_tags;
DROP INDEX IF EXISTS idx_jobs_application_deadline;
DROP INDEX IF EXISTS idx_jobs_created_at;
DROP INDEX IF EXISTS idx_jobs_status;
DROP INDEX IF EXISTS idx_jobs_employment_type;
DROP INDEX IF EXISTS idx_jobs_employer_id;

DROP INDEX IF EXISTS idx_sessions_last_activity;
DROP INDEX IF EXISTS idx_sessions_expires_at;
DROP INDEX IF EXISTS idx_sessions_session_token;
DROP INDEX IF EXISTS idx_sessions_user_id;

DROP INDEX IF EXISTS idx_comments_created_at;
DROP INDEX IF EXISTS idx_comments_parent_comment_id;
DROP INDEX IF EXISTS idx_comments_question_id;
DROP INDEX IF EXISTS idx_comments_post_id;
DROP INDEX IF EXISTS idx_comments_user_id;

DROP INDEX IF EXISTS idx_questions_tags;
DROP INDEX IF EXISTS idx_questions_is_answered;
DROP INDEX IF EXISTS idx_questions_created_at;
DROP INDEX IF EXISTS idx_questions_status;
DROP INDEX IF EXISTS idx_questions_category;
DROP INDEX IF EXISTS idx_questions_user_id;

DROP INDEX IF EXISTS idx_posts_tags;
DROP INDEX IF EXISTS idx_posts_slug;
DROP INDEX IF EXISTS idx_posts_published_at;
DROP INDEX IF EXISTS idx_posts_created_at;
DROP INDEX IF EXISTS idx_posts_status;
DROP INDEX IF EXISTS idx_posts_category;
DROP INDEX IF EXISTS idx_posts_user_id;

DROP INDEX IF EXISTS idx_users_last_seen;
DROP INDEX IF EXISTS idx_users_created_at;
DROP INDEX IF EXISTS idx_users_expertise;
DROP INDEX IF EXISTS idx_users_is_online;
DROP INDEX IF EXISTS idx_users_is_active;
DROP INDEX IF EXISTS idx_users_role;
DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;
