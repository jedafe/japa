-- 000424_drop_orphaned_tag_rbac_nodes.up.sql
-- BUG-004 (tags audit): the permission nodes `tags.create`, `tags.edit`, and
-- `tags.delete` were seeded in 000099/000109/000125 but are never referenced
-- by any RequirePermission call in the server. All admin tag operations are
-- gated solely on `tags.manage` (server.go /admin/tags routes). These three
-- nodes are dead: they inflate the node count, confuse RBAC auditing, and
-- are documented in permission-key-naming.md as superseded by `tags.manage`.
--
-- This migration drops the role_permissions grants and the permission rows
-- themselves. The i18n translation keys `tags.create` / `tags.edit` /
-- `tags.delete` (in the i18n_translations table) are unrelated and are NOT
-- touched — they remain as UI labels for the admin panel.
--
-- Idempotent: safe to re-run.

BEGIN;

-- 1. Drop role_permissions grants referencing the three orphaned nodes.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions
    WHERE key IN ('tags.create', 'tags.edit', 'tags.delete')
);

-- 2. Drop the orphaned permission rows.
DELETE FROM permissions
WHERE key IN ('tags.create', 'tags.edit', 'tags.delete');

COMMIT;
