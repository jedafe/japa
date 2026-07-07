-- Phase 10: User and space mute tables (Section 52 — Mute System).
-- Migration 000102: creates user_mutes and space_mutes tables.

CREATE TABLE IF NOT EXISTS user_mutes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    muted_user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, muted_user_id)
);

CREATE INDEX IF NOT EXISTS idx_user_mutes_user ON user_mutes(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_user_mutes_muted ON user_mutes(tenant_id, muted_user_id);

CREATE TABLE IF NOT EXISTS space_mutes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    space_id    uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, user_id, space_id)
);

CREATE INDEX IF NOT EXISTS idx_space_mutes_user ON space_mutes(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_space_mutes_space ON space_mutes(tenant_id, space_id);
