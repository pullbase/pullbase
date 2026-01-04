-- Migration: Remove rollback_events table
-- Created: 2025-06-22

DROP TABLE IF EXISTS rollback_events CASCADE; 