-- 000122_moderation_alerts.up.sql
-- Adds the moderation_alerts table backing the admin moderation alerts page
-- (audit A13 — entire /admin/moderation/alerts surface was missing).

CREATE TABLE moderation_alerts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id),
    alert_type      text NOT NULL,
    severity        text NOT NULL DEFAULT 'medium',
    title           text NOT NULL,
    description     text,
    status          text NOT NULL DEFAULT 'open',
    meta            jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,
    resolved_at     timestamptz,
    acknowledged_by uuid REFERENCES users(id),
    resolved_by     uuid REFERENCES users(id)
);

CREATE INDEX idx_moderation_alerts_tenant ON moderation_alerts(tenant_id);
CREATE INDEX idx_moderation_alerts_status ON moderation_alerts(tenant_id, status);

-- Permission node for administering moderation alerts.
INSERT INTO permissions (key, description) VALUES
    ('moderation.alerts.manage', 'Moderation Alerts Manage')
ON CONFLICT (key) DO NOTHING;

-- Grant super_admin the new node.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'moderation.alerts.manage'
ON CONFLICT DO NOTHING;

COMMIT;
