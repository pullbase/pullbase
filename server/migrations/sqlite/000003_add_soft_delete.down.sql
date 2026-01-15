-- Drop index on deleted_at
DROP INDEX IF EXISTS idx_servers_deleted_at;

-- Remove deleted_at column
ALTER TABLE servers DROP COLUMN deleted_at;
