-- Restore CASCADE constraint
ALTER TABLE agent_status DROP CONSTRAINT IF EXISTS agent_status_server_id_fkey;
ALTER TABLE agent_status ADD CONSTRAINT agent_status_server_id_fkey
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE;

-- Drop index on deleted_at
DROP INDEX IF EXISTS idx_servers_deleted_at;

-- Remove deleted_at column
ALTER TABLE servers DROP COLUMN IF EXISTS deleted_at; 