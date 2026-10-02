-- 000773 — §111 booker disputes: dispute_reason column on
-- service_bookings + the org-facing service_booking_disputed
-- notification type seed.
--
-- Ref: .jeos/sections/111-service-packages-bookings.md

BEGIN;

ALTER TABLE service_bookings
    ADD COLUMN dispute_reason text;

INSERT INTO notification_types (key, label, description, default_in_app, default_email, default_push, category) VALUES
    ('service_booking_disputed', 'Booking disputed', 'A booker reported a problem with a booking', true, true, false, 'services')
ON CONFLICT (key) DO NOTHING;

COMMIT;
