-- 000775_wiki_topic_pages.down.sql
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

ALTER TABLE wiki_type_page_templates
    DROP CONSTRAINT IF EXISTS wiki_page_templates_no_self_parent,
    DROP CONSTRAINT IF EXISTS wiki_page_templates_preview_limit_nonneg;

DROP INDEX IF EXISTS idx_wiki_page_templates_parent;

ALTER TABLE wiki_type_page_templates
    DROP COLUMN IF EXISTS parent_template_id,
    DROP COLUMN IF EXISTS section_label,
    DROP COLUMN IF EXISTS preview_limit;

DELETE FROM comparison_type_definitions WHERE slug = 'city-transportation-prices' AND is_system;

DELETE FROM wiki_type_page_templates
WHERE is_system
  AND slug IN (
    'food-groceries-prices',
    'restaurant-prices',
    'transportation-prices',
    'utilities-prices',
    'lifestyle-prices'
  );

-- The auto-provisioned topic groups + assignments survive (same convention
-- as category/template deletion: orphaned groups are cleaned up in the
-- Admin Panel).

COMMIT;
