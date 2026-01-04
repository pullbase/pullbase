-- Migration: Restore polling support and fallback status
-- Rollback: 2025-06-23

-- Add back the last_poll_at column
ALTER TABLE environments ADD COLUMN last_poll_at TIMESTAMP;

-- Update status constraint to include 'fallback' 
ALTER TABLE environments DROP CONSTRAINT IF EXISTS environments_status_check;
ALTER TABLE environments ADD CONSTRAINT environments_status_check 
    CHECK (status IN ('pending', 'active', 'error', 'fallback'));

-- Remove comments
COMMENT ON TABLE environments IS NULL;
COMMENT ON COLUMN environments.status IS NULL; 