-- Remove redundant Git-related fields from servers table
-- These fields should come from the environment that the server belongs to

ALTER TABLE servers DROP COLUMN repo_url;
ALTER TABLE servers DROP COLUMN branch;
ALTER TABLE servers DROP COLUMN deploy_path;
