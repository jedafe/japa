-- 001049_staff_tier_groups.up.sql
-- Staff tiers: super_admin, admin and staff become first-class tier values on
-- user_groups.tier, ranked ABOVE the subscription tiers (regular/pro/business).
-- ResolveUserTier returns the EFFECTIVE tier (highest across all tier-bearing
-- group memberships), so every tier-scoped quota setting
-- (jce.comparisons.max_per_day.{tier}, ai.assistant.daily_queries.{tier},
-- organizations.max_per_user.{tier}, content.posts_per_day.{tier},
-- credits.pack_discount_pct.{tier}, wallet.max_daily_earn_points.{tier})
-- picks up a staff/admin/super_admin variant automatically.
--
-- Subscription mechanics are unaffected: SetUserTier still only accepts and
-- only manages the three subscription-tier groups, and users.profile_type
-- keeps its regular/pro/business CHECK.
--
-- Group mapping:
--   super_admin (system group, already seeded) -> tier 'super_admin'
--   admin-group (seeded RBAC admin group)       -> tier 'admin'
--   staff (new system group)                    -> tier 'staff'
-- A 'Staff Role' backing role is created and attached so the group renders
-- correctly in the RBAC admin UI (001032 convention); it grants no nodes —
-- the group is a quota-tier marker.

BEGIN;

-- 1) Widen the tier domain.
ALTER TABLE user_groups DROP CONSTRAINT user_groups_tier_check;
ALTER TABLE user_groups ADD CONSTRAINT user_groups_tier_check
  CHECK (tier IN ('regular','pro','business','staff','admin','super_admin'));

-- 2) Tag the existing admin groups.
UPDATE user_groups SET tier = 'super_admin', updated_at = now()
WHERE slug = 'super_admin';
UPDATE user_groups SET tier = 'admin', updated_at = now()
WHERE slug = 'admin-group';

-- 3) Staff group + backing role.
INSERT INTO user_groups (id, name, slug, is_system, tier, max_spaces_creatable)
VALUES ('00000000-0000-0000-0000-000000000060', 'Staff', 'staff', true, 'staff', -1)
ON CONFLICT (slug) DO UPDATE SET
  tier = EXCLUDED.tier,
  is_system = EXCLUDED.is_system,
  max_spaces_creatable = EXCLUDED.max_spaces_creatable;

INSERT INTO user_roles (id, name, slug, created_at, updated_at)
VALUES ('00000000-0000-0000-0000-000000000203', 'Staff Role', 'staff-role', now(), now())
ON CONFLICT (slug) DO NOTHING;

INSERT INTO group_roles (group_id, role_id)
SELECT g.id, r.id
FROM user_groups g
JOIN user_roles r ON r.slug = 'staff-role'
WHERE g.slug = 'staff'
ON CONFLICT DO NOTHING;

-- 4) Seed the staff-tier settings keys. Defaults mirror business-tier
-- entitlements (unlimited quotas, business-level credit discount) so a staff
-- member never lands below business perks; admins can tighten any key later.
INSERT INTO system_settings (key, value)
VALUES
    ('organizations.max_per_user.staff',        to_jsonb(-1)),
    ('organizations.max_per_user.admin',        to_jsonb(-1)),
    ('organizations.max_per_user.super_admin',  to_jsonb(-1)),
    ('ai.assistant.daily_queries.staff',        to_jsonb(-1)),
    ('ai.assistant.daily_queries.admin',        to_jsonb(-1)),
    ('ai.assistant.daily_queries.super_admin',  to_jsonb(-1)),
    ('content.posts_per_day.staff',             to_jsonb(-1)),
    ('content.posts_per_day.admin',             to_jsonb(-1)),
    ('content.posts_per_day.super_admin',       to_jsonb(-1)),
    ('credits.pack_discount_pct.staff',         to_jsonb(10)),
    ('credits.pack_discount_pct.admin',         to_jsonb(10)),
    ('credits.pack_discount_pct.super_admin',   to_jsonb(10)),
    ('jce.comparisons.max_per_day.staff',       to_jsonb(-1)),
    ('jce.comparisons.max_per_day.admin',       to_jsonb(-1)),
    ('jce.comparisons.max_per_day.super_admin', to_jsonb(-1)),
    ('wallet.max_daily_earn_points.staff',      to_jsonb(-1)),
    ('wallet.max_daily_earn_points.admin',      to_jsonb(-1)),
    ('wallet.max_daily_earn_points.super_admin',to_jsonb(-1))
ON CONFLICT (key) DO NOTHING;

COMMIT;
