-- Add github_id column to users table
ALTER TABLE users 
ADD COLUMN github_id BIGINT UNIQUE;
