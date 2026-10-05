-- 000792_coupon_deal_slugs.down.sql
-- Reverts the coupons.slug column added for /deal/* public URLs (§55).

BEGIN;

DROP INDEX IF EXISTS idx_coupons_slug;

ALTER TABLE coupons DROP COLUMN IF EXISTS slug;

COMMIT;
