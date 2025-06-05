-- Initial schema for Guvnor incident management platform
-- This migration creates the core tables: incidents, events, signals, and schedules

-- Enable UUID extension for generating UUIDs
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Enable JSONB operators for indexing
CREATE EXTENSION IF NOT EXISTS "btree_gin";

-- Create enum types for better data integrity
CREATE TYPE incident_status AS ENUM ('triggered', 'acknowledged', 'investigating', 'resolved');
CREATE TYPE incident_severity AS ENUM ('critical', 'high', 'medium', 'low', 'info');
CREATE TYPE channel_type AS ENUM ('email', 'slack', 'sms', 'webhook', 'pagerduty');

-- Incidents table - core incident management
CREATE TABLE incidents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    title VARCHAR(200) NOT NULL,
    description TEXT,
    status incident_status NOT NULL DEFAULT 'triggered',
    severity incident_severity NOT NULL,
    team_id VARCHAR(255) NOT NULL,
    assignee_id VARCHAR(255),
    service_id VARCHAR(255),
    labels JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,

    -- Constraints
    CONSTRAINT incident_title_not_empty CHECK (LENGTH(TRIM(title)) > 0),
    CONSTRAINT incident_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0),
    CONSTRAINT incident_resolved_check CHECK (
        (status = 'resolved' AND resolved_at IS NOT NULL) OR
        (status != 'resolved' AND resolved_at IS NULL)
    )
);

-- Events table - audit trail for incidents
CREATE TABLE events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    type VARCHAR(100) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    metadata JSONB DEFAULT '{}',
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT event_type_not_empty CHECK (LENGTH(TRIM(type)) > 0),
    CONSTRAINT event_actor_not_empty CHECK (LENGTH(TRIM(actor)) > 0),
    CONSTRAINT event_description_not_empty CHECK (LENGTH(TRIM(description)) > 0)
);

-- Signals table - incoming alerts and notifications
CREATE TABLE signals (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    source VARCHAR(255) NOT NULL,
    title VARCHAR(500) NOT NULL,
    description TEXT,
    severity incident_severity NOT NULL,
    labels JSONB DEFAULT '{}',
    annotations JSONB DEFAULT '{}',
    payload JSONB DEFAULT '{}',
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT signal_source_not_empty CHECK (LENGTH(TRIM(source)) > 0),
    CONSTRAINT signal_title_not_empty CHECK (LENGTH(TRIM(title)) > 0)
);

-- Incident signals junction table - many-to-many relationship
CREATE TABLE incident_signals (
    incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
    signal_id UUID NOT NULL REFERENCES signals(id) ON DELETE CASCADE,
    attached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (incident_id, signal_id)
);

-- Schedules table - on-call scheduling (mentioned in acceptance criteria)
CREATE TABLE schedules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    team_id VARCHAR(255) NOT NULL,
    timezone VARCHAR(100) NOT NULL DEFAULT 'UTC',
    config JSONB NOT NULL DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT schedule_name_not_empty CHECK (LENGTH(TRIM(name)) > 0),
    CONSTRAINT schedule_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0),
    CONSTRAINT schedule_timezone_not_empty CHECK (LENGTH(TRIM(timezone)) > 0)
);

-- Schedule shifts table - individual shifts within schedules
CREATE TABLE schedule_shifts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT shift_user_id_not_empty CHECK (LENGTH(TRIM(user_id)) > 0),
    CONSTRAINT shift_time_order CHECK (start_time < end_time)
);

-- Notification rules table - how and when to notify
CREATE TABLE notification_rules (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    team_id VARCHAR(255) NOT NULL,
    severity incident_severity NOT NULL,
    channel_type channel_type NOT NULL,
    channel_config JSONB NOT NULL DEFAULT '{}',
    delay_minutes INTEGER NOT NULL DEFAULT 0,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT notification_rule_name_not_empty CHECK (LENGTH(TRIM(name)) > 0),
    CONSTRAINT notification_rule_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0),
    CONSTRAINT notification_rule_delay_positive CHECK (delay_minutes >= 0)
);

-- Performance Indexes
-- These indexes are critical for a system that handles alerts at 3 AM

-- Incidents indexes
CREATE INDEX idx_incidents_status ON incidents(status);
CREATE INDEX idx_incidents_severity ON incidents(severity);
CREATE INDEX idx_incidents_team_id ON incidents(team_id);
CREATE INDEX idx_incidents_assignee_id ON incidents(assignee_id) WHERE assignee_id IS NOT NULL;
CREATE INDEX idx_incidents_service_id ON incidents(service_id) WHERE service_id IS NOT NULL;
CREATE INDEX idx_incidents_created_at ON incidents(created_at);
CREATE INDEX idx_incidents_updated_at ON incidents(updated_at);
CREATE INDEX idx_incidents_resolved_at ON incidents(resolved_at) WHERE resolved_at IS NOT NULL;
CREATE INDEX idx_incidents_team_status ON incidents(team_id, status);
CREATE INDEX idx_incidents_team_severity ON incidents(team_id, severity);

-- JSONB indexes for flexible querying
CREATE INDEX idx_incidents_labels ON incidents USING GIN(labels);

-- Events indexes
CREATE INDEX idx_events_incident_id ON events(incident_id);
CREATE INDEX idx_events_type ON events(type);
CREATE INDEX idx_events_actor ON events(actor);
CREATE INDEX idx_events_occurred_at ON events(occurred_at);
CREATE INDEX idx_events_incident_occurred ON events(incident_id, occurred_at);

-- JSONB indexes
CREATE INDEX idx_events_metadata ON events USING GIN(metadata);

-- Signals indexes
CREATE INDEX idx_signals_source ON signals(source);
CREATE INDEX idx_signals_severity ON signals(severity);
CREATE INDEX idx_signals_received_at ON signals(received_at);
CREATE INDEX idx_signals_source_received ON signals(source, received_at);

-- JSONB indexes
CREATE INDEX idx_signals_labels ON signals USING GIN(labels);
CREATE INDEX idx_signals_annotations ON signals USING GIN(annotations);
CREATE INDEX idx_signals_payload ON signals USING GIN(payload);

-- Incident signals indexes
CREATE INDEX idx_incident_signals_signal_id ON incident_signals(signal_id);
CREATE INDEX idx_incident_signals_attached_at ON incident_signals(attached_at);

-- Schedules indexes
CREATE INDEX idx_schedules_team_id ON schedules(team_id);
CREATE INDEX idx_schedules_active ON schedules(active) WHERE active = true;
CREATE INDEX idx_schedules_team_active ON schedules(team_id, active);

-- JSONB indexes
CREATE INDEX idx_schedules_config ON schedules USING GIN(config);

-- Schedule shifts indexes
CREATE INDEX idx_schedule_shifts_schedule_id ON schedule_shifts(schedule_id);
CREATE INDEX idx_schedule_shifts_user_id ON schedule_shifts(user_id);
CREATE INDEX idx_schedule_shifts_start_time ON schedule_shifts(start_time);
CREATE INDEX idx_schedule_shifts_end_time ON schedule_shifts(end_time);
CREATE INDEX idx_schedule_shifts_time_range ON schedule_shifts(start_time, end_time);
CREATE INDEX idx_schedule_shifts_user_time ON schedule_shifts(user_id, start_time, end_time);

-- Notification rules indexes
CREATE INDEX idx_notification_rules_team_id ON notification_rules(team_id);
CREATE INDEX idx_notification_rules_severity ON notification_rules(severity);
CREATE INDEX idx_notification_rules_channel_type ON notification_rules(channel_type);
CREATE INDEX idx_notification_rules_active ON notification_rules(active) WHERE active = true;
CREATE INDEX idx_notification_rules_team_severity ON notification_rules(team_id, severity);

-- JSONB indexes
CREATE INDEX idx_notification_rules_config ON notification_rules USING GIN(channel_config);

-- Create updated_at trigger function
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Apply updated_at triggers
CREATE TRIGGER update_incidents_updated_at
    BEFORE UPDATE ON incidents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_schedules_updated_at
    BEFORE UPDATE ON schedules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_notification_rules_updated_at
    BEFORE UPDATE ON notification_rules
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
