-- 000456_grant_attestation_guide_update_member_plus.up.sql
-- Fix BUG-010: §97 specifies attestation_guide.update as "member+", meaning
-- all roles at member tier and above. The original seed (000338) only granted
-- it to member and super_admin. This grants it to professional, agent,
-- moderator, and admin so all member+ roles can submit community updates.

BEGIN;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('professional', 'agent', 'moderator', 'admin')
  AND p.key = 'attestation_guide.update'
ON CONFLICT DO NOTHING;

COMMIT;
