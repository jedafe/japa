-- 000571_jce_default_comparison_ui_options.up.sql
-- Backfill comparison type config with default page-size and sort options
-- so the JCE frontend no longer needs hardcoded fallback lists.

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'page_size_options', '[12, 24, 48, 96]'::jsonb
)
WHERE config IS NULL OR NOT (config ? 'page_size_options') OR config->'page_size_options' = 'null'::jsonb;

UPDATE comparison_type_definitions
SET config = COALESCE(config, '{}'::jsonb) || jsonb_build_object(
    'sort_options', '["title", "title_desc", "members", "newest"]'::jsonb
)
WHERE config IS NULL OR NOT (config ? 'sort_options') OR config->'sort_options' = 'null'::jsonb;
