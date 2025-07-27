
-- =======================================
-- CORE TABLES (Users, Categories, Sessions)
-- =======================================

CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    
    -- Authentication fields
    email VARCHAR(320) NOT NULL UNIQUE,
    username VARCHAR(50) NOT NULL UNIQUE,
    password_hash VARCHAR(255),
    
    -- Profile information
    first_name VARCHAR(100),
    last_name VARCHAR(100),
    display_name VARCHAR(150) GENERATED ALWAYS AS (
        CASE 
            WHEN first_name IS NOT NULL AND last_name IS NOT NULL 
            THEN first_name || ' ' || last_name
            WHEN first_name IS NOT NULL 
            THEN first_name
            ELSE username
        END
    ) STORED,
    
    -- Professional information
    affiliation VARCHAR(255),
    job_title VARCHAR(150),
    bio TEXT,
    years_experience SMALLINT DEFAULT 0 CHECK (years_experience >= 0 AND years_experience <= 100),
    core_competencies TEXT,
    expertise expertise_level DEFAULT 'none' NOT NULL,
    
    -- File storage (Cloudinary URLs)
    profile_url TEXT,
    profile_public_id VARCHAR(255),
    cv_url TEXT,
    cv_public_id VARCHAR(255),
    
    -- Contact and social
    website_url TEXT,
    linkedin_profile TEXT,
    twitter_handle VARCHAR(50),
    
    -- System fields
    role user_role DEFAULT 'user' NOT NULL,
    is_verified BOOLEAN DEFAULT FALSE NOT NULL,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    is_online BOOLEAN DEFAULT FALSE NOT NULL,
    email_notifications BOOLEAN DEFAULT TRUE NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    last_seen TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    email_verified_at TIMESTAMPTZ,
    password_changed_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    
    -- Constraints
    CONSTRAINT users_email_format CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$'),
    CONSTRAINT users_username_format CHECK (username ~* '^[a-zA-Z0-9_-]{3,50}$'),
    CONSTRAINT users_website_url_format CHECK (website_url IS NULL OR website_url ~* '^https?://'),
    CONSTRAINT users_twitter_handle_format CHECK (twitter_handle IS NULL OR twitter_handle ~* '^[A-Za-z0-9_]{1,50}$')
);

-- Categories table
CREATE TABLE IF NOT EXISTS categories (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    icon VARCHAR(100),
    color VARCHAR(7), -- hex color
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Sessions table (enhanced)
CREATE TABLE IF NOT EXISTS sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_activity TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    ip_address INET,
    user_agent TEXT,
    is_active BOOLEAN DEFAULT TRUE NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Table comments for documentation
COMMENT ON TABLE users IS 'Core user accounts with authentication and profile information';
COMMENT ON TABLE categories IS 'Content categories for posts and questions';
COMMENT ON TABLE sessions IS 'User session management with security tracking';
