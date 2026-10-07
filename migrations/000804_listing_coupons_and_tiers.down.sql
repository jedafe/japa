-- 000804_listing_coupons_and_tiers.down.sql
-- Reverse of 000804 up, in reverse order.
BEGIN;

-- settings
DELETE FROM system_settings WHERE key IN (
    'service_packages.coupons.max_per_listing',
    'service_packages.tiers.max_count'
);

-- booking snapshots
ALTER TABLE service_bookings
    DROP COLUMN IF EXISTS discount_amount,
    DROP COLUMN IF EXISTS coupon_code,
    DROP COLUMN IF EXISTS coupon_id,
    DROP COLUMN IF EXISTS tier_id;

-- package pricing tiers (drops idx_tiers_meta with the table)
DROP TABLE IF EXISTS service_package_tiers;

-- authoritative redemption record (drops idx_redemptions_* with the table)
DROP TABLE IF EXISTS coupon_redemptions;

-- listing ↔ coupon attach (drops idx_slc_listing with the table)
DROP TABLE IF EXISTS service_listing_coupons;

-- coupons: org ownership + redemption rule set
DROP INDEX IF EXISTS idx_coupons_org;
ALTER TABLE coupons
    DROP COLUMN IF EXISTS applies_to,
    DROP COLUMN IF EXISTS first_time_only,
    DROP COLUMN IF EXISTS min_spend,
    DROP COLUMN IF EXISTS per_user_limit,
    DROP COLUMN IF EXISTS max_redemptions,
    DROP COLUMN IF EXISTS organization_id;

COMMIT;
