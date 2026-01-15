-- Remove environment_id from servers table
DROP INDEX IF EXISTS idx_servers_environment_id;
ALTER TABLE servers DROP COLUMN environment_id;
