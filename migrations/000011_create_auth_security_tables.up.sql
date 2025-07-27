-- 000011_create_auth_security_tables.up.sql

-- Login attempts table for security monitoring
CREATE TABLE IF NOT EXISTS login_attempts (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(320) NOT NULL,
    ip_address INET,
    user_agent TEXT,
    success BOOLEAN NOT NULL,
    failure_reason VARCHAR(100),
    attempted_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Password reset tokens table
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Email verification tokens table
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token VARCHAR(255) UNIQUE NOT NULL,
    email VARCHAR(320) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    verified_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL
);

-- Account locks table (for temporary locks)
CREATE TABLE IF NOT EXISTS account_locks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(255) NOT NULL,
    locked_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    locked_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    unlocked_at TIMESTAMPTZ,
    unlocked_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);

-- Comments
COMMENT ON TABLE login_attempts IS 'Login attempt tracking for security';
COMMENT ON TABLE password_reset_tokens IS 'Password reset token management';
COMMENT ON TABLE email_verification_tokens IS 'Email verification token management';
COMMENT ON TABLE account_locks IS 'Account lock history and management';