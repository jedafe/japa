-- Section 34 (Referral): Referral program and referral tracking tables
-- Supports configurable referral rewards and referral code tracking.

CREATE TABLE IF NOT EXISTS referral_programs (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id              uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    referrer_reward_cents  int NOT NULL DEFAULT 500,
    referee_reward_cents   int NOT NULL DEFAULT 500,
    max_referrals          int NOT NULL DEFAULT 0,  -- 0 = unlimited
    cookie_days            int NOT NULL DEFAULT 30,
    is_active              boolean NOT NULL DEFAULT true,
    description            text NOT NULL DEFAULT '',
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id)
);

CREATE TABLE IF NOT EXISTS referrals (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    referrer_id   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    referee_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    referral_code text NOT NULL,
    status        text NOT NULL DEFAULT 'pending',  -- 'pending' | 'qualified' | 'rewarded' | 'expired'
    reward_paid_at timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_referrals_tenant ON referrals(tenant_id, status);
CREATE INDEX idx_referrals_referrer ON referrals(referrer_id);
CREATE INDEX idx_referrals_code ON referrals(referral_code);
