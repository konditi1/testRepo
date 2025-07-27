-- =======================================
-- JOB SYSTEM TABLES
-- =======================================

-- Jobs table (enhanced)
CREATE TABLE IF NOT EXISTS jobs (
    id BIGSERIAL PRIMARY KEY,
    employer_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    requirements TEXT,
    responsibilities TEXT,
    
    -- Employment details
    employment_type employment_type NOT NULL,
    location VARCHAR(255),
    salary_range VARCHAR(100),
    is_remote BOOLEAN DEFAULT FALSE,
    
    -- Timing
    application_deadline TIMESTAMPTZ,
    start_date TIMESTAMPTZ,
    
    -- Status and tracking
    status job_status DEFAULT 'draft' NOT NULL,
    views_count INTEGER DEFAULT 0,
    applications_count INTEGER DEFAULT 0,
    
    -- SEO and metadata
    slug VARCHAR(255) UNIQUE,
    tags TEXT[],
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    published_at TIMESTAMPTZ
);

-- Job applications table
CREATE TABLE IF NOT EXISTS job_applications (
    id BIGSERIAL PRIMARY KEY,
    job_id BIGINT NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    applicant_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    
    -- Application content
    cover_letter TEXT NOT NULL,
    application_letter_url TEXT,
    application_letter_public_id VARCHAR(255),
    
    -- Status tracking
    status application_status DEFAULT 'pending' NOT NULL,
    notes TEXT,
    
    -- Timestamps
    applied_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    reviewed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    -- Unique constraint
    UNIQUE(job_id, applicant_id)
);

-- Table comments
COMMENT ON TABLE jobs IS 'Job postings with application tracking';
COMMENT ON TABLE job_applications IS 'Job applications with status tracking';
