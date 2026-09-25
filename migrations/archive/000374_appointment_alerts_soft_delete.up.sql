-- 000374_appointment_alerts_soft_delete.up.sql
-- Add soft-delete support to appointment_alerts (Rule 11 — no hard-delete of
-- user content in V1 app code; only account:deletion_cleanup may hard-delete).
-- Ref: japaconnect-engineering/sections/91-embassy-appointment-alerter.md
--      japaconnect-engineering/AGENTS.md Rule 11

BEGIN;

ALTER TABLE appointment_alerts
    ADD COLUMN IF NOT EXISTS deleted_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_appointment_alerts_deleted_at
    ON appointment_alerts(deleted_at);

COMMIT;
