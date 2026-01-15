-- Add auto_reconcile column to servers table
ALTER TABLE servers ADD COLUMN auto_reconcile INTEGER NOT NULL DEFAULT 1;
