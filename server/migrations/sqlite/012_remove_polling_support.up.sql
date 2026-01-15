-- Migration: Remove polling support and fallback status
UPDATE environments SET status = 'error' WHERE status = 'fallback';