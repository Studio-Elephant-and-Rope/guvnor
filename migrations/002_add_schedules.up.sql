-- Add enhanced scheduling features for Guvnor
-- This migration adds additional tables and indexes for advanced scheduling

-- Add schedule rotation table for complex scheduling patterns
CREATE TABLE schedule_rotations (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    rotation_type VARCHAR(50) NOT NULL, -- daily, weekly, monthly
    rotation_config JSONB NOT NULL DEFAULT '{}',
    participants JSONB NOT NULL DEFAULT '[]', -- Array of user IDs
    start_date DATE NOT NULL,
    end_date DATE,
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT rotation_name_not_empty CHECK (LENGTH(TRIM(name)) > 0),
    CONSTRAINT rotation_type_valid CHECK (rotation_type IN ('daily', 'weekly', 'monthly')),
    CONSTRAINT rotation_date_order CHECK (end_date IS NULL OR start_date <= end_date)
);

-- Add schedule overrides table for temporary changes
CREATE TABLE schedule_overrides (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    schedule_id UUID NOT NULL REFERENCES schedules(id) ON DELETE CASCADE,
    user_id VARCHAR(255) NOT NULL,
    override_user_id VARCHAR(255) NOT NULL,
    start_time TIMESTAMPTZ NOT NULL,
    end_time TIMESTAMPTZ NOT NULL,
    reason VARCHAR(500),
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT override_user_id_not_empty CHECK (LENGTH(TRIM(user_id)) > 0),
    CONSTRAINT override_override_user_id_not_empty CHECK (LENGTH(TRIM(override_user_id)) > 0),
    CONSTRAINT override_created_by_not_empty CHECK (LENGTH(TRIM(created_by)) > 0),
    CONSTRAINT override_time_order CHECK (start_time < end_time)
);

-- Add escalation policies table
CREATE TABLE escalation_policies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    team_id VARCHAR(255) NOT NULL,
    description TEXT,
    policy_config JSONB NOT NULL DEFAULT '{}',
    active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT escalation_policy_name_not_empty CHECK (LENGTH(TRIM(name)) > 0),
    CONSTRAINT escalation_policy_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0)
);

-- Add escalation steps table
CREATE TABLE escalation_steps (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    policy_id UUID NOT NULL REFERENCES escalation_policies(id) ON DELETE CASCADE,
    step_order INTEGER NOT NULL,
    step_type VARCHAR(50) NOT NULL, -- user, schedule, webhook
    target_id VARCHAR(255) NOT NULL, -- user_id, schedule_id, or webhook_id
    delay_minutes INTEGER NOT NULL DEFAULT 0,
    step_config JSONB DEFAULT '{}',

    -- Constraints
    CONSTRAINT escalation_step_order_positive CHECK (step_order > 0),
    CONSTRAINT escalation_step_type_valid CHECK (step_type IN ('user', 'schedule', 'webhook')),
    CONSTRAINT escalation_target_id_not_empty CHECK (LENGTH(TRIM(target_id)) > 0),
    CONSTRAINT escalation_delay_positive CHECK (delay_minutes >= 0),
    UNIQUE (policy_id, step_order)
);

-- Add audit log table for tracking all changes
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    entity_type VARCHAR(100) NOT NULL, -- incident, schedule, user, etc.
    entity_id VARCHAR(255) NOT NULL,
    action VARCHAR(100) NOT NULL, -- create, update, delete
    actor VARCHAR(255) NOT NULL, -- who performed the action
    changes JSONB DEFAULT '{}', -- what changed
    metadata JSONB DEFAULT '{}', -- additional context
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT audit_entity_type_not_empty CHECK (LENGTH(TRIM(entity_type)) > 0),
    CONSTRAINT audit_entity_id_not_empty CHECK (LENGTH(TRIM(entity_id)) > 0),
    CONSTRAINT audit_action_not_empty CHECK (LENGTH(TRIM(action)) > 0),
    CONSTRAINT audit_actor_not_empty CHECK (LENGTH(TRIM(actor)) > 0)
);

-- Performance Indexes for new tables

-- Schedule rotations indexes
CREATE INDEX idx_schedule_rotations_schedule_id ON schedule_rotations(schedule_id);
CREATE INDEX idx_schedule_rotations_active ON schedule_rotations(active) WHERE active = true;
CREATE INDEX idx_schedule_rotations_type ON schedule_rotations(rotation_type);
CREATE INDEX idx_schedule_rotations_dates ON schedule_rotations(start_date, end_date);

-- JSONB indexes
CREATE INDEX idx_schedule_rotations_config ON schedule_rotations USING GIN(rotation_config);
CREATE INDEX idx_schedule_rotations_participants ON schedule_rotations USING GIN(participants);

-- Schedule overrides indexes
CREATE INDEX idx_schedule_overrides_schedule_id ON schedule_overrides(schedule_id);
CREATE INDEX idx_schedule_overrides_user_id ON schedule_overrides(user_id);
CREATE INDEX idx_schedule_overrides_override_user_id ON schedule_overrides(override_user_id);
CREATE INDEX idx_schedule_overrides_time_range ON schedule_overrides(start_time, end_time);
CREATE INDEX idx_schedule_overrides_created_by ON schedule_overrides(created_by);

-- Escalation policies indexes
CREATE INDEX idx_escalation_policies_team_id ON escalation_policies(team_id);
CREATE INDEX idx_escalation_policies_active ON escalation_policies(active) WHERE active = true;

-- JSONB indexes
CREATE INDEX idx_escalation_policies_config ON escalation_policies USING GIN(policy_config);

-- Escalation steps indexes
CREATE INDEX idx_escalation_steps_policy_id ON escalation_steps(policy_id);
CREATE INDEX idx_escalation_steps_order ON escalation_steps(policy_id, step_order);
CREATE INDEX idx_escalation_steps_type ON escalation_steps(step_type);
CREATE INDEX idx_escalation_steps_target ON escalation_steps(target_id);

-- JSONB indexes
CREATE INDEX idx_escalation_steps_config ON escalation_steps USING GIN(step_config);

-- Audit logs indexes
CREATE INDEX idx_audit_logs_entity ON audit_logs(entity_type, entity_id);
CREATE INDEX idx_audit_logs_actor ON audit_logs(actor);
CREATE INDEX idx_audit_logs_occurred_at ON audit_logs(occurred_at);
CREATE INDEX idx_audit_logs_action ON audit_logs(action);
CREATE INDEX idx_audit_logs_entity_occurred ON audit_logs(entity_type, entity_id, occurred_at);

-- JSONB indexes
CREATE INDEX idx_audit_logs_changes ON audit_logs USING GIN(changes);
CREATE INDEX idx_audit_logs_metadata ON audit_logs USING GIN(metadata);

-- Add updated_at triggers for new tables
CREATE TRIGGER update_schedule_rotations_updated_at
    BEFORE UPDATE ON schedule_rotations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_escalation_policies_updated_at
    BEFORE UPDATE ON escalation_policies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Add incident-escalation policy relationship
ALTER TABLE incidents ADD COLUMN escalation_policy_id UUID REFERENCES escalation_policies(id);
CREATE INDEX idx_incidents_escalation_policy ON incidents(escalation_policy_id) WHERE escalation_policy_id IS NOT NULL;

-- Add team preferences table for storing team-specific settings
CREATE TABLE team_preferences (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id VARCHAR(255) UNIQUE NOT NULL,
    preferences JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT team_preferences_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0)
);

-- Team preferences indexes
CREATE INDEX idx_team_preferences_team_id ON team_preferences(team_id);
CREATE INDEX idx_team_preferences_prefs ON team_preferences USING GIN(preferences);

-- Add updated_at trigger
CREATE TRIGGER update_team_preferences_updated_at
    BEFORE UPDATE ON team_preferences
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
