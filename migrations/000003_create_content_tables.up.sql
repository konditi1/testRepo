-- =======================================
-- CONTENT TABLES (Posts, Questions, Comments)
-- =======================================

-- Posts table (enhanced)
CREATE TABLE IF NOT EXISTS posts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    category VARCHAR(100) NOT NULL,
    status content_status DEFAULT 'draft' NOT NULL,
    
    -- Media
    image_url TEXT,
    image_public_id VARCHAR(255),
    
    -- Engagement tracking
    views_count INTEGER DEFAULT 0,
    likes_count INTEGER DEFAULT 0,
    dislikes_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    
    -- SEO and metadata
    slug VARCHAR(255) UNIQUE,
    meta_description TEXT,
    tags TEXT[],
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    published_at TIMESTAMPTZ
);

-- Questions table (enhanced)
CREATE TABLE IF NOT EXISTS questions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    content TEXT,
    category VARCHAR(100) NOT NULL,
    target_group VARCHAR(100) DEFAULT 'All',
    status content_status DEFAULT 'draft' NOT NULL,
    
    -- Attachments
    file_url TEXT,
    file_public_id VARCHAR(255),
    
    -- Engagement tracking
    views_count INTEGER DEFAULT 0,
    likes_count INTEGER DEFAULT 0,
    dislikes_count INTEGER DEFAULT 0,
    comments_count INTEGER DEFAULT 0,
    
    -- Question-specific fields
    is_answered BOOLEAN DEFAULT FALSE,
    accepted_answer_id BIGINT,
    
    -- SEO and metadata
    slug VARCHAR(255) UNIQUE,
    tags TEXT[],
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    published_at TIMESTAMPTZ
);

-- Comments table (unified for posts/questions/documents)
CREATE TABLE IF NOT EXISTS comments (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    
    -- Parent references (exactly one must be set)
    post_id BIGINT REFERENCES posts(id) ON DELETE CASCADE,
    question_id BIGINT REFERENCES questions(id) ON DELETE CASCADE,
    document_id BIGINT, -- Will be added when document system is implemented
    
    -- Thread support
    parent_comment_id BIGINT REFERENCES comments(id) ON DELETE CASCADE,
    thread_level INTEGER DEFAULT 0,
    
    -- Engagement tracking
    likes_count INTEGER DEFAULT 0,
    dislikes_count INTEGER DEFAULT 0,
    
    -- Moderation
    is_flagged BOOLEAN DEFAULT FALSE,
    is_approved BOOLEAN DEFAULT TRUE,
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    -- Ensure exactly one parent is set
    CONSTRAINT check_single_parent CHECK (
        (post_id IS NOT NULL)::int + 
        (question_id IS NOT NULL)::int + 
        (document_id IS NOT NULL)::int = 1
    )
);

-- We'll add the foreign key after creating the comments table

-- Table comments
COMMENT ON TABLE posts IS 'Community posts with engagement tracking';
COMMENT ON TABLE questions IS 'Community questions with Q&A functionality';
COMMENT ON TABLE comments IS 'Unified comments for posts, questions, and documents';

-- Now that both tables exist, add the foreign key constraint
ALTER TABLE questions 
ADD CONSTRAINT fk_questions_accepted_answer 
FOREIGN KEY (accepted_answer_id) 
REFERENCES comments(id) 
ON DELETE SET NULL 
DEFERRABLE INITIALLY DEFERRED;

-- Add index on accepted_answer_id for better query performance
CREATE INDEX IF NOT EXISTS idx_questions_accepted_answer ON questions(accepted_answer_id) WHERE accepted_answer_id IS NOT NULL;
