-- Migration: Remove polling support and fallback status
-- Created: 2025-06-23

-- Drop the last_poll_at column
ALTER TABLE environments DROP COLUMN IF EXISTS last_poll_at;

-- Update status constraint to remove 'fallback'
ALTER TABLE environments DROP CONSTRAINT IF EXISTS environments_status_check;
ALTER TABLE environments ADD CONSTRAINT environments_status_check 
    CHECK (status IN ('pending', 'active', 'error'));

-- Update any existing fallback statuses to error
UPDATE environments SET status = 'error' WHERE status = 'fallback';

-- Add comment for documentation
COMMENT ON TABLE environments IS 'Git environments monitored via webhooks only';
COMMENT ON COLUMN environments.status IS 'Webhook status: pending, active, error'; 