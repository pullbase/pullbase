-- Migration: Remove polling support and fallback status
-- Note: SQLite cannot modify CHECK constraints. The constraint was already
-- created without 'fallback' in the initial schema, so this is a no-op.
-- We only need to update any existing fallback statuses.

UPDATE environments SET status = 'error' WHERE status = 'fallback';
