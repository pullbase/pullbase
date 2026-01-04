-- Remove redundant Git-related fields from servers table
-- These fields should come from the environment that the server belongs to

ALTER TABLE servers DROP COLUMN IF EXISTS repo_url;
ALTER TABLE servers DROP COLUMN IF EXISTS branch;
ALTER TABLE servers DROP COLUMN IF EXISTS deploy_path;

COMMENT ON TABLE servers IS 'Servers inherit Git configuration from their environment'; 