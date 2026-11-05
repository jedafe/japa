-- Tier resolution queries for the Unified User Tiers system (2026-09-03).
-- A user's effective tier is the highest tier among their user_group
-- memberships, sourced from user_groups.tier.
-- Staff tiers (staff/admin/super_admin, migration 001049) rank above the
-- subscription tiers so every tier-scoped quota honours staff overrides.
-- Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md

-- name: ResolveUserTier :one
-- Returns a rank for the user's highest EFFECTIVE tier across all
-- tier-bearing groups (regular=1, pro=2, business=3, staff=4, admin=5,
-- super_admin=6).
SELECT COALESCE(MAX(CASE ug.tier
 WHEN 'super_admin' THEN 6
 WHEN 'admin' THEN 5
 WHEN 'staff' THEN 4
 WHEN 'business' THEN 3
 WHEN 'pro' THEN 2
 WHEN 'regular' THEN 1
 ELSE 0
END), 1)::int AS tier_rank
FROM user_group_memberships ugm
JOIN user_groups ug ON ug.id = ugm.group_id
WHERE ugm.user_id = @user_id;

-- name: ResolveSubscriptionTier :one
-- Returns a rank for the user's highest SUBSCRIPTION tier only
-- (regular=1, pro=2, business=3). Staff-tier groups are ignored — used by
-- payment grant/revoke flows where staff rank must not mask plan state.
SELECT COALESCE(MAX(CASE ug.tier
 WHEN 'business' THEN 3
 WHEN 'pro' THEN 2
 WHEN 'regular' THEN 1
 ELSE 0
END), 1)::int AS tier_rank
FROM user_group_memberships ugm
JOIN user_groups ug ON ug.id = ugm.group_id
WHERE ugm.user_id = @user_id;

-- name: GetTierGroupID :one
-- Returns the UUID of a tier group (members/pro/business) for a platform.
SELECT id FROM user_groups
WHERE slug = @slug;

-- name: RemoveUserFromTierGroups :exec
-- Removes the user from any SUBSCRIPTION-tier group (regular/pro/business),
-- leaving staff-tier groups (staff/admin/super_admin) and non-tier groups
-- (spaces, custom groups) untouched — SetUserTier must never strip a
-- user's staff/admin group membership.
DELETE FROM user_group_memberships ugm
WHERE ugm.user_id = @user_id
 AND ugm.group_id IN (
 SELECT id FROM user_groups
 WHERE user_groups.tier IN ('regular','pro','business')
 );

-- name: AddUserToTierGroup :exec
INSERT INTO user_group_memberships (user_id, group_id) VALUES (@user_id, @group_id)

ON CONFLICT (user_id, group_id) DO NOTHING;

-- name: ClearPublicGroupTierRole :exec
-- §03 alias amendment: after a tier reset, clear public_group_id when it
-- pointed at a subscription-tier group the user no longer belongs to.
-- Staff-tier groups are not touched by SetUserTier, so a public role
-- pointing at one must not be cleared here.
UPDATE users u SET public_group_id = NULL, updated_at = now()
WHERE u.id = @user_id
 AND EXISTS (
   SELECT 1 FROM user_groups g
   WHERE g.id = u.public_group_id AND g.tier IN ('regular','pro','business')
 );
