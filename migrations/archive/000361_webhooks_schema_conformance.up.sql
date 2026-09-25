-- Section 12 (Webhooks Outbound) schema conformance.
-- Adds the columns mandated by .jeos/sections/12-webhooks.md that were missing
-- from migration 000035: name, is_active, verified_at, created_by on
-- webhook_endpoints; next_attempt_at, last_response_code, last_response_body
-- on webhook_deliveries; plus the status CHECK constraint with 'retrying'.
--
-- Ref: japaconnect-engineering/sections/12-webhooks.md

-- webhook_endpoints: §12 mandates name, is_active (default false until
-- challenge/response verified), verified_at, created_by (FK users ON DELETE
-- SET NULL).
ALTER TABLE webhook_endpoints
    ADD COLUMN IF NOT EXISTS name text,
    ADD COLUMN IF NOT EXISTS is_active boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS verified_at timestamptz,
    ADD COLUMN IF NOT EXISTS created_by uuid REFERENCES users(id) ON DELETE SET NULL;

-- Backfill name from url for existing rows so the NOT NULL constraint can be
-- applied safely.
UPDATE webhook_endpoints SET name = url WHERE name IS NULL;

ALTER TABLE webhook_endpoints
    ALTER COLUMN name SET NOT NULL;

-- Existing endpoints that were created under the old "active" status default
-- are grandfathered as is_active=true so in-flight deliveries are not broken.
-- New endpoints default to is_active=false until verified (BUG-002).
UPDATE webhook_endpoints
SET is_active = true
WHERE status = 'active' AND is_active = false;

-- webhook_deliveries: §12 mandates next_attempt_at, last_response_code,
-- last_response_body, and a status CHECK with 'retrying'.
ALTER TABLE webhook_deliveries
    ADD COLUMN IF NOT EXISTS next_attempt_at timestamptz,
    ADD COLUMN IF NOT EXISTS last_response_code int,
    ADD COLUMN IF NOT EXISTS last_response_body text;

-- Add the CHECK constraint on delivery status. Existing rows have
-- 'pending'/'delivered'/'failed' which are all valid under the new constraint.
ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS webhook_deliveries_status_check;

ALTER TABLE webhook_deliveries
    ADD CONSTRAINT webhook_deliveries_status_check
    CHECK (status IN ('pending', 'delivered', 'failed', 'retrying'));

-- §12 index: idx_wd_retry for the retry sweeper.
CREATE INDEX IF NOT EXISTS idx_wd_retry
    ON webhook_deliveries (next_attempt_at)
    WHERE status = 'retrying';
