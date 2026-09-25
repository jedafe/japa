-- 000571_jce_default_comparison_ui_options.down.sql
-- Remove the seeded default page-size and sort options.

UPDATE comparison_type_definitions
SET config = config - 'page_size_options' - 'sort_options'
WHERE config IS NOT NULL;
