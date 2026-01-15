ALTER TABLE agent_status ADD COLUMN drift_details TEXT;

CREATE INDEX idx_agent_status_drift ON agent_status(server_id, is_drifted) WHERE is_drifted = 1;
