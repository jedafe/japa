-- 000259_referral_system_v2.up.sql
-- Referral System V2 (Section 60, ADR-078).
-- Adds the referral_codes table and extends the existing referrals table
-- with V2 columns (qualified_payment_id, reward_amount, flagged_reason,
-- qualified_at, credited_at). Seeds referral.* system_settings + RBAC nodes.
--
-- Ref: japaconnect-engineering/sections/60-referral-system.md
--      japaconnect-engineering/adr/ADR-078-referral-system.md

BEGIN;

-- =====================================================================
-- referral_codes — one code per user, platform-wide.
-- =====================================================================
CREATE TABLE IF NOT EXISTS referral_codes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    code        text NOT NULL UNIQUE,
    click_count int  NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_referral_codes_code ON referral_codes (code);
CREATE INDEX IF NOT EXISTS idx_referral_codes_tenant ON referral_codes (tenant_id, user_id);

-- =====================================================================
-- Extend the existing referrals table with V2 columns.
-- The existing table has: id, tenant_id, referrer_id, referee_id,
-- referral_code, status, reward_paid_at, created_at.
-- V2 adds: qualified_payment_id, reward_amount, flagged_reason,
-- qualified_at, credited_at. Also extends the status CHECK constraint.
-- =====================================================================

-- Add new columns (IF NOT EXISTS for idempotency).
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS qualified_payment_id uuid REFERENCES payments(id) ON DELETE SET NULL;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS reward_amount numeric(12,2);
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS flagged_reason text;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS qualified_at timestamptz;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS credited_at timestamptz;

-- The existing status CHECK may only allow 'pending','qualified','credited'.
-- We need to add 'flagged' and 'rejected'. Drop and recreate to be safe.
ALTER TABLE referrals DROP CONSTRAINT IF EXISTS referrals_status_check;
ALTER TABLE referrals ADD CONSTRAINT referrals_status_check
    CHECK (status IN ('pending','qualified','credited','flagged','rejected'));

CREATE INDEX IF NOT EXISTS idx_referrals_referrer ON referrals (tenant_id, referrer_id, status);
CREATE INDEX IF NOT EXISTS idx_referrals_status ON referrals (tenant_id, status, created_at DESC);

-- =====================================================================
-- system_settings — 4 referral.* keys (Section 60, ADR-078)
-- =====================================================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'referral.enabled',                '"true"'),
    ('00000000-0000-0000-0000-000000000001', 'referral.reward_credits',         '"500"'),
    ('00000000-0000-0000-0000-000000000001', 'referral.attribution_window_days', '"30"'),
    ('00000000-0000-0000-0000-000000000001', 'referral.max_rewards_per_period',  '"10"')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes — 2 referral.* nodes (Section 60, ADR-078)
-- =====================================================================
INSERT INTO permissions (key, description) VALUES
    ('referral.use',    'Access personal referral code and stats (all authenticated users)'),
    ('referral.manage', 'Admin: view all referrals, resolve flagged, adjust settings')
ON CONFLICT (key) DO NOTHING;

-- Grant referral.manage to super_admin.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key = 'referral.manage'
ON CONFLICT DO NOTHING;

-- Grant referral.use to the default 'member' role.
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'referral.use'
ON CONFLICT DO NOTHING;

COMMIT;
