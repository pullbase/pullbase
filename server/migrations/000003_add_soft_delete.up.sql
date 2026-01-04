-- Add deleted_at column to servers table
ALTER TABLE servers ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMP WITH TIME ZONE;

-- Add index on deleted_at
CREATE INDEX IF NOT EXISTS idx_servers_deleted_at ON servers(deleted_at);
 
-- Remove CASCADE constraint from agent_status
ALTER TABLE agent_status DROP CONSTRAINT IF EXISTS agent_status_server_id_fkey;
ALTER TABLE agent_status ADD CONSTRAINT agent_status_server_id_fkey
    FOREIGN KEY (server_id) REFERENCES servers(id); 