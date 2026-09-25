-- 000305_seed_scam_alert_rbac.up.sql
-- RBAC nodes for Scam Alert Board (§81, ADR-089).
--   scam_alert.create — all (including guest, rate-limited)
--   scam_alert.review — moderator, admin, super_admin
--   scam_alert.pin    — admin, super_admin

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('scam_alert.create', 'Submit a scam report (all users, rate-limited)'),
    ('scam_alert.review', 'Review and moderate scam reports (moderator+)'),
    ('scam_alert.pin',    'Pin scam alerts to a Space (admin+)')
ON CONFLICT (key) DO NOTHING;

-- Grant scam_alert.create to all roles including guest.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.key = 'scam_alert.create'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'scam_alert.review'
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'scam_alert.pin'
ON CONFLICT DO NOTHING;

COMMIT;
