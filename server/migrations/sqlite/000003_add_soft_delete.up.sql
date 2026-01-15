-- Add deleted_at column to servers table
ALTER TABLE servers ADD COLUMN deleted_at DATETIME;

-- Add index on deleted_at
CREATE INDEX IF NOT EXISTS idx_servers_deleted_at ON servers(deleted_at);

-- Note: SQLite cannot modify foreign key constraints after table creation.
-- The foreign key behavior change (removing CASCADE) is handled at application level.
