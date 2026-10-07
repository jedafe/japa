-- 000805_booking_tier_coupon_snapshots.up.sql
-- Services Marketplace — booking-time tier snapshots (spec §2.4): the
-- tier's display name and price are frozen on the booking row so a later
-- tiers[] replace-set can't rewrite the price a booker actually agreed to.
-- Read queries coalesce the snapshot over the live tier join.
-- Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md
BEGIN;

ALTER TABLE service_bookings
    ADD COLUMN IF NOT EXISTS tier_name text,
    ADD COLUMN IF NOT EXISTS tier_price_amount bigint;

COMMIT;
