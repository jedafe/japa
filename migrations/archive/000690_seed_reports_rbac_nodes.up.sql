-- 000690_seed_reports_rbac_nodes.up.sql
-- Renumbered from 000417 — it collided with 000417_scam_alert_status_enum_align.
-- De-tenanted form: user_roles.tenant_id was dropped by 000668, so the original
-- tenant filter on the role_permissions grants is gone. Tail-position body;
-- databases that applied the 000417 file skip this via the 000000 registry marker.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('reports.submit', 'Submit content reports (default: all verified members)'),
    ('reports.view_own', 'View own submitted reports (default: all)')
ON CONFLICT (key) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'super_admin'
  AND p.key IN ('reports.submit', 'reports.view_own')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.slug = 'member'
  AND p.key IN ('reports.submit', 'reports.view_own')
ON CONFLICT DO NOTHING;

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push) VALUES
    ('report.new', 'New Report', 'A new content report was submitted', true, false, true)
ON CONFLICT (key) DO NOTHING;

COMMIT;
