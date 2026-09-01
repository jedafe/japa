-- 000584_assessment_geo_content_resolution_setting.up.sql
-- Add the assessment.geo_content_resolution_enabled setting (default true).
-- When enabled, GenerateRoadmap attempts to resolve each curated roadmap step's
-- content_slug to a destination-/stage-/visa-specific content item from the
-- corpus before storing the final roadmap_content.
--
-- Ref: docs/superpowers/plans/2026-08-31-geo-aware-readiness-roadmap.md

BEGIN;

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'assessment.geo_content_resolution_enabled', to_jsonb(true))
ON CONFLICT (tenant_id, key) DO NOTHING;

COMMIT;
