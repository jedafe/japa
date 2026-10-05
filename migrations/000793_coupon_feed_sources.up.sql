-- 000793_coupon_feed_sources.up.sql
--
-- Deal feed import (§55 "Deal Feed Import"):
--   * coupon_feed_sources — registered external deal-feed endpoints
--     (CouponAPI.org incremental feeds, generic full-snapshot JSON feeds).
--   * coupons.feed_id / coupons.external_id — provenance + idempotent upsert
--     key for imported offers.
--   * coupons.code becomes nullable — feed "deal" offers frequently carry no
--     code; NULLs never collide under coupons_code_key.
--
-- Ref: .jeos/sections/55-coupons-external-affiliates.md (Deal Feed Import)

BEGIN;

CREATE TABLE coupon_feed_sources (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name                text NOT NULL,
    source_type         text NOT NULL
                          CHECK (source_type IN ('couponapi', 'generic')),
    endpoint_url        text NOT NULL,
    api_key_encrypted   text,
    default_provider_id uuid REFERENCES affiliate_providers(id) ON DELETE SET NULL,
    enabled             boolean NOT NULL DEFAULT true,
    last_synced_at      timestamptz,
    last_status         text CHECK (last_status IN ('ok', 'error')),
    last_error          text,
    stats               jsonb,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE coupons
    ADD COLUMN feed_id uuid REFERENCES coupon_feed_sources(id) ON DELETE SET NULL,
    ADD COLUMN external_id text,
    ALTER COLUMN code DROP NOT NULL;

CREATE UNIQUE INDEX idx_coupons_feed_external
    ON coupons (feed_id, external_id)
    WHERE feed_id IS NOT NULL;

CREATE INDEX idx_coupons_feed ON coupons (feed_id)
    WHERE feed_id IS NOT NULL;

COMMIT;
