-- 000178: Add geo targeting columns to ad_campaigns (Section 25).
-- target_geo: array of ISO country codes for geo-targeted ad serving.
-- target_space_ids: array of specific space UUIDs for fine-grained targeting.

ALTER TABLE ad_campaigns
    ADD COLUMN IF NOT EXISTS target_geo text[] DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS target_space_ids uuid[] DEFAULT '{}';

COMMENT ON COLUMN ad_campaigns.target_geo IS 'ISO country codes for geo-targeted serving (e.g., {NG,CA,GB}). Empty = all countries.';
COMMENT ON COLUMN ad_campaigns.target_space_ids IS 'Specific space UUIDs for fine-grained targeting. Empty = all spaces.';
