DROP INDEX IF EXISTS idx_agent_status_drift;
ALTER TABLE agent_status DROP COLUMN drift_details;
