BEGIN;

DROP INDEX IF EXISTS idx_comparison_items_item_ref;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS item_ref;
ALTER TABLE comparison_type_definitions DROP COLUMN IF EXISTS config;
DROP INDEX IF EXISTS idx_comparison_content_metadata_promoted_from;
DROP INDEX IF EXISTS idx_comparison_content_metadata_tenant_depth;
DROP TABLE IF EXISTS comparison_content_metadata;

COMMIT;
