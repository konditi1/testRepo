-- =======================================
-- SYSTEM TABLES (Messages, Notifications)
-- =======================================

-- Messages table (for chat/notifications)
CREATE TABLE IF NOT EXISTS messages (
    id BIGSERIAL PRIMARY KEY,
    sender_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    is_read BOOLEAN DEFAULT FALSE NOT NULL,
    message_type notification_type DEFAULT 'chat_message',
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    read_at TIMESTAMPTZ
);

-- Notifications table (system notifications)
CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type notification_type NOT NULL,
    title VARCHAR(255) NOT NULL,
    content TEXT,
    
    -- Related entity references
    related_post_id BIGINT REFERENCES posts(id) ON DELETE CASCADE,
    related_question_id BIGINT REFERENCES questions(id) ON DELETE CASCADE,
    related_comment_id BIGINT REFERENCES comments(id) ON DELETE CASCADE,
    related_job_id BIGINT REFERENCES jobs(id) ON DELETE CASCADE,
    related_user_id BIGINT REFERENCES users(id) ON DELETE CASCADE,
    
    -- Status
    is_read BOOLEAN DEFAULT FALSE NOT NULL,
    is_sent BOOLEAN DEFAULT FALSE NOT NULL,
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP NOT NULL,
    read_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ
);

-- Table comments
COMMENT ON TABLE messages IS 'Direct messages between users';
COMMENT ON TABLE notifications IS 'System notifications for users';