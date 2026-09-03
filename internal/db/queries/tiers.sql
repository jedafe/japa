-- Tier resolution queries for the Unified User Tiers system (2026-09-03).
-- A user's effective tier is the highest tier among their user_group
-- memberships, sourced from user_groups.tier.
-- Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md

-- name: ResolveUserTier :one
-- Returns a rank (regular=1, pro=2, business=3) for the user's highest tier.
SELECT COALESCE(MAX(CASE ug.tier
    WHEN 'business' THEN 3
    WHEN 'pro' THEN 2
    WHEN 'regular' THEN 1
    ELSE 0
END), 1)::int AS tier_rank
FROM user_group_memberships ugm
JOIN user_groups ug ON ug.id = ugm.group_id AND ug.tenant_id = ugm.tenant_id
WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id;

-- name: GetTierGroupID :one
-- Returns the UUID of a tier group (members/pro/business) for a tenant.
SELECT id FROM user_groups
WHERE tenant_id = @tenant_id AND slug = @slug;

-- name: RemoveUserFromTierGroups :exec
-- Removes the user from any group that has a tier value assigned, leaving
-- non-tier groups (e.g. spaces, custom groups) untouched.
DELETE FROM user_group_memberships ugm
WHERE ugm.tenant_id = @tenant_id AND ugm.user_id = @user_id
  AND ugm.group_id IN (
    SELECT id FROM user_groups
    WHERE user_groups.tenant_id = @tenant_id AND user_groups.tier IS NOT NULL AND user_groups.tier <> ''
  );

-- name: AddUserToTierGroup :exec
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
VALUES (@tenant_id, @user_id, @group_id)
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;
