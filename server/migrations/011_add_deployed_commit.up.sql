-- Migration: Add deployed_commit field to environments table
-- Created: 2025-06-22

ALTER TABLE environments ADD COLUMN deployed_commit VARCHAR(40);

-- Add comment for documentation
COMMENT ON COLUMN environments.deployed_commit IS 'Currently deployed commit hash for this environment'; 