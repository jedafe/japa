-- Space Members table for space membership tracking.
-- Supports join/leave toggle functionality.

CREATE TABLE space_members (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    role        text NOT NULL DEFAULT 'member',
    joined_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(space_id, user_id)
);

CREATE INDEX idx_space_members_space ON space_members(space_id);
CREATE INDEX idx_space_members_user ON space_members(user_id);
CREATE INDEX idx_space_members_tenant ON space_members(tenant_id);
