-- Rollback enhanced scheduling features for Guvnor
-- This migration removes all tables, indexes, and columns added in 002_add_schedules.up.sql

-- Drop triggers first
DROP TRIGGER IF EXISTS update_team_preferences_updated_at ON team_preferences;
DROP TRIGGER IF EXISTS update_escalation_policies_updated_at ON escalation_policies;
DROP TRIGGER IF EXISTS update_schedule_rotations_updated_at ON schedule_rotations;

-- Drop the escalation policy column from incidents table
ALTER TABLE incidents DROP COLUMN IF EXISTS escalation_policy_id;

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS team_preferences;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS escalation_steps;
DROP TABLE IF EXISTS escalation_policies;
DROP TABLE IF EXISTS schedule_overrides;
DROP TABLE IF EXISTS schedule_rotations;
