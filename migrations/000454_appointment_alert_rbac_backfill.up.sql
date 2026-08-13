-- 000454_appointment_alert_rbac_backfill.up.sql
-- Backfill: grant appointment_alert.create and appointment_alert.manage_own
-- to professional, agent, moderator, and admin roles that were missing from
-- the original 000328 seed (BUG-004). The spec (§91) says these are "member+"
-- nodes, but 000328 only granted them to 'member' and 'super_admin'.
-- Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('professional', 'agent', 'moderator', 'admin')
  AND p.key IN ('appointment_alert.create', 'appointment_alert.manage_own')
ON CONFLICT DO NOTHING;

COMMIT;
