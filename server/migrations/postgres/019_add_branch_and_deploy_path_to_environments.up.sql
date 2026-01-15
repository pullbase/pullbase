ALTER TABLE environments
    ADD COLUMN branch TEXT NOT NULL DEFAULT 'main',
    ADD COLUMN deploy_path TEXT NOT NULL DEFAULT 'config.yaml';

ALTER TABLE environments
    ALTER COLUMN branch DROP DEFAULT,
    ALTER COLUMN deploy_path DROP DEFAULT;
