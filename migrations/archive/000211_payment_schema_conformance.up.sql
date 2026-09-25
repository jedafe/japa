-- Section 17: Payment Core schema conformance with JEOS spec.
-- Fixes BUG-013 (subscription schema drift), BUG-014 (missing CHECK
-- constraints), BUG-015 (missing idempotency_key on payments),
-- BUG-017 (missing event_type on payment_webhook_events), and
-- BUG-021 (missing course_access_grants table).

-- =====================
-- BUG-015: payments.idempotency_key
-- JEOS §17 schema defines idempotency_key text UNIQUE for DB-level
-- duplicate charge prevention (Rule 9).
-- =====================
ALTER TABLE payments ADD COLUMN IF NOT EXISTS idempotency_key text UNIQUE;

-- =====================
-- BUG-017: payment_webhook_events.event_type
-- JEOS §17 schema defines event_type text NOT NULL for auditing.
-- =====================
ALTER TABLE payment_webhook_events
    ADD COLUMN IF NOT EXISTS event_type text NOT NULL DEFAULT '';

-- =====================
-- BUG-013: Subscription schema drift
-- JEOS §17 defines provider_sub_id (NOT NULL UNIQUE), cancel_at_period_end
-- (boolean), and cancelled_at (timestamptz). The original migration had
-- provider_ref (nullable, no unique) and cancel_at (wrong semantics).
-- We add the new columns and keep provider_ref for backward compatibility.
-- =====================
ALTER TABLE subscriptions
    ADD COLUMN IF NOT EXISTS provider_sub_id text UNIQUE,
    ADD COLUMN IF NOT EXISTS cancel_at_period_end boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS cancelled_at timestamptz;

-- =====================
-- BUG-014: CHECK constraints on enumerated columns
-- JEOS §17 defines CHECK constraints that prevent invalid values.
-- =====================

-- payments.provider
ALTER TABLE payments
    ADD CONSTRAINT payments_provider_check
    CHECK (provider IN ('stripe','paystack','nowpayments','manual_transfer','credit'));

-- payments.purpose_type
ALTER TABLE payments
    ADD CONSTRAINT payments_purpose_type_check
    CHECK (purpose_type IN ('pro_membership','org_boost','donation','course_access','credit_purchase'));

-- payments.status
ALTER TABLE payments
    ADD CONSTRAINT payments_status_check
    CHECK (status IN ('pending','processing','succeeded','failed','refunded','cancelled'));

-- subscriptions.plan
ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_plan_check
    CHECK (plan IN ('pro_monthly','pro_annual','pro_yearly','business_monthly','business_yearly'));

-- subscriptions.provider
ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_provider_check
    CHECK (provider IN ('stripe','paystack'));

-- subscriptions.status
ALTER TABLE subscriptions
    ADD CONSTRAINT subscriptions_status_check
    CHECK (status IN ('active','past_due','cancelled','canceled','expired','trialing'));

-- discount_codes.discount_type
ALTER TABLE discount_codes
    ADD CONSTRAINT discount_codes_discount_type_check
    CHECK (discount_type IN ('percent','fixed','percentage'));

-- discount_codes.applicable_to
ALTER TABLE discount_codes
    ADD CONSTRAINT discount_codes_applicable_to_check
    CHECK (applicable_to IN ('course_access','pro_membership','all'));

-- =====================
-- BUG-021: course_access_grants table
-- JEOS §17 schema defines one-time course access grants from payment or
-- Pro subscription. Used for refund revocation (BUG-003) and Pro
-- subscription "included course access" benefit tracking.
-- =====================
CREATE TABLE IF NOT EXISTS course_access_grants (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id  uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    grant_type  text NOT NULL CHECK (grant_type IN ('purchase','subscription','admin')),
    payment_id  uuid REFERENCES payments(id),
    granted_at  timestamptz NOT NULL DEFAULT now(),
    revoked_at  timestamptz,
    UNIQUE(user_id, content_id)
);
CREATE INDEX IF NOT EXISTS idx_course_access_grants_user ON course_access_grants(user_id);
CREATE INDEX IF NOT EXISTS idx_course_access_grants_content ON course_access_grants(content_id);
