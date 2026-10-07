-- 000805_booking_tier_coupon_snapshots.down.sql
-- Reverse of 000805 up.
BEGIN;

ALTER TABLE service_bookings
    DROP COLUMN IF EXISTS tier_name,
    DROP COLUMN IF EXISTS tier_price_amount;

COMMIT;
