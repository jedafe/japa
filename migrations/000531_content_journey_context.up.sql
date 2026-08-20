-- Section 32: Activity Feed / Timeline
-- Add journey metadata to contents for origin-destination-aware recommendations.
-- Ref: docs/superpowers/specs/2026-08-21-journey-aware-recommendations-design.md

ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS journey_stage text,
    ADD COLUMN IF NOT EXISTS visa_route text,
    ADD COLUMN IF NOT EXISTS source_country text,
    ADD COLUMN IF NOT EXISTS target_country text;

-- Help the feed queries filter/prioritise by journey axes without scanning all rows.
CREATE INDEX IF NOT EXISTS idx_contents_journey
    ON contents(tenant_id, journey_stage, visa_route)
    WHERE deleted_at IS NULL AND status = 'published' AND visibility = 'public';

CREATE INDEX IF NOT EXISTS idx_contents_target_country
    ON contents(tenant_id, target_country)
    WHERE deleted_at IS NULL AND status = 'published' AND visibility = 'public';

CREATE INDEX IF NOT EXISTS idx_contents_source_target
    ON contents(tenant_id, source_country, target_country)
    WHERE deleted_at IS NULL AND status = 'published' AND visibility = 'public';
