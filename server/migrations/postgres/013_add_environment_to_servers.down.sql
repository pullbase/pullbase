-- Remove environment_id from servers table
DROP INDEX IF EXISTS idx_servers_environment_id;
ALTER TABLE servers DROP CONSTRAINT IF EXISTS fk_servers_environment_id;
ALTER TABLE servers DROP COLUMN IF EXISTS environment_id; 