-- Migration: Add rollback_events table

CREATE TABLE IF NOT EXISTS rollback_events (
    id INTEGER PRIMARY KEY,
    environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    from_commit TEXT NOT NULL,
    to_commit TEXT NOT NULL,
    initiated_by TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'in_progress', 'completed', 'failed')),
    reason TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    error_message TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes for efficient querying
CREATE INDEX idx_rollback_events_environment_id ON rollback_events(environment_id);
CREATE INDEX idx_rollback_events_status ON rollback_events(status);
CREATE INDEX idx_rollback_events_created_at ON rollback_events(created_at DESC);
