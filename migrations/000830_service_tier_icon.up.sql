-- 000830_service_tier_icon.up.sql
-- §114 (2026-10-10 amendment): tiers[] applies to every listing kind —
-- service / mentorship / package detail pages all render the tiered
-- pricing cards from the mockups. The card's leading icon chip rides a
-- kebab-case Lucide name resolved against the frontend whitelist (same
-- contract as service_categories.icon / service_listing_meta.*_icons).
BEGIN;

ALTER TABLE service_package_tiers
    ADD COLUMN IF NOT EXISTS icon text;

COMMIT;
