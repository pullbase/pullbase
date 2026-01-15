-- Rollback: Remove drift_details column from agent_status table

DROP INDEX IF EXISTS idx_agent_status_drift;
ALTER TABLE agent_status DROP COLUMN IF EXISTS drift_details;
