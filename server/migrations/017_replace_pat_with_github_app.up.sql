ALTER TABLE environments
    ADD COLUMN github_installation_id BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN github_app_slug TEXT,
    ADD COLUMN github_repository_id BIGINT;

UPDATE environments
SET github_installation_id = 0
WHERE github_installation_id IS NULL;

ALTER TABLE environments
    ALTER COLUMN github_installation_id DROP DEFAULT;

ALTER TABLE environments
    DROP COLUMN token;
