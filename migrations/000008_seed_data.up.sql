-- =======================================
-- SEED DATA (Initial Application Data)
-- =======================================

-- Insert default categories
INSERT INTO categories (name, description, icon, color) VALUES 
    ('M&E in Science', 'Monitoring and Evaluation in Scientific Research', '🔬', '#3B82F6'),
    ('M&E in Health', 'Health Program Monitoring and Evaluation', '🏥', '#10B981'),
    ('M&E in Education', 'Educational Program Assessment', '📚', '#F59E0B'),
    ('M&E in Social Sciences', 'Social Program Evaluation', '👥', '#8B5CF6'),
    ('M&E in Climate Change', 'Climate and Environmental M&E', '🌱', '#059669'),
    ('M&E in Agriculture', 'Agricultural Program Monitoring', '🌾', '#84CC16'),
    ('General M&E', 'General Monitoring and Evaluation Topics', '📊', '#6B7280')
ON CONFLICT (name) DO NOTHING;

-- Insert default admin user (optional - for initial setup)
INSERT INTO users (
    email, 
    username, 
    password_hash, 
    first_name, 
    last_name, 
    role, 
    is_verified, 
    is_active,
    expertise
) VALUES (
    'admin@evalhub.com',
    'admin',
    '$2a$12$LQv3c1yqBWVHxkd0LHAkCOYz6TtxMQJqhN8/LewgcHyGhpkONzPh6', -- password: admin123
    'System',
    'Administrator',
    'admin',
    true,
    true,
    'expert'
) ON CONFLICT (email) DO NOTHING;

-- Insert welcome notification for admin
INSERT INTO notifications (
    user_id,
    type,
    title,
    content,
    is_read,
    is_sent
) 
SELECT 
    u.id,
    'system_update',
    'Welcome to EvalHub!',
    'Welcome to the EvalHub platform. Your account has been set up successfully.',
    false,
    true
FROM users u 
WHERE u.email = 'admin@evalhub.com'
ON CONFLICT DO NOTHING;

-- Insert sample job employment types reference data (if needed)
-- This ensures all enum values are represented in the application

-- Comments for documentation
COMMENT ON SCHEMA public IS 'EvalHub consolidated database schema with seed data';

-- Log successful seed data insertion
DO $$
BEGIN
    RAISE NOTICE 'Seed data inserted successfully';
    RAISE NOTICE 'Categories: %', (SELECT COUNT(*) FROM categories);
    RAISE NOTICE 'Admin users: %', (SELECT COUNT(*) FROM users WHERE role = 'admin');
END $$;
