-- Down migration for 000531_content_journey_context

DROP INDEX IF EXISTS idx_contents_source_target;
DROP INDEX IF EXISTS idx_contents_target_country;
DROP INDEX IF EXISTS idx_contents_journey;

ALTER TABLE contents
    DROP COLUMN IF EXISTS target_country,
    DROP COLUMN IF EXISTS source_country,
    DROP COLUMN IF EXISTS visa_route,
    DROP COLUMN IF EXISTS journey_stage;
