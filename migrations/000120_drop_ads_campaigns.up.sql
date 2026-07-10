-- AUDIT-BE-055: Drop dead ads_campaigns table (duplicate of real ad_campaigns in 000024)
-- This table was created in 000044_v2_groundwork.sql but has zero code references.
DROP TABLE IF EXISTS ads_campaigns;
