-- 000606_unify_user_tiers.up.sql
-- Add user tier groups, rename users.profile_type, and seed tier settings.

BEGIN;

-- Add tier columns.
ALTER TABLE user_groups
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business'));

ALTER TABLE subscription_plans
  ADD COLUMN IF NOT EXISTS tier text CHECK (tier IN ('regular','pro','business'));

-- Seed pro and business tier groups for the default tenant.
INSERT INTO user_groups (id, tenant_id, name, slug, is_system, tier, max_spaces_creatable)
VALUES
  ('00000000-0000-0000-0000-000000000040', '00000000-0000-0000-0000-000000000001', 'Pro', 'pro', true, 'pro', -1),
  ('00000000-0000-0000-0000-000000000050', '00000000-0000-0000-0000-000000000001', 'Business', 'business', true, 'business', -1)
ON CONFLICT (tenant_id, slug) DO UPDATE SET
  tier = EXCLUDED.tier,
  max_spaces_creatable = EXCLUDED.max_spaces_creatable;

-- Assign the base members group to the regular tier.
UPDATE user_groups
SET tier = 'regular', max_spaces_creatable = 3
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'members';

-- Backfill user group memberships: everyone gets the base members group.
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
SELECT u.tenant_id, u.id, g.id
FROM users u
JOIN user_groups g ON g.tenant_id = u.tenant_id AND g.slug = 'members'
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;

-- Business/organisation users also get the business tier group.
INSERT INTO user_group_memberships (tenant_id, user_id, group_id)
SELECT u.tenant_id, u.id, g.id
FROM users u
JOIN user_groups g ON g.tenant_id = u.tenant_id AND g.slug = 'business'
WHERE u.profile_type IN ('business', 'organisation')
ON CONFLICT (tenant_id, user_id, group_id) DO NOTHING;

-- Rename profile_type values before adding the new CHECK constraint.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_profile_type_check;

UPDATE users
SET profile_type = CASE profile_type
  WHEN 'personal' THEN 'regular'
  WHEN 'business' THEN 'business'
  WHEN 'organisation' THEN 'business'
  ELSE 'regular'
END;

ALTER TABLE users ADD CONSTRAINT users_profile_type_check CHECK (profile_type IN ('regular','pro','business'));

-- Map subscription plan slugs to tiers.
UPDATE subscription_plans
SET tier = CASE slug
  WHEN 'free' THEN 'regular'
  WHEN 'pro' THEN 'pro'
  WHEN 'business' THEN 'business'
  ELSE 'regular'
END;

-- Remove the "Professional profile" feature from the Pro plan's feature list.
UPDATE subscription_plans
SET features = '[{"text":"Everything in Free","included":true},{"text":"Unlimited posts","included":true},{"text":"AI assistant (100 queries/day)","included":true},{"text":"Priority support","included":true},{"text":"Advanced analytics","included":true},{"text":"Document verification","included":true},{"text":"Custom branding","included":false}]'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'pro';

COMMIT;
