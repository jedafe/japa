DROP INDEX IF EXISTS idx_service_bookings_reminder_due;

ALTER TABLE service_bookings
    DROP COLUMN IF EXISTS reminded_at;
