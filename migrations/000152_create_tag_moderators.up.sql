-- Tag moderators (Section 49: tag moderation).
-- Tracks which users moderate a general tag and who assigned them, mirroring
-- the space_moderators table from Section 05.
CREATE TABLE IF NOT EXISTS tag_moderators (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    tag_id              uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    assigned_by         text NOT NULL DEFAULT 'site_admin',  -- 'site_admin' | 'tag_owner'
    assigned_by_user_id uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, tag_id, user_id)
);

CREATE INDEX idx_tag_moderators_tag ON tag_moderators(tag_id);
CREATE INDEX idx_tag_moderators_user ON tag_moderators(user_id);
