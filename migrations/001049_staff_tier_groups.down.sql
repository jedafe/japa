-- 001049_staff_tier_groups.down.sql
-- Reverses 001049: drop the staff-tier settings keys, remove the staff group
-- and its backing role, clear tier on super_admin/admin-group, then restore
-- the original three-value CHECK (must run last — rows still carrying staff
-- tier values would violate it).

BEGIN;

DELETE FROM system_settings WHERE key IN (
    'organizations.max_per_user.staff',
    'organizations.max_per_user.admin',
    'organizations.max_per_user.super_admin',
    'ai.assistant.daily_queries.staff',
    'ai.assistant.daily_queries.admin',
    'ai.assistant.daily_queries.super_admin',
    'content.posts_per_day.staff',
    'content.posts_per_day.admin',
    'content.posts_per_day.super_admin',
    'credits.pack_discount_pct.staff',
    'credits.pack_discount_pct.admin',
    'credits.pack_discount_pct.super_admin',
    'jce.comparisons.max_per_day.staff',
    'jce.comparisons.max_per_day.admin',
    'jce.comparisons.max_per_day.super_admin',
    'wallet.max_daily_earn_points.staff',
    'wallet.max_daily_earn_points.admin',
    'wallet.max_daily_earn_points.super_admin'
);

-- Clear plain-FK references that would block the group delete
-- (comparison_type_access, tickets, ticket_types have no ON DELETE action).
DELETE FROM comparison_type_access
WHERE user_group_id = '00000000-0000-0000-0000-000000000060';
UPDATE tickets SET assigned_group_id = NULL
WHERE assigned_group_id = '00000000-0000-0000-0000-000000000060';
UPDATE ticket_types SET auto_assign_group_id = NULL
WHERE auto_assign_group_id = '00000000-0000-0000-0000-000000000060';

-- Remove staff group membership rows, the group_role link, the group itself
-- and the backing role (order matters for FKs).
DELETE FROM user_group_memberships
WHERE group_id = '00000000-0000-0000-0000-000000000060';
DELETE FROM group_roles
WHERE group_id = '00000000-0000-0000-0000-000000000060'
   OR role_id = '00000000-0000-0000-0000-000000000203';
DELETE FROM role_permissions
WHERE role_id = '00000000-0000-0000-0000-000000000203';
DELETE FROM user_groups WHERE id = '00000000-0000-0000-0000-000000000060';
DELETE FROM user_roles WHERE id = '00000000-0000-0000-0000-000000000203';

-- Clear staff tier tags from the pre-existing groups.
UPDATE user_groups SET tier = NULL, updated_at = now()
WHERE slug IN ('super_admin', 'admin-group') AND tier IN ('super_admin', 'admin');

ALTER TABLE user_groups DROP CONSTRAINT user_groups_tier_check;
ALTER TABLE user_groups ADD CONSTRAINT user_groups_tier_check
  CHECK (tier IN ('regular','pro','business'));

COMMIT;
