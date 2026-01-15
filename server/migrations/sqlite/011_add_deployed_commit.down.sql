-- Migration: Remove deployed_commit field from environments table

ALTER TABLE environments DROP COLUMN deployed_commit;
