-- 000750_wiki_page_templates_rollup.down.sql
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

ALTER TABLE wiki_type_categories DROP COLUMN IF EXISTS rollup;
ALTER TABLE custom_fields DROP COLUMN IF EXISTS section;
ALTER TABLE wiki_pages
    DROP COLUMN IF EXISTS page_template_id,
    DROP COLUMN IF EXISTS field_group_id,
    DROP COLUMN IF EXISTS position;

DROP TABLE IF EXISTS wiki_type_page_templates;

DELETE FROM comparison_type_definitions WHERE slug = 'city-prices' AND is_system;

UPDATE system_settings
SET value = value - 'internship' - 'work_career',
    updated_at = now()
WHERE key = 'intel.category_method_map';

-- The auto-provisioned group + assignment survive (same convention as
-- category deletion: orphaned groups are cleaned up in the Admin Panel).

COMMIT;
