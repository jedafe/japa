-- 000628_seed_scam_alert_edit_permissions.up.sql
-- Adds the missing scam_alert.edit_own and scam_alert.edit_any permission
-- nodes for the PATCH /api/v1/scam-alerts/:id public edit route.
--   scam_alert.edit_own — all authenticated roles (author-only edit; the
--     service still enforces ownership — the node only gates route access)
--   scam_alert.edit_any — moderator, admin, super_admin (edit override)
-- Grants are applied across ALL tenants (pattern: 000485).
--
-- Ref: japaconnect-engineering/sections/81-scam-alert-board.md
-- Ref: japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('scam_alert.edit_own', 'Edit own scam report (authenticated members)'),
    ('scam_alert.edit_any', 'Edit any scam report (moderator/admin+)')
ON CONFLICT (key) DO NOTHING;

-- scam_alert.edit_own — every non-guest role in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'scam_alert.edit_own'
  AND ur.slug <> 'guest'
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

-- scam_alert.edit_any — moderator + admin + super_admin in all tenants.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE p.key = 'scam_alert.edit_any'
  AND ur.slug IN ('moderator', 'admin', 'super_admin')
  AND NOT EXISTS (
      SELECT 1 FROM role_permissions rp
      WHERE rp.role_id = ur.id AND rp.permission_id = p.id
  )
ON CONFLICT DO NOTHING;

COMMIT;
