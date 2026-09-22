-- 000715_seed_seo_growth_tools_feature_flags.down.sql
-- Reverses 000715: removes the four SEO growth tool feature flags.

BEGIN;

DELETE FROM feature_flags
WHERE key IN (
    'crs_calculator.enabled',
    'pof_calculator.enabled',
    'eca_guide.enabled',
    'sop_generator.enabled'
);

COMMIT;
