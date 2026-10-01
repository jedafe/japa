-- 000765_service_packages_listing_fields.down.sql
BEGIN;

DROP INDEX IF EXISTS idx_service_listing_meta_kind;

ALTER TABLE service_listing_meta
    DROP CONSTRAINT IF EXISTS service_listing_meta_purchase_mode_check,
    DROP CONSTRAINT IF EXISTS service_listing_meta_price_per_check,
    DROP CONSTRAINT IF EXISTS service_listing_meta_listing_kind_check;

ALTER TABLE service_listing_meta
    DROP COLUMN IF EXISTS listing_kind,
    DROP COLUMN IF EXISTS price_amount,
    DROP COLUMN IF EXISTS price_currency,
    DROP COLUMN IF EXISTS price_per,
    DROP COLUMN IF EXISTS purchase_mode,
    DROP COLUMN IF EXISTS capacity,
    DROP COLUMN IF EXISTS available_from,
    DROP COLUMN IF EXISTS available_until;

COMMIT;
