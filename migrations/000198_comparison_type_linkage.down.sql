-- 000198_comparison_type_linkage.down.sql
DROP INDEX IF EXISTS idx_comparison_dimensions_type;
DROP INDEX IF EXISTS idx_comparison_items_type;
ALTER TABLE comparison_dimensions DROP COLUMN IF EXISTS type_id;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS type_id;
