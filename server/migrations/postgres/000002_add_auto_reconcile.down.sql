-- Remove auto_reconcile column from servers table
ALTER TABLE servers DROP COLUMN IF EXISTS auto_reconcile; 