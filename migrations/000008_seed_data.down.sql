
-- Remove seed data in reverse order
DELETE FROM notifications WHERE type = 'system_update' AND title = 'Welcome to EvalHub!';
DELETE FROM users WHERE email = 'admin@evalhub.com';
DELETE FROM categories WHERE name IN (
    'M&E in Science',
    'M&E in Health', 
    'M&E in Education',
    'M&E in Social Sciences',
    'M&E in Climate Change',
    'M&E in Agriculture',
    'General M&E'
);
