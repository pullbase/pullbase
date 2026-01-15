-- Migration: Add deployed_commit field to environments table

ALTER TABLE environments ADD COLUMN deployed_commit TEXT;
