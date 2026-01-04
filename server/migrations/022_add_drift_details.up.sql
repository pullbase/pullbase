-- Migration: Add drift_details column to agent_status table
-- This stores the before/after state when drift is detected

ALTER TABLE agent_status ADD COLUMN drift_details JSONB;

-- Add index for querying drifted status entries with details
CREATE INDEX idx_agent_status_drift ON agent_status(server_id, is_drifted) WHERE is_drifted = true;

COMMENT ON COLUMN agent_status.drift_details IS 'JSON object containing drift details: packages, services, files that differ from desired state';
