
-- Create post_reports table
CREATE TABLE IF NOT EXISTS post_reports (
    id BIGSERIAL PRIMARY KEY,
    post_id BIGINT NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    reporter_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    reason VARCHAR(100) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'reviewed', 'resolved', 'dismissed')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMP WITH TIME ZONE,
    resolved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    resolution_notes TEXT,
    
    -- Ensure a user can only report a post once
    UNIQUE(post_id, reporter_id)
);

-- Add trigger for updated_at
CREATE OR REPLACE FUNCTION update_post_reports_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_update_post_reports_updated_at
BEFORE UPDATE ON post_reports
FOR EACH ROW
EXECUTE FUNCTION update_post_reports_updated_at();

-- Add comment for documentation
COMMENT ON TABLE post_reports IS 'Stores user reports for posts that violate community guidelines';
COMMENT ON COLUMN post_reports.status IS 'Status of the report: pending, reviewed, resolved, or dismissed';
COMMENT ON COLUMN post_reports.reason IS 'Category of the violation (e.g., spam, harassment, etc.)';
COMMENT ON COLUMN post_reports.description IS 'Detailed description of the issue provided by the reporter';
