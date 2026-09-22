-- 000716_seed_seo_growth_tools_settings.down.sql
-- Reverses 000716: removes the ten SEO growth tool settings keys.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'crs_calculator.subdomain', 'crs_calculator.custom_domain',
    'pof_calculator.subdomain', 'pof_calculator.custom_domain',
    'eca_guide.subdomain', 'eca_guide.custom_domain',
    'sop_generator.subdomain', 'sop_generator.custom_domain',
    'sop_generator.quota_standard', 'sop_generator.prompt_template'
);

COMMIT;
