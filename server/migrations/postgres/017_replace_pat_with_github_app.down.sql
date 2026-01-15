ALTER TABLE environments
    ADD COLUMN token TEXT;

ALTER TABLE environments
    DROP COLUMN github_repository_id,
    DROP COLUMN github_app_slug,
    DROP COLUMN github_installation_id;
