-- 000626_seed_calendar_edit_permissions.up.sql
-- Adds the missing calendar.edit_own and calendar.edit_any permission nodes
-- for the PATCH /api/v1/calendar/:id public edit route.
-- Ref: japaconnect-engineering/sections/86-community-calendar.md

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('calendar.edit_own', 'Edit own calendar entries (member+)'),
    ('calendar.edit_any', 'Edit any calendar entry (moderator/admin+)')
ON CONFLICT (key) DO NOTHING;

-- Grant edit_own to members and super_admins.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('member', 'super_admin')
  AND p.key = 'calendar.edit_own'
ON CONFLICT DO NOTHING;

-- Grant edit_any to super_admins (admin+).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin')
  AND p.key = 'calendar.edit_any'
ON CONFLICT DO NOTHING;

COMMIT;
