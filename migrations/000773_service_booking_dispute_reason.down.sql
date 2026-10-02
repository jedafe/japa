BEGIN;

DELETE FROM notification_types WHERE key = 'service_booking_disputed';

ALTER TABLE service_bookings
    DROP COLUMN IF EXISTS dispute_reason;

COMMIT;
