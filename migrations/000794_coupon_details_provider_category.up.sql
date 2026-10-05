-- 000794_coupon_details_provider_category.up.sql
--
-- CostGoat-style deal pages (§55):
--   affiliate_providers.category        — provider category for /deals filters
--   affiliate_providers.organization_id — links a provider to the org that owns
--                                         it; org members submit deals as pending
--   coupons.details                     — JSONB editorial content for the deal page
--   coupons.status + 'pending'          — org submissions awaiting moderation
--
-- Ref: .jeos/sections/55-coupons-external-affiliates.md

BEGIN;

ALTER TABLE affiliate_providers ADD COLUMN category text;
ALTER TABLE affiliate_providers ADD COLUMN organization_id uuid REFERENCES organizations(id);

CREATE UNIQUE INDEX idx_affiliate_providers_org
    ON affiliate_providers (organization_id) WHERE organization_id IS NOT NULL;

ALTER TABLE coupons ADD COLUMN details jsonb;

ALTER TABLE coupons DROP CONSTRAINT coupons_status_check;
ALTER TABLE coupons ADD CONSTRAINT coupons_status_check
    CHECK (status IN ('active', 'expired', 'suspended', 'pending'));

COMMIT;
