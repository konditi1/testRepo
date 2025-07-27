-- User role enum for role-based access control
DO $$ BEGIN
    CREATE TYPE user_role AS ENUM (
        'user',        -- Regular user
        'reviewer',    -- Can review content
        'moderator',   -- Can moderate discussions
        'admin'        -- Full admin access
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Reaction types for likes/dislikes
DO $$ BEGIN
    CREATE TYPE reaction_type AS ENUM (
        'like',
        'dislike'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Expertise levels for user profiles
DO $$ BEGIN
    CREATE TYPE expertise_level AS ENUM (
        'none',
        'beginner',
        'intermediate',
        'advanced',
        'expert'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Job status for job postings
DO $$ BEGIN
    CREATE TYPE job_status AS ENUM (
        'draft',       -- Being prepared
        'active',      -- Open for applications
        'paused',      -- Temporarily closed
        'closed',      -- No longer accepting applications
        'filled'       -- Position has been filled
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Employment types
DO $$ BEGIN
    CREATE TYPE employment_type AS ENUM (
        'full_time',
        'part_time',
        'contract',
        'temporary',
        'internship',
        'volunteer',
        'freelance'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Application status for job applications
DO $$ BEGIN
    CREATE TYPE application_status AS ENUM (
        'pending',      -- Submitted, not yet reviewed
        'reviewing',    -- Under review
        'shortlisted',  -- Selected for next stage
        'interviewed',  -- Interview completed
        'accepted',     -- Offer extended/accepted
        'rejected',     -- Application declined
        'withdrawn'     -- Applicant withdrew
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Content status for posts/questions
DO $$ BEGIN
    CREATE TYPE content_status AS ENUM (
        'draft',       -- Being written
        'published',   -- Live and visible
        'archived',    -- No longer active but preserved
        'deleted',     -- Soft deleted
        'flagged',     -- Flagged for review
        'approved',    -- Manually approved after review
        'rejected'     -- Rejected after review
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Notification types for system notifications
DO $$ BEGIN
    CREATE TYPE notification_type AS ENUM (
        'new_post',
        'new_question',
        'post_comment',
        'question_comment',
        'comment_reply',
        'post_like',
        'question_like',
        'comment_like',
        'chat_message',
        'job_posted',
        'job_application',
        'job_status_update',
        'announcement',
        'system_update',
        'security_alert'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Comment: All enum types created successfully
COMMENT ON TYPE user_role IS 'User roles for authorization';
COMMENT ON TYPE reaction_type IS 'Types of reactions (like/dislike)';
COMMENT ON TYPE expertise_level IS 'User expertise levels';
COMMENT ON TYPE job_status IS 'Job posting statuses';
COMMENT ON TYPE employment_type IS 'Employment type classifications';
COMMENT ON TYPE application_status IS 'Job application statuses';
COMMENT ON TYPE content_status IS 'Content publication statuses';
COMMENT ON TYPE notification_type IS 'System notification types';
