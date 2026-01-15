-- Migration: Add rollback_events table
-- Created: 2025-06-20

CREATE TABLE IF NOT EXISTS rollback_events (
    id BIGSERIAL PRIMARY KEY,
    environment_id BIGINT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    from_commit VARCHAR(40) NOT NULL,
    to_commit VARCHAR(40) NOT NULL,
    initiated_by VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP,
    error_message TEXT,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient querying
CREATE INDEX idx_rollback_events_environment_id ON rollback_events(environment_id);
CREATE INDEX idx_rollback_events_status ON rollback_events(status);
CREATE INDEX idx_rollback_events_created_at ON rollback_events(created_at DESC);

-- Add trigger to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_rollback_events_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_rollback_events_updated_at
    BEFORE UPDATE ON rollback_events
    FOR EACH ROW
    EXECUTE FUNCTION update_rollback_events_updated_at();

-- Add comment for documentation
COMMENT ON TABLE rollback_events IS 'Tracks rollback operations and their status';
COMMENT ON COLUMN rollback_events.status IS 'Current status: pending, in_progress, completed, failed';
COMMENT ON COLUMN rollback_events.from_commit IS 'Original commit being rolled back from';
COMMENT ON COLUMN rollback_events.to_commit IS 'Target commit to roll back to';
COMMENT ON COLUMN rollback_events.initiated_by IS 'User or system that initiated the rollback';
