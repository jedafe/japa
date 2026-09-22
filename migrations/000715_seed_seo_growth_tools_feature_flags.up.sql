-- 000715_seed_seo_growth_tools_feature_flags.up.sql
-- Feature flags for the SEO growth tools (§§107–110). All default OFF —
-- enabled per-tool via the Control Panel once content is seeded.

BEGIN;

INSERT INTO feature_flags (key, enabled) VALUES
    ('crs_calculator.enabled', false),
    ('pof_calculator.enabled', false),
    ('eca_guide.enabled', false),
    ('sop_generator.enabled', false)
ON CONFLICT (key) DO NOTHING;

COMMIT;
