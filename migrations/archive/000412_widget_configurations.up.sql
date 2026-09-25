-- Widget configurations: data-source-driven dynamic blocks (Section 16).
-- Stores admin-configured widgets that render live data (latest content,
-- trending tags, space members, etc.) with Valkey-cached output.
CREATE TABLE IF NOT EXISTS widget_configurations (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    widget_type     text NOT NULL,
    -- 'latest_content'|'trending_tags'|'space_members'|'user_leaderboard'
    -- |'upcoming_events'|'featured_spaces'|'custom_html'
    config          jsonb NOT NULL DEFAULT '{}',
    cache_ttl_secs  int NOT NULL DEFAULT 300,
    is_active       boolean NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_widget_configurations_tenant ON widget_configurations (tenant_id, is_active);
CREATE UNIQUE INDEX IF NOT EXISTS idx_widget_configurations_tenant_name ON widget_configurations (tenant_id, name);

-- RBAC: widgets.manage gates all admin widget CRUD routes (Section 16).
INSERT INTO permissions (key, description)
VALUES ('widgets.manage', 'Manage widget configurations')
ON CONFLICT (key) DO NOTHING;

-- Grant widgets.manage to super_admin and admin roles on the seed tenant.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'admin')
  AND p.key = 'widgets.manage'
ON CONFLICT DO NOTHING;
