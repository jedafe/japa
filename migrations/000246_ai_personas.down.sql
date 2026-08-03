-- 000246_ai_personas.down.sql
-- Reverses 000246_ai_personas.up.sql.

BEGIN;

-- Revoke persona.* grants from super_admin.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND rp.permission_id = p.id
  AND p.key IN ('persona.manage', 'persona.view');

-- Remove persona.* permission nodes.
DELETE FROM permissions WHERE key IN ('persona.manage', 'persona.view');

-- Drop the ai_personas table.
DROP TABLE IF EXISTS ai_personas;

-- Remove platform_events.is_synthetic column + index.
DROP INDEX IF EXISTS idx_pe_synthetic;
ALTER TABLE platform_events DROP COLUMN IF EXISTS is_synthetic;

-- Remove users.is_bot column + index.
DROP INDEX IF EXISTS idx_users_is_bot;
ALTER TABLE users DROP COLUMN IF EXISTS is_bot;

COMMIT;
