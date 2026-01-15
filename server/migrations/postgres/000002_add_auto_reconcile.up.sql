-- Add auto_reconcile column to servers table
ALTER TABLE servers ADD COLUMN IF NOT EXISTS auto_reconcile BOOLEAN NOT NULL DEFAULT TRUE; 