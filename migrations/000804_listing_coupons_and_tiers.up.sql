-- 000804_listing_coupons_and_tiers.up.sql
-- Services Marketplace — org-owned coupons with redemption rules, listing ↔
-- coupon attachment, authoritative redemption records, and package pricing
-- tiers (with booking-time snapshots on service_bookings).
-- Ref: docs/superpowers/specs/2026-10-07-services-marketplace-upgrade-design.md
BEGIN;

-- coupons: org ownership + redemption rule set (all nullable/defaulted → zero downtime)
ALTER TABLE coupons
    ADD COLUMN IF NOT EXISTS organization_id uuid REFERENCES organizations(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS max_redemptions int,
    ADD COLUMN IF NOT EXISTS per_user_limit int,
    ADD COLUMN IF NOT EXISTS min_spend bigint,          -- minor units
    ADD COLUMN IF NOT EXISTS first_time_only boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS applies_to text NOT NULL DEFAULT 'both'
        CHECK (applies_to IN ('service','package','both'));

CREATE INDEX IF NOT EXISTS idx_coupons_org
    ON coupons (organization_id, status) WHERE organization_id IS NOT NULL;

-- listing ↔ coupon attach (ordered, per-listing overrides)
CREATE TABLE IF NOT EXISTS service_listing_coupons (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id       uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    listing_meta_id uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    position        int  NOT NULL DEFAULT 0,
    overrides       jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (coupon_id, listing_meta_id)
);
CREATE INDEX IF NOT EXISTS idx_slc_listing ON service_listing_coupons (listing_meta_id);

-- authoritative redemption record (counts exclude dead bookings — see query)
CREATE TABLE IF NOT EXISTS coupon_redemptions (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id          uuid NOT NULL REFERENCES coupons(id) ON DELETE CASCADE,
    user_id            uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    service_booking_id uuid REFERENCES service_bookings(id) ON DELETE SET NULL,
    listing_meta_id    uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    amount_saved       bigint NOT NULL DEFAULT 0,
    currency           text,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_redemptions_coupon ON coupon_redemptions (coupon_id);
CREATE INDEX IF NOT EXISTS idx_redemptions_user   ON coupon_redemptions (coupon_id, user_id);

-- package pricing tiers
CREATE TABLE IF NOT EXISTS service_package_tiers (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    listing_meta_id uuid NOT NULL REFERENCES service_listing_meta(id) ON DELETE CASCADE,
    name            text NOT NULL,
    badge           text,
    description     text,
    price_amount    bigint NOT NULL,
    price_currency  text,
    features        text[] NOT NULL DEFAULT '{}',
    position        int  NOT NULL DEFAULT 0,
    is_featured     bool NOT NULL DEFAULT false,
    is_active       bool NOT NULL DEFAULT true,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_tiers_meta ON service_package_tiers (listing_meta_id, position);

-- booking snapshots
ALTER TABLE service_bookings
    ADD COLUMN IF NOT EXISTS tier_id uuid REFERENCES service_package_tiers(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS coupon_id uuid REFERENCES coupons(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS coupon_code text,
    ADD COLUMN IF NOT EXISTS discount_amount bigint;

-- settings (code defaults identical)
INSERT INTO system_settings (key, value) VALUES
    ('service_packages.coupons.max_per_listing', to_jsonb(3)),
    ('service_packages.tiers.max_count',         to_jsonb(5))
ON CONFLICT (key) DO NOTHING;

COMMIT;
