-- Section 17: Payment Core
-- Payment processing, subscriptions, discount codes, and refunds.

-- =====================
-- Payments
-- =====================
CREATE TABLE payments (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id),
    provider        text NOT NULL, -- paystack|stripe|nowpayments|manual_transfer|credit
    provider_ref    text,          -- external provider reference
    purpose_type    text NOT NULL, -- pro_membership|org_boost|donation|course_access|credit_purchase
    purpose_id      uuid,         -- optional FK to the entity being paid for
    amount          bigint NOT NULL, -- kobo/cents
    currency        text NOT NULL DEFAULT 'NGN',
    status          text NOT NULL DEFAULT 'pending', -- pending|processing|succeeded|failed|refunded|disputed
    metadata        jsonb NOT NULL DEFAULT '{}',
    subscription_id uuid,         -- optional FK to subscriptions table
    discount_code_id uuid,        -- optional FK to discount_codes table
    discount_amount bigint NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_user ON payments(user_id);
CREATE INDEX idx_payments_provider_ref ON payments(provider_ref);
CREATE INDEX idx_payments_purpose ON payments(purpose_type, purpose_id);
CREATE INDEX idx_payments_status ON payments(status);
CREATE INDEX idx_payments_tenant ON payments(tenant_id);

-- =====================
-- Payment Webhook Events (idempotency)
-- =====================
CREATE TABLE payment_webhook_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    provider    text NOT NULL,
    event_id    text NOT NULL,      -- provider's unique event ID
    raw_payload jsonb NOT NULL DEFAULT '{}',
    processed_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, provider, event_id)
);
CREATE INDEX idx_payment_webhook_events_provider ON payment_webhook_events(provider, event_id);

-- =====================
-- Subscriptions
-- =====================
CREATE TABLE subscriptions (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id             uuid NOT NULL REFERENCES users(id),
    plan                text NOT NULL, -- pro_monthly|pro_yearly
    provider            text NOT NULL, -- paystack|stripe
    provider_ref        text,
    status              text NOT NULL DEFAULT 'active', -- active|canceled|past_due|trialing
    current_period_start timestamptz NOT NULL DEFAULT now(),
    current_period_end  timestamptz NOT NULL,
    cancel_at           timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_subscriptions_user ON subscriptions(user_id);
CREATE INDEX idx_subscriptions_status ON subscriptions(status);
CREATE INDEX idx_subscriptions_tenant ON subscriptions(tenant_id);

-- =====================
-- Discount Codes
-- =====================
CREATE TABLE discount_codes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    code            text NOT NULL,
    discount_type   text NOT NULL,   -- 'percent'|'fixed'
    discount_value  numeric NOT NULL,
    applicable_to   text NOT NULL,   -- 'course_access'|'pro_membership'|'all'
    max_uses        int,             -- null = unlimited
    uses_per_user   int NOT NULL DEFAULT 1,
    group_id        uuid,            -- null = any user; non-null = scoped to this UserGroup
    valid_from      timestamptz NOT NULL DEFAULT now(),
    valid_until     timestamptz,     -- null = no expiry
    created_by      uuid REFERENCES users(id),
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, code)
);
CREATE INDEX idx_discount_codes_tenant ON discount_codes(tenant_id);
CREATE INDEX idx_discount_codes_valid ON discount_codes(valid_from, valid_until);

-- =====================
-- Discount Code Uses
-- =====================
CREATE TABLE discount_code_uses (
    code_id     uuid NOT NULL REFERENCES discount_codes(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id),
    payment_id  uuid REFERENCES payments(id),
    used_at     timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (code_id, user_id)
);
CREATE INDEX idx_discount_code_uses_code ON discount_code_uses(code_id);

-- =====================
-- Payment Refunds
-- =====================
CREATE TABLE payment_refunds (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id  uuid NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    amount      bigint NOT NULL,
    reason      text,
    status      text NOT NULL DEFAULT 'pending', -- pending|succeeded|failed
    provider_ref text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payment_refunds_payment ON payment_refunds(payment_id);