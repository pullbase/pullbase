-- Restore token column
ALTER TABLE environments ADD COLUMN token TEXT;

-- Remove GitHub App columns
ALTER TABLE environments DROP COLUMN github_repository_id;
ALTER TABLE environments DROP COLUMN github_app_slug;
ALTER TABLE environments DROP COLUMN github_installation_id;
