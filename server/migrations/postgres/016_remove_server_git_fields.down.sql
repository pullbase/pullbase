-- Restore Git-related fields to servers table (rollback)

ALTER TABLE servers ADD COLUMN IF NOT EXISTS repo_url VARCHAR(1024) NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS branch VARCHAR(255) NOT NULL DEFAULT 'main';
ALTER TABLE servers ADD COLUMN IF NOT EXISTS deploy_path VARCHAR(1024) NOT NULL DEFAULT 'config.yaml';

COMMENT ON TABLE servers IS 'Servers with individual Git configuration (legacy)'; 