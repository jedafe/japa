-- 000159_granular_bans.up.sql
-- Granular ban system: per-operation bans + typed warnings.

-- Enum of restrictable operations.
CREATE TYPE ban_operation AS ENUM (
    'post',
    'comment',
    'discussion',
    'blog',
    'message'
);

-- Per-operation ban restrictions.
CREATE TABLE user_operation_bans (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    operation   ban_operation NOT NULL,
    reason      text,
    expires_at  timestamptz,
    issued_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE UNIQUE INDEX idx_uob_active_unique
    ON user_operation_bans (user_id, operation)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_uob_user_active
    ON user_operation_bans (user_id)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_uob_tenant
    ON user_operation_bans (tenant_id, operation);

-- Add violation_type to warnings.
ALTER TABLE user_moderation_actions ADD COLUMN violation_type text;

CREATE INDEX idx_uma_violation_type
    ON user_moderation_actions (user_id, violation_type)
    WHERE action = 'warning' AND deleted_at IS NULL;

-- Allow 'suspension' action value.
ALTER TABLE user_moderation_actions
    DROP CONSTRAINT user_moderation_actions_action_check;
ALTER TABLE user_moderation_actions
    ADD CONSTRAINT user_moderation_actions_action_check
    CHECK (action IN ('ban', 'unban', 'warning', 'suspension'));
