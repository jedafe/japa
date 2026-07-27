-- 000178 down: Remove geo targeting columns from ad_campaigns.

ALTER TABLE ad_campaigns
    DROP COLUMN IF EXISTS target_space_ids,
    DROP COLUMN IF EXISTS target_geo;
