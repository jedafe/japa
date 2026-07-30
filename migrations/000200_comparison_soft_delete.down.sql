-- 000200_comparison_soft_delete.down.sql
DROP INDEX IF EXISTS idx_comparison_dimensions_not_deleted;
DROP INDEX IF EXISTS idx_comparison_items_not_deleted;
ALTER TABLE comparison_dimensions DROP COLUMN IF EXISTS deleted_at;
ALTER TABLE comparison_items DROP COLUMN IF EXISTS deleted_at;
