-- Reverse migration 000361: remove §12 conformance columns.

DROP INDEX IF EXISTS idx_wd_retry;

ALTER TABLE webhook_deliveries
    DROP CONSTRAINT IF EXISTS webhook_deliveries_status_check;

ALTER TABLE webhook_deliveries
    DROP COLUMN IF EXISTS last_response_body,
    DROP COLUMN IF EXISTS last_response_code,
    DROP COLUMN IF EXISTS next_attempt_at;

ALTER TABLE webhook_endpoints
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS verified_at,
    DROP COLUMN IF EXISTS is_active,
    DROP COLUMN IF EXISTS name;
