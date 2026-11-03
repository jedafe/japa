-- 001020_drop_content_edit_permission.up.sql
-- Drops the legacy `content.edit` permission node. It predates the
-- _own/_any scoping convention (migration 000516 introduced
-- content.edit_own / content.edit_any) and is dead weight: no code path
-- checks the bare `content.edit` key — every reference left in comments
-- was stale documentation for what is actually a content.edit_any check.
-- Role grants are removed first to satisfy the FK before the row drops.

BEGIN;

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key = 'content.edit';

DELETE FROM permissions WHERE key = 'content.edit';

COMMIT;
