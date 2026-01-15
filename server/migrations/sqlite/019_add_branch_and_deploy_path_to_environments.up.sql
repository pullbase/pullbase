-- Add branch and deploy_path columns (SQLite requires separate ALTER statements)
ALTER TABLE environments ADD COLUMN branch TEXT NOT NULL DEFAULT 'main';
ALTER TABLE environments ADD COLUMN deploy_path TEXT NOT NULL DEFAULT 'config.yaml';
