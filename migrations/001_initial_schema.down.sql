-- Rollback initial schema for Guvnor incident management platform
-- This migration removes all tables, indexes, functions, and types created in 001_initial_schema.up.sql

-- Drop triggers first
DROP TRIGGER IF EXISTS update_notification_rules_updated_at ON notification_rules;
DROP TRIGGER IF EXISTS update_schedules_updated_at ON schedules;
DROP TRIGGER IF EXISTS update_incidents_updated_at ON incidents;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop tables in reverse dependency order
DROP TABLE IF EXISTS notification_rules;
DROP TABLE IF EXISTS schedule_shifts;
DROP TABLE IF EXISTS schedules;
DROP TABLE IF EXISTS incident_signals;
DROP TABLE IF EXISTS signals;
DROP TABLE IF EXISTS events;
DROP TABLE IF EXISTS incidents;

-- Drop enum types
DROP TYPE IF EXISTS channel_type;
DROP TYPE IF EXISTS incident_severity;
DROP TYPE IF EXISTS incident_status;

-- Note: We don't drop extensions as they might be used by other applications
-- DROP EXTENSION IF EXISTS "btree_gin";
-- DROP EXTENSION IF EXISTS "uuid-ossp";
