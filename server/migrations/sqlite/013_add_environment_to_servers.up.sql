-- Add environment_id to servers table for environment-level agent access
ALTER TABLE servers ADD COLUMN environment_id INTEGER REFERENCES environments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_servers_environment_id ON servers(environment_id);
