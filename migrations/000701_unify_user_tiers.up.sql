-- 000701_unify_user_tiers.up.sql
-- Renumbered from 000606 — it collided with 000606_activity_rule_one_time.
-- De-tenanted form: tenant_id was dropped from user_groups, subscription_plans,
-- users, and user_group_memberships by 000668 — all tenant filters and columns
-- are gone, and the unique keys are now (slug) on user_groups and
-- (user_id, group_id) on user_group_memberships. Tail-position body; databases
-- that applied the 000606 file skip this via the 000000 registry marker.

BEGIN;

ALTER TABLE user_groups
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business'));
ALTER TABLE subscription_plans
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business'));

INSERT INTO user_groups (id, name, slug, is_system, tier, max_spaces_creatable)
VALUES
  ('00000000-0000-0000-0000-000000000040', 'Pro', 'pro', true, 'pro', -1),
  ('00000000-0000-0000-0000-000000000050', 'Business', 'business', true, 'business', -1)
ON CONFLICT (slug) DO UPDATE SET
  tier = EXCLUDED.tier,
  max_spaces_creatable = EXCLUDED.max_spaces_creatable;

UPDATE user_groups
SET tier = 'regular', max_spaces_creatable = 3
WHERE slug = 'members';

INSERT INTO user_group_memberships (user_id, group_id)
SELECT u.id, g.id
FROM users u
JOIN user_groups g ON g.slug = 'members'
ON CONFLICT (user_id, group_id) DO NOTHING;

INSERT INTO user_group_memberships (user_id, group_id)
SELECT u.id, g.id
FROM users u
JOIN user_groups g ON g.slug = 'business'
WHERE u.profile_type IN ('business', 'organisation')
ON CONFLICT (user_id, group_id) DO NOTHING;

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_profile_type_check;
UPDATE users
SET profile_type = CASE profile_type
  WHEN 'personal' THEN 'regular'
  WHEN 'business' THEN 'business'
  WHEN 'organisation' THEN 'business'
  ELSE 'regular'
END;
ALTER TABLE users ADD CONSTRAINT users_profile_type_check CHECK (profile_type IN ('regular','pro','business'));

COMMIT;
