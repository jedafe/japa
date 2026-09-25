-- 000259_referral_system_v2.down.sql
-- Reverses 000259_referral_system_v2.up.sql.

BEGIN;

-- Revoke referral.* grants.
DELETE FROM role_permissions rp
USING user_roles ur, permissions p
WHERE rp.role_id = ur.id
  AND ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'member')
  AND rp.permission_id = p.id
  AND p.key IN ('referral.use', 'referral.manage');

-- Remove referral.* permission nodes.
DELETE FROM permissions WHERE key IN ('referral.use', 'referral.manage');

-- Remove referral.* system_settings.
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN ('referral.enabled', 'referral.reward_credits',
              'referral.attribution_window_days', 'referral.max_rewards_per_period');

-- Drop V2 indexes.
DROP INDEX IF EXISTS idx_referrals_status;
DROP INDEX IF EXISTS idx_referrals_referrer;

-- Revert the status CHECK constraint to the original (best-effort).
ALTER TABLE referrals DROP CONSTRAINT IF EXISTS referrals_status_check;

-- Drop V2 columns.
ALTER TABLE referrals DROP COLUMN IF EXISTS credited_at;
ALTER TABLE referrals DROP COLUMN IF EXISTS qualified_at;
ALTER TABLE referrals DROP COLUMN IF EXISTS flagged_reason;
ALTER TABLE referrals DROP COLUMN IF EXISTS reward_amount;
ALTER TABLE referrals DROP COLUMN IF EXISTS qualified_payment_id;

-- Drop the referral_codes table.
DROP TABLE IF EXISTS referral_codes;

COMMIT;
