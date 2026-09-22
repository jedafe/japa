-- 000716_seed_seo_growth_tools_settings.up.sql
-- system_settings keys for the SEO growth tools (§§107–110):
--   {tool}.subdomain / {tool}.custom_domain — host-detection keys for the
--     Tools surface (ADR-086); empty = no standalone host configured.
--   sop_generator.quota_standard  — monthly generation quota, standard members.
--   sop_generator.prompt_template — AI Gateway prompt template (Rule 3).

BEGIN;

INSERT INTO system_settings (key, value) VALUES
    ('crs_calculator.subdomain', to_jsonb(''::text)),
    ('crs_calculator.custom_domain', to_jsonb(''::text)),
    ('pof_calculator.subdomain', to_jsonb(''::text)),
    ('pof_calculator.custom_domain', to_jsonb(''::text)),
    ('eca_guide.subdomain', to_jsonb(''::text)),
    ('eca_guide.custom_domain', to_jsonb(''::text)),
    ('sop_generator.subdomain', to_jsonb(''::text)),
    ('sop_generator.custom_domain', to_jsonb(''::text)),
    ('sop_generator.quota_standard', to_jsonb(3::int)),
    ('sop_generator.prompt_template', '{}'::jsonb)
ON CONFLICT (key) DO NOTHING;

COMMIT;
