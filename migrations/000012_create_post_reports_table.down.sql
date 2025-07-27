-- Drop the trigger first
DROP TRIGGER IF EXISTS trigger_update_post_reports_updated_at ON post_reports;

-- Drop the function
DROP FUNCTION IF EXISTS update_post_reports_updated_at();

-- Drop the table
DROP TABLE IF EXISTS post_reports;
