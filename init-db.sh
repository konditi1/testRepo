#!/bin/bash
set -e

echo "Starting database migration for Heroku deployment..."

# Check if DATABASE_URL is set
if [ -z "$DATABASE_URL" ]; then
    echo "DATABASE_URL is not set"
    exit 1
fi

echo "Running comprehensive database migration..."

psql $DATABASE_URL << 'EOF'

-- Create all enum types first
DO $$ BEGIN
    CREATE TYPE user_role AS ENUM ('admin', 'moderator', 'reviewer', 'user');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE reaction_type AS ENUM ('like', 'dislike');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE expertise_level AS ENUM ('none', 'beginner', 'intermediate', 'advanced');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE job_status AS ENUM ('active', 'closed', 'paused');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE employment_type AS ENUM ('full_time', 'part_time', 'contract', 'internship', 'volunteer');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE application_status AS ENUM ('pending', 'reviewed', 'shortlisted', 'interviewed', 'accepted', 'rejected');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE notification_type AS ENUM (
        'new_post', 
        'new_question', 
        'post_comment', 
        'question_comment', 
        'post_like', 
        'question_like', 
        'comment_like',
        'chat_message', 
        'job_posted', 
        'job_application',
        'announcement'
    );
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Create users table (foundation table)
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    email TEXT UNIQUE NOT NULL,
    username TEXT UNIQUE NOT NULL,
    first_name VARCHAR(255) DEFAULT NULL,
    last_name VARCHAR(255) DEFAULT NULL,
    profile_url TEXT DEFAULT NULL,
    profile_public_id TEXT DEFAULT NULL,
    affiliation TEXT DEFAULT NULL,
    bio TEXT DEFAULT NULL,
    years_experience INTEGER DEFAULT 0,
    cv_url TEXT DEFAULT NULL,
    cv_public_id TEXT DEFAULT NULL,
    core_competencies TEXT DEFAULT NULL,
    expertise expertise_level DEFAULT 'none',
    role user_role DEFAULT 'user',
    password VARCHAR(255) DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_online BOOLEAN DEFAULT FALSE,
    last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Add last_seen column if it doesn't exist (for existing deployments)
DO $$ 
BEGIN
    ALTER TABLE users ADD COLUMN IF NOT EXISTS last_seen TIMESTAMP DEFAULT CURRENT_TIMESTAMP;
EXCEPTION
    WHEN duplicate_column THEN NULL;
END $$;

-- Create all other tables (in dependency order)
CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    name TEXT UNIQUE NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    session_token TEXT UNIQUE NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    last_activity TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS posts (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    category TEXT NOT NULL,
    image_url TEXT DEFAULT NULL,
    image_public_id TEXT DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS questions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    content TEXT,
    category TEXT NOT NULL,
    file_url TEXT DEFAULT NULL,
    file_public_id TEXT DEFAULT NULL,
    target_group TEXT DEFAULT 'All',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Fix comments table to support both posts and questions
DO $$ 
BEGIN
    -- Add question_id column if it doesn't exist
    ALTER TABLE comments ADD COLUMN IF NOT EXISTS question_id INTEGER DEFAULT NULL;
    
    -- Make post_id nullable
    ALTER TABLE comments ALTER COLUMN post_id DROP NOT NULL;
EXCEPTION
    WHEN others THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS comments (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    post_id INTEGER DEFAULT NULL,
    question_id INTEGER DEFAULT NULL,
    username TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE
);

-- Add constraints for comments
DO $$ 
BEGIN
    -- Add foreign key for question_id if it doesn't exist
    ALTER TABLE comments ADD CONSTRAINT fk_comments_question_id 
        FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE;
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ 
BEGIN
    -- Add check constraint
    ALTER TABLE comments DROP CONSTRAINT IF EXISTS check_post_or_question;
    ALTER TABLE comments ADD CONSTRAINT check_post_or_question 
        CHECK ((post_id IS NOT NULL) != (question_id IS NOT NULL));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

-- Create remaining tables
CREATE TABLE IF NOT EXISTS post_reactions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    post_id INTEGER NOT NULL,
    reaction reaction_type NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (post_id) REFERENCES posts(id) ON DELETE CASCADE,
    UNIQUE (user_id, post_id)
);

CREATE TABLE IF NOT EXISTS question_reactions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    question_id INTEGER NOT NULL,
    reaction reaction_type NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (question_id) REFERENCES questions(id) ON DELETE CASCADE,
    UNIQUE (user_id, question_id)
);

CREATE TABLE IF NOT EXISTS comment_reactions (
    user_id INTEGER NOT NULL,
    comment_id INTEGER NOT NULL,
    reaction reaction_type NOT NULL,
    PRIMARY KEY (user_id, comment_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (comment_id) REFERENCES comments(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS messages (
    id SERIAL PRIMARY KEY,
    sender_id INTEGER NOT NULL,
    recipient_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    is_read BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (sender_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (recipient_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS jobs (
    id SERIAL PRIMARY KEY,
    employer_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    description TEXT NOT NULL,
    requirements TEXT,
    responsibilities TEXT,
    location TEXT,
    employment_type employment_type DEFAULT 'full_time',
    salary_range TEXT,
    application_deadline DATE,
    status job_status DEFAULT 'active',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (employer_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS job_applications (
    id SERIAL PRIMARY KEY,
    job_id INTEGER NOT NULL,
    applicant_id INTEGER NOT NULL,
    cover_letter TEXT NOT NULL,
    application_letter_url TEXT DEFAULT NULL,
    application_letter_public_id TEXT DEFAULT NULL,
    status application_status DEFAULT 'pending',
    applied_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    reviewed_at TIMESTAMP DEFAULT NULL,
    notes TEXT DEFAULT NULL,
    FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE CASCADE,
    FOREIGN KEY (applicant_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE (job_id, applicant_id)
);

-- Add all other tables from your schema...
CREATE TABLE IF NOT EXISTS notification_preferences (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE,
    new_posts BOOLEAN DEFAULT TRUE,
    new_questions BOOLEAN DEFAULT TRUE,
    comments_on_my_posts BOOLEAN DEFAULT TRUE,
    comments_on_my_questions BOOLEAN DEFAULT TRUE,
    likes_on_my_content BOOLEAN DEFAULT TRUE,
    chat_messages BOOLEAN DEFAULT TRUE,
    job_postings BOOLEAN DEFAULT TRUE,
    job_applications BOOLEAN DEFAULT TRUE,
    announcements BOOLEAN DEFAULT TRUE,
    email_notifications BOOLEAN DEFAULT FALSE,
    push_notifications BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS notifications (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    type notification_type NOT NULL,
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    read BOOLEAN DEFAULT FALSE,
    entity_id INTEGER DEFAULT NULL,
    entity_type TEXT DEFAULT NULL,
    actor_id INTEGER DEFAULT NULL,
    actor_username TEXT DEFAULT NULL,
    actor_profile_url TEXT DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS user_stats (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL UNIQUE,
    reputation_points INTEGER DEFAULT 0,
    posts_count INTEGER DEFAULT 0,
    questions_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    likes_given INTEGER DEFAULT 0,
    likes_received INTEGER DEFAULT 0,
    total_contributions INTEGER DEFAULT 0,
    last_activity TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS badges (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NOT NULL,
    icon VARCHAR(50) NOT NULL,
    color VARCHAR(20) DEFAULT '#3b82f6',
    criteria_type VARCHAR(50) NOT NULL,
    criteria_value INTEGER NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS user_badges (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    badge_id INTEGER NOT NULL,
    earned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (badge_id) REFERENCES badges(id) ON DELETE CASCADE,
    UNIQUE(user_id, badge_id)
);

CREATE TABLE IF NOT EXISTS documents (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    title TEXT NOT NULL,
    description TEXT,
    file_url TEXT NOT NULL,
    file_public_id TEXT NOT NULL,
    file_type TEXT NOT NULL,
    version INTEGER DEFAULT 1,
    tags TEXT,
    is_public BOOLEAN DEFAULT TRUE,
    download_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS document_comments (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    document_id INTEGER NOT NULL,
    username TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS document_reactions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL,
    document_id INTEGER NOT NULL,
    reaction reaction_type NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (document_id) REFERENCES documents(id) ON DELETE CASCADE,
    UNIQUE (user_id, document_id)
);

-- Create all indexes
CREATE INDEX IF NOT EXISTS idx_jobs_employer_id ON jobs(employer_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_job_applications_job_id ON job_applications(job_id);
CREATE INDEX IF NOT EXISTS idx_job_applications_applicant_id ON job_applications(applicant_id);
CREATE INDEX IF NOT EXISTS idx_job_applications_status ON job_applications(status);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notifications_created_at ON notifications(created_at);
CREATE INDEX IF NOT EXISTS idx_notification_preferences_user_id ON notification_preferences(user_id);

CREATE INDEX IF NOT EXISTS idx_user_stats_user_id ON user_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_user_stats_reputation ON user_stats(reputation_points DESC);
CREATE INDEX IF NOT EXISTS idx_user_stats_total_contributions ON user_stats(total_contributions DESC);
CREATE INDEX IF NOT EXISTS idx_user_badges_user_id ON user_badges(user_id);
CREATE INDEX IF NOT EXISTS idx_badges_criteria ON badges(criteria_type, criteria_value);

CREATE INDEX IF NOT EXISTS idx_documents_user_id ON documents(user_id);
CREATE INDEX IF NOT EXISTS idx_documents_title ON documents(title);
CREATE INDEX IF NOT EXISTS idx_documents_tags ON documents(tags);
CREATE INDEX IF NOT EXISTS idx_document_reactions_document_id ON document_reactions(document_id);
CREATE INDEX IF NOT EXISTS idx_document_comments_document_id ON document_comments(document_id);

-- Seed initial data
INSERT INTO categories (name)
VALUES 
    ('M&E in Science'),
    ('M&E in Health'),
    ('M&E in Education'),
    ('M&E in Social Sciences'),
    ('M&E in Climate Change'),
    ('M&E in Agriculture')
ON CONFLICT (name) DO NOTHING;

INSERT INTO badges (name, description, icon, color, criteria_type, criteria_value) VALUES
('First Post', 'Published your first post', 'fa-solid fa-seedling', '#10b981', 'posts', 1),
('Active Poster', 'Published 10 posts', 'fa-solid fa-fire', '#f59e0b', 'posts', 10),
('Prolific Writer', 'Published 50 posts', 'fa-solid fa-trophy', '#ef4444', 'posts', 50),
('Question Asker', 'Asked your first question', 'fa-solid fa-circle-question', '#8b5cf6', 'questions', 1),
('Curious Mind', 'Asked 10 questions', 'fa-solid fa-lightbulb', '#06b6d4', 'questions', 10),
('Community Helper', 'Received 10 likes', 'fa-solid fa-heart', '#ec4899', 'likes_received', 10),
('Popular Contributor', 'Received 50 likes', 'fa-solid fa-star', '#f59e0b', 'likes_received', 50),
('Conversation Starter', 'Made 25 comments', 'fa-solid fa-comments', '#6366f1', 'comments', 25),
('Rising Star', 'Earned 100 reputation points', 'fa-solid fa-rocket', '#8b5cf6', 'reputation', 100),
('Expert Evaluator', 'Earned 500 reputation points', 'fa-solid fa-medal', '#eab308', 'reputation', 500)
ON CONFLICT (name) DO NOTHING;

-- Initialize user stats for existing users
INSERT INTO user_stats (user_id, reputation_points, posts_count, questions_count, comments_count, total_contributions)
SELECT 
    u.id,
    (COALESCE(p.posts_count, 0) * 10) + 
    (COALESCE(q.questions_count, 0) * 5) + 
    (COALESCE(c.comments_count, 0) * 2) + 
    (COALESCE(likes.likes_received, 0) * 3) as reputation_points,
    COALESCE(p.posts_count, 0),
    COALESCE(q.questions_count, 0),
    COALESCE(c.comments_count, 0),
    COALESCE(p.posts_count, 0) + COALESCE(q.questions_count, 0) + COALESCE(c.comments_count, 0)
FROM users u
LEFT JOIN (SELECT user_id, COUNT(*) as posts_count FROM posts GROUP BY user_id) p ON u.id = p.user_id
LEFT JOIN (SELECT user_id, COUNT(*) as questions_count FROM questions GROUP BY user_id) q ON u.id = q.user_id
LEFT JOIN (SELECT user_id, COUNT(*) as comments_count FROM comments GROUP BY user_id) c ON u.id = c.user_id
LEFT JOIN (
    SELECT 
        posts.user_id,
        COUNT(*) as likes_received
    FROM post_reactions pr
    JOIN posts ON pr.post_id = posts.id
    WHERE pr.reaction = 'like'
    GROUP BY posts.user_id
    UNION ALL
    SELECT 
        questions.user_id,
        COUNT(*) as likes_received
    FROM question_reactions qr
    JOIN questions ON qr.question_id = questions.id
    WHERE qr.reaction = 'like'
    GROUP BY questions.user_id
) likes ON u.id = likes.user_id
WHERE NOT EXISTS (SELECT 1 FROM user_stats WHERE user_id = u.id);

EOF

echo "Database migration completed successfully!"
echo "All tables, indexes, and initial data have been created/updated."