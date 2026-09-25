-- Down migration for 000211_payment_schema_conformance

-- Remove course_access_grants table (BUG-021)
DROP TABLE IF EXISTS course_access_grants;

-- Remove CHECK constraints (BUG-014)
ALTER TABLE discount_codes DROP CONSTRAINT IF EXISTS discount_codes_applicable_to_check;
ALTER TABLE discount_codes DROP CONSTRAINT IF EXISTS discount_codes_discount_type_check;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_status_check;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_provider_check;
ALTER TABLE subscriptions DROP CONSTRAINT IF EXISTS subscriptions_plan_check;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_status_check;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_purpose_type_check;
ALTER TABLE payments DROP CONSTRAINT IF EXISTS payments_provider_check;

-- Remove subscription columns (BUG-013)
ALTER TABLE subscriptions
    DROP COLUMN IF EXISTS cancelled_at,
    DROP COLUMN IF EXISTS cancel_at_period_end,
    DROP COLUMN IF EXISTS provider_sub_id;

-- Remove event_type column (BUG-017)
ALTER TABLE payment_webhook_events DROP COLUMN IF EXISTS event_type;

-- Remove idempotency_key column (BUG-015)
ALTER TABLE payments DROP COLUMN IF EXISTS idempotency_key;
