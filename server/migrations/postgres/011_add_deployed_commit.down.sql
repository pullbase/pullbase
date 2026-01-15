-- Migration: Remove deployed_commit field from environments table
-- Created: 2025-06-22

ALTER TABLE environments DROP COLUMN IF EXISTS deployed_commit; 