-- 000474_comparison_items_content_id_nullable.down.sql
BEGIN;

-- Restore the original non-partial indexes
DROP INDEX IF EXISTS idx_comparison_items_content;
CREATE INDEX idx_comparison_items_content ON comparison_items(content_id);

DROP INDEX IF EXISTS idx_comparison_dimensions_content;
CREATE INDEX idx_comparison_dimensions_content ON comparison_dimensions(content_id);

-- Restore NOT NULL (will fail if any NULL content_id rows exist —
-- those must be backfilled or deleted before this down migration can run)
ALTER TABLE comparison_dimensions
    ALTER COLUMN content_id SET NOT NULL;

ALTER TABLE comparison_items
    ALTER COLUMN content_id SET NOT NULL;

COMMIT;
