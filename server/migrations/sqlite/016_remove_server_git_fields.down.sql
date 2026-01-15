-- Restore Git-related fields to servers table (rollback)

ALTER TABLE servers ADD COLUMN repo_url TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN branch TEXT NOT NULL DEFAULT 'main';
ALTER TABLE servers ADD COLUMN deploy_path TEXT NOT NULL DEFAULT 'config.yaml';
