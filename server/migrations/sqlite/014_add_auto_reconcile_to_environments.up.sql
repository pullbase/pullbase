-- Add auto_reconcile column to environments table
ALTER TABLE environments ADD COLUMN auto_reconcile INTEGER NOT NULL DEFAULT 0;

-- Create index for auto_reconcile column for performance
CREATE INDEX idx_environments_auto_reconcile ON environments(auto_reconcile);
