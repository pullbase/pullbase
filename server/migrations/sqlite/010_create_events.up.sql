-- Migration: Add events table

CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY,
    environment_id INTEGER REFERENCES environments(id) ON DELETE CASCADE,
    server_id INTEGER,
    event_type TEXT NOT NULL,
    message TEXT NOT NULL,
    timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient querying
CREATE INDEX idx_events_environment_id ON events(environment_id);
CREATE INDEX idx_events_server_id ON events(server_id);
CREATE INDEX idx_events_event_type ON events(event_type);
CREATE INDEX idx_events_timestamp ON events(timestamp DESC);
