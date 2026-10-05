-- 000794_coupon_details_provider_category.down.sql

BEGIN;

DELETE FROM coupons WHERE status = 'pending';

ALTER TABLE coupons DROP CONSTRAINT coupons_status_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_status_check
    CHECK (status IN ('active', 'expired', 'suspended'));

ALTER TABLE coupons DROP COLUMN details;

DROP INDEX IF EXISTS idx_affiliate_providers_org;
ALTER TABLE affiliate_providers DROP COLUMN organization_id;
ALTER TABLE affiliate_providers DROP COLUMN category;

COMMIT;
