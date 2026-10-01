-- 000765_service_packages_listing_fields.up.sql
-- §111 / ADR-123 — extends service_listing_meta with package + pricing
-- fields. All columns are additive with defaults → zero downtime.
-- Money columns are bigint minor units, matching payments.amount.
--
-- Ref: .jeos/sections/111-service-packages-bookings.md

BEGIN;

ALTER TABLE service_listing_meta
    ADD COLUMN listing_kind text NOT NULL DEFAULT 'service',
    ADD COLUMN price_amount bigint,
    ADD COLUMN price_currency text,
    ADD COLUMN price_per text,
    ADD COLUMN purchase_mode text NOT NULL DEFAULT 'inquiry',
    ADD COLUMN capacity int,
    ADD COLUMN available_from date,
    ADD COLUMN available_until date;

ALTER TABLE service_listing_meta
    ADD CONSTRAINT service_listing_meta_listing_kind_check
        CHECK (listing_kind IN ('service','package')),
    ADD CONSTRAINT service_listing_meta_price_per_check
        CHECK (price_per IN ('person','package','group')),
    -- 'full' is added to this enum when agent payout infrastructure lands (§111 Non-Goals)
    ADD CONSTRAINT service_listing_meta_purchase_mode_check
        CHECK (purchase_mode IN ('inquiry','external','paid'));

-- Filter support for GET /services?kind=package and admin queue kind filters.
CREATE INDEX IF NOT EXISTS idx_service_listing_meta_kind
    ON service_listing_meta (listing_kind)
    WHERE listing_kind = 'package';

COMMIT;
