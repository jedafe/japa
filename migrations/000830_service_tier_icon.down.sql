-- 000830_service_tier_icon.down.sql
-- Reverse of 000830 up.
BEGIN;

ALTER TABLE service_package_tiers
    DROP COLUMN IF EXISTS icon;

COMMIT;
