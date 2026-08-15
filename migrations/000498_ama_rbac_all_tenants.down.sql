-- 000498_ama_rbac_all_tenants.down.sql
-- Reverses 000498: removes the cross-tenant grants added for
-- forum.ama.create and forum.ama.moderate.
-- Note: this does NOT remove the original (tenant-1-only) super_admin grants
-- from 000319 — those rows remain. This down migration only removes the rows
-- that 000498 inserted (professional, agent, admin, moderator grants).

BEGIN;

-- Remove forum.ama.create grants from professional + agent + admin.
-- super_admin grants (from 000319) are kept.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'forum.ama.create'
  AND ur.slug IN ('professional', 'agent', 'admin');

-- Remove forum.ama.moderate grants from moderator + admin.
-- super_admin grants (from 000319) are kept.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND rp.permission_id = p.id
  AND p.key = 'forum.ama.moderate'
  AND ur.slug IN ('moderator', 'admin');

COMMIT;
