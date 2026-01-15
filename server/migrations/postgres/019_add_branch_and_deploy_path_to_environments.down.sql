ALTER TABLE environments
    DROP COLUMN IF EXISTS branch,
    DROP COLUMN IF EXISTS deploy_path;
