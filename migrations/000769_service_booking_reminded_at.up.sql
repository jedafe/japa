-- 000769 — §111 SLA reminder marker on service_bookings.
-- The hourly booking_expire_pending sweep sends service_booking_reminder
-- at half the org-response window; reminded_at makes the nudge one-shot
-- instead of re-firing on every sweep pass.
ALTER TABLE service_bookings
    ADD COLUMN reminded_at timestamptz;

CREATE INDEX idx_service_bookings_reminder_due
    ON service_bookings (expires_at)
    WHERE status = 'pending' AND reminded_at IS NULL;
