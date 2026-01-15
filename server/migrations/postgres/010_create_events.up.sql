-- Migration: Add events table
-- Created: 2025-06-22

CREATE TABLE IF NOT EXISTS events (
    id BIGSERIAL PRIMARY KEY,
    environment_id BIGINT REFERENCES environments(id) ON DELETE CASCADE,
    server_id BIGINT,
    event_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    timestamp TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient querying
CREATE INDEX idx_events_environment_id ON events(environment_id);
CREATE INDEX idx_events_server_id ON events(server_id);
CREATE INDEX idx_events_event_type ON events(event_type);
CREATE INDEX idx_events_timestamp ON events(timestamp DESC);

-- Add comment for documentation
COMMENT ON TABLE events IS 'System events and audit trail';
COMMENT ON COLUMN events.event_type IS 'Type of event (e.g., rollback_completed, config_applied)';
COMMENT ON COLUMN events.message IS 'Human-readable event description'; 