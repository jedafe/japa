-- Wallet & Credits System (Section 28)
-- Two-tier internal currency: Points (soft, activity-earned) and Credits (spendable, monetary-equivalent).

-- =====================
-- Wallet Ledger
-- =====================
CREATE TABLE wallet_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ledger_type TEXT NOT NULL CHECK (ledger_type IN ('points', 'credits')),
    operation TEXT NOT NULL CHECK (operation IN ('earn', 'spend', 'purchase', 'convert', 'refund', 'clawback', 'admin_adjust')),
    amount BIGINT NOT NULL,
    resulting_balance BIGINT NOT NULL,
    source TEXT,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    reference TEXT,
    idempotency_key TEXT UNIQUE,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ledger indexes for balance calculation and querying
CREATE INDEX idx_wallet_ledger_user_type_created ON wallet_ledger (user_id, ledger_type, created_at DESC);
CREATE INDEX idx_wallet_ledger_tenant_user ON wallet_ledger (tenant_id, user_id);
CREATE INDEX idx_wallet_ledger_idempotency ON wallet_ledger (idempotency_key) WHERE idempotency_key IS NOT NULL;

-- =====================
-- Activity Point Rules
-- =====================
CREATE TABLE activity_point_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    activity_key TEXT NOT NULL,
    point_value INT NOT NULL,
    is_enabled BOOLEAN DEFAULT true,
    cap_per_day INT,
    cap_per_week INT,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, activity_key)
);

-- Activity rules index
CREATE INDEX idx_activity_point_rules_tenant ON activity_point_rules (tenant_id);
CREATE INDEX idx_activity_point_rules_key ON activity_point_rules (tenant_id, activity_key);

-- =====================
-- Activity Events (for async processing)
-- =====================
CREATE TABLE activity_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_key TEXT NOT NULL,
    reference TEXT NOT NULL,
    payload JSONB DEFAULT '{}',
    processed BOOLEAN DEFAULT false,
    processed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, activity_key, reference)
);

-- Activity events indexes for queue processing and cap checking
CREATE INDEX idx_activity_events_user_key ON activity_events (user_id, activity_key);
CREATE INDEX idx_activity_events_unprocessed ON activity_events (processed, created_at) WHERE processed = false;
CREATE INDEX idx_activity_events_tenant ON activity_events (tenant_id);

-- =====================
-- Wallet Settings
-- =====================
CREATE TABLE wallet_settings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    points_to_credits_rate INT DEFAULT 100, -- 100 points = 1 credit
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(tenant_id)
);

-- Wallet settings index
CREATE INDEX idx_wallet_settings_tenant ON wallet_settings (tenant_id);