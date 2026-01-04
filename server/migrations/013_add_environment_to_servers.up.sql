-- Add environment_id to servers table for environment-level agent access
ALTER TABLE servers ADD COLUMN environment_id INTEGER;

-- Add foreign key constraint to environments table  
ALTER TABLE servers ADD CONSTRAINT fk_servers_environment_id 
    FOREIGN KEY (environment_id) REFERENCES environments(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_servers_environment_id ON servers(environment_id);

COMMENT ON COLUMN servers.environment_id IS 'Optional link to environment for environment-level agent authentication'; 