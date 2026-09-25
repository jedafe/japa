CREATE TABLE IF NOT EXISTS content_views (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    space_id   uuid REFERENCES spaces(id) ON DELETE SET NULL,
    viewed_at  timestamptz NOT NULL DEFAULT now(),
    ip_hash    text
);

CREATE INDEX IF NOT EXISTS idx_content_views_tenant_space ON content_views(tenant_id, space_id);
CREATE INDEX IF NOT EXISTS idx_content_views_tenant_content ON content_views(tenant_id, content_id);
