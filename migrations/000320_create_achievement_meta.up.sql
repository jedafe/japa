-- 000320_create_achievement_meta.up.sql
-- Community Achievement Feed (§89, ADR-097) — achievement_meta table.
-- Achievements are content_type = 'achievement' in the contents table.
-- Ref: japaconnect-engineering/sections/89-achievement-feed.md

BEGIN;

CREATE TABLE IF NOT EXISTS achievement_meta (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id             uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id            uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    milestone_type        text NOT NULL, -- visa_approved|landed|first_job|graduation|citizenship|other
    destination_space_id  uuid REFERENCES spaces(id) ON DELETE SET NULL,
    origin_space_id       uuid REFERENCES spaces(id) ON DELETE SET NULL,
    is_anonymous          boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_achievement_meta_tenant_dest
    ON achievement_meta(tenant_id, destination_space_id);
CREATE INDEX IF NOT EXISTS idx_achievement_meta_tenant_type
    ON achievement_meta(tenant_id, milestone_type);

COMMIT;
