-- 000603_add_credit_withdrawal.up.sql
-- Section 58 amendment: credit withdrawal (cashout) infrastructure.
-- Ref: japaconnect-engineering/sections/58-credit-currency-system.md

BEGIN;

-- =========================================
-- 1. Extend wallet ledger operations
-- =========================================
ALTER TABLE wallet_ledger
    DROP CONSTRAINT wallet_ledger_operation_check,
    ADD CONSTRAINT wallet_ledger_operation_check
        CHECK (operation = ANY (ARRAY['earn', 'spend', 'purchase', 'convert', 'refund', 'clawback', 'admin_adjust', 'withdraw']));

-- =========================================
-- 2. User withdrawal methods (payout destinations)
-- =========================================
CREATE TABLE wallet_withdrawal_methods (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider      text NOT NULL,             -- 'nowpayments', 'paypal', 'paystack', 'flutterwave', 'stripe', 'bank_transfer', 'manual_crypto'
    label         text,
    account       text NOT NULL,             -- encrypted payout destination (address, email, bank account)
    currency_code text REFERENCES currencies(code),
    is_verified   boolean NOT NULL DEFAULT false,
    is_default    boolean NOT NULL DEFAULT false,
    is_active     boolean NOT NULL DEFAULT true,
    metadata      jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wallet_withdrawal_methods_user
    ON wallet_withdrawal_methods(tenant_id, user_id, is_active);
CREATE INDEX idx_wallet_withdrawal_methods_default
    ON wallet_withdrawal_methods(tenant_id, user_id, is_default)
    WHERE is_default = true;

-- =========================================
-- 3. Payout provider configuration per tenant
-- =========================================
CREATE TABLE payout_providers (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type                 text NOT NULL,     -- 'nowpayments', 'paypal', 'paystack', 'flutterwave', 'stripe', 'bank_transfer', 'manual_crypto'
    name                 text NOT NULL,
    is_active            boolean NOT NULL DEFAULT false,
    is_default           boolean NOT NULL DEFAULT false,
    config               jsonb NOT NULL DEFAULT '{}',
    supported_currencies text[] NOT NULL DEFAULT '{}',
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, type)
);

CREATE INDEX idx_payout_providers_active
    ON payout_providers(tenant_id, is_active);

-- =========================================
-- 4. Withdrawal fee tiers
-- =========================================
CREATE TABLE wallet_withdrawal_fee_tiers (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    min_amount int NOT NULL DEFAULT 0,       -- inclusive lower bound in credit or fiat minor units
    max_amount int,                          -- null = open-ended
    fee_type   text NOT NULL CHECK (fee_type IN ('percentage', 'fixed')),
    fee_value  int NOT NULL,                 -- percentage in basis points or fixed minor units
    tier_mode  text NOT NULL DEFAULT 'credit' CHECK (tier_mode IN ('credit', 'fiat')),
    is_active  boolean NOT NULL DEFAULT true,
    position   int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_wallet_withdrawal_fee_tiers_tenant
    ON wallet_withdrawal_fee_tiers(tenant_id, is_active, position);

-- =========================================
-- 5. Withdrawal requests
-- =========================================
CREATE TABLE wallet_withdrawals (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credit_amount      bigint NOT NULL CHECK (credit_amount > 0),
    currency_code      text NOT NULL REFERENCES currencies(code),
    credits_per_unit   int NOT NULL,         -- snapshot of currency rate at submission
    fiat_amount        bigint NOT NULL,      -- credit_amount / credits_per_unit, minor units
    fee_amount         bigint NOT NULL DEFAULT 0,
    fee_basis          text NOT NULL DEFAULT 'percentage' CHECK (fee_basis IN ('percentage', 'fixed')),
    fee_rate           numeric(10,6) NOT NULL DEFAULT 0, -- snapshot: basis points or fixed minor units
    net_amount         bigint NOT NULL,      -- fiat_amount - fee_amount
    status             text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'pending_approval', 'approved', 'processing', 'completed', 'failed', 'rejected', 'cancelled')),
    payout_method_id   uuid REFERENCES wallet_withdrawal_methods(id),
    payout_provider    text NOT NULL,
    provider_ref       text,
    admin_note         text,
    reviewed_by        uuid REFERENCES users(id),
    reviewed_at        timestamptz,
    idempotency_key    text,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, idempotency_key)
);

CREATE INDEX idx_wallet_withdrawals_user
    ON wallet_withdrawals(tenant_id, user_id, created_at DESC);
CREATE INDEX idx_wallet_withdrawals_status
    ON wallet_withdrawals(tenant_id, status, created_at DESC);

-- =========================================
-- 6. User group withdrawal policy
-- =========================================
ALTER TABLE user_groups
    ADD COLUMN IF NOT EXISTS auto_withdrawal_limit int DEFAULT 0,            -- 0 = no auto-approval limit (always auto); null = always require approval
    ADD COLUMN IF NOT EXISTS requires_withdrawal_approval boolean NOT NULL DEFAULT true;

-- =========================================
-- 7. Permission nodes
-- =========================================
INSERT INTO permissions (key, description) VALUES
    ('wallet.withdraw',            'Request credit withdrawals'),
    ('wallet.withdraw.approve',    'Approve or reject credit withdrawals'),
    ('wallet.withdraw.manage',     'Manage withdrawal settings and fee tiers'),
    ('wallet.payout_provider.manage', 'Manage payout providers (NOWPayments, PayPal, etc.)')
ON CONFLICT (key) DO NOTHING;

-- Grant new nodes to super_admin and members (request only)
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
CROSS JOIN permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug IN ('super_admin', 'admin')
  AND p.key IN ('wallet.withdraw', 'wallet.withdraw.approve', 'wallet.withdraw.manage', 'wallet.payout_provider.manage')
ON CONFLICT DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur
CROSS JOIN permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key = 'wallet.withdraw'
ON CONFLICT DO NOTHING;

-- =========================================
-- 8. System settings defaults
-- =========================================
INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'wallet.withdrawal_default_currency', '"USD"'),
    ('00000000-0000-0000-0000-000000000001', 'wallet.daily_withdrawal_limit', '100000'),     -- credits
    ('00000000-0000-0000-0000-000000000001', 'wallet.monthly_withdrawal_limit', '1000000'),  -- credits
    ('00000000-0000-0000-0000-000000000001', 'wallet.min_withdrawal_credits', '1000')
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
