-- 001027: RBAC backing-role cleanup.
--
-- Every group's permission set is materialized as a user_roles row named
-- "<group name> Role" (the "backing role"). Two drift cases accumulated
-- before the rename/delete paths learned to keep that row in sync:
--
--   1. Stale names — a renamed group's backing role kept the old
--      "<old name> Role" label, so the admin Roles page shows a mirror of
--      a group name that no longer exists.
--   2. Orphans — a deleted group's backing role stayed behind with zero
--      group_roles attachments, still listed on the Roles page.
--
-- The sweep is conservative: a role is only renamed when it can only be a
-- stale backing role (attached to exactly one group, convention-shaped
-- name, and no group still carries the name it backs), and only deleted
-- when it is unattached and convention-shaped — an unattached role grants
-- nothing, so an admin's deliberately named standalone set is at worst
-- recreated in one click. Seeded system-role slugs are excluded
-- explicitly.

-- 1) Rename stale backing roles to "<current group name> Role".
--    Slug collisions (against existing rows, or between two stale roles on
--    the same group) are skipped rather than failing the migration —
--    user_roles.slug is unique-indexed.
WITH stale AS (
    SELECT ur.id AS role_id,
           ug.name AS group_name,
           -- slugify() parity: lowercase, drop non-[a-z0-9 ._-], collapse
           -- separators to single dashes, trim dashes.
           btrim(
               regexp_replace(
                   regexp_replace(lower(ug.name || ' Role'), '[^a-z0-9 ._-]', '', 'g'),
                   '[ ._-]+', '-', 'g'),
               '-') AS new_slug
    FROM user_roles ur
    JOIN group_roles gr ON gr.role_id = ur.id
    JOIN user_groups ug ON ug.id = gr.group_id
    WHERE ur.name LIKE '% Role'
      AND ur.name <> ug.name || ' Role'
      AND NOT EXISTS (
          SELECT 1 FROM user_groups ug2
          WHERE ug2.name = substring(ur.name from 1 for length(ur.name) - 5))
      AND (SELECT COUNT(*) FROM group_roles gr2 WHERE gr2.role_id = ur.id) = 1
),
cand AS (
    SELECT s.* FROM stale s
    WHERE NOT EXISTS (SELECT 1 FROM user_roles ur3 WHERE ur3.slug = s.new_slug)
),
picked AS (
    -- Keep exactly one candidate per target slug so two stale roles on the
    -- same group can't collide with each other.
    SELECT DISTINCT ON (new_slug) * FROM cand ORDER BY new_slug, role_id
)
UPDATE user_roles ur
SET name = picked.group_name || ' Role',
    slug = picked.new_slug,
    updated_at = now()
FROM picked
WHERE ur.id = picked.role_id;

-- 2) Delete orphaned convention-shaped backing roles (zero attachments).
DELETE FROM user_roles ur
WHERE ur.name LIKE '% Role'
  AND ur.slug LIKE '%-role'
  AND NOT EXISTS (SELECT 1 FROM group_roles gr WHERE gr.role_id = ur.id)
  AND ur.slug NOT IN (
      'super_admin', 'super_admin_role', 'member', 'guest',
      'pro_member', 'business_member', 'admin-group-role');
