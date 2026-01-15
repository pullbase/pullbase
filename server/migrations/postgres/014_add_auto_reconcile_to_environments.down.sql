-- Remove auto_reconcile column from environments table
DROP INDEX IF EXISTS idx_environments_auto_reconcile;
ALTER TABLE environments DROP COLUMN IF EXISTS auto_reconcile; 