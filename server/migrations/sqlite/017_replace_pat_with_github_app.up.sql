-- Add GitHub App columns (SQLite requires separate ALTER statements)
ALTER TABLE environments ADD COLUMN github_installation_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE environments ADD COLUMN github_app_slug TEXT;
ALTER TABLE environments ADD COLUMN github_repository_id INTEGER;

-- Drop the token column
ALTER TABLE environments DROP COLUMN token;
