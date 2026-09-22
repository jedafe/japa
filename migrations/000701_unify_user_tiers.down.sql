-- 000701_unify_user_tiers.down.sql
-- Renumbered from 000606 (duplicate version prefix). See the .up file.
-- 000606_unify_user_tiers.down.sql
-- Best-effort revert of the tier unification migration.

BEGIN;

-- Restore subscription plan features (best-effort; re-adds "Professional profile").
UPDATE subscription_plans
SET features = '[{"text":"Everything in Free","included":true},{"text":"Unlimited posts","included":true},{"text":"Professional profile","included":true},{"text":"AI assistant (100 queries/day)","included":true},{"text":"Priority support","included":true},{"text":"Advanced analytics","included":true},{"text":"Document verification","included":true},{"text":"Custom branding","included":false}]'::jsonb
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'pro';

UPDATE subscription_plans SET tier = NULL;

-- Restore profile_type values. Cannot restore business vs organisation distinction.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_profile_type_check;

UPDATE users
SET profile_type = CASE profile_type
  WHEN 'regular' THEN 'personal'
  WHEN 'pro' THEN 'business'
  WHEN 'business' THEN 'business'
  ELSE 'personal'
END;

ALTER TABLE users ADD CONSTRAINT users_profile_type_check CHECK (profile_type IN ('personal','business','organisation'));

-- Remove tier group memberships for pro/business groups.
DELETE FROM user_group_memberships
WHERE group_id IN (
  SELECT id FROM user_groups
  WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug IN ('pro','business')
);

-- Remove pro/business tier groups.
DELETE FROM user_groups
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug IN ('pro','business');

-- Remove the members tier assignment.
UPDATE user_groups
SET tier = NULL
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'members';

ALTER TABLE subscription_plans DROP COLUMN IF EXISTS tier;
ALTER TABLE user_groups DROP COLUMN IF EXISTS tier;

COMMIT;
