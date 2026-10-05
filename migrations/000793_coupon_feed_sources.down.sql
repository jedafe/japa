-- 000793_coupon_feed_sources.down.sql

BEGIN;

DROP INDEX IF EXISTS idx_coupons_feed;
DROP INDEX IF EXISTS idx_coupons_feed_external;

-- Restore NOT NULL on code: feed-imported coupons without a code get '' first
-- so the constraint can be re-applied.
UPDATE coupons SET code = '' WHERE code IS NULL;

ALTER TABLE coupons
    ALTER COLUMN code SET NOT NULL,
    DROP COLUMN external_id,
    DROP COLUMN feed_id;

DROP TABLE IF EXISTS coupon_feed_sources;

COMMIT;
