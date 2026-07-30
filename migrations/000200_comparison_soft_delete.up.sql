-- 000200_comparison_soft_delete.up.sql
-- Add deleted_at columns to comparison_items and comparison_dimensions
-- so deletes can be soft (Rule 11 — only account:deletion_cleanup may hard-delete).

ALTER TABLE comparison_items ADD COLUMN deleted_at timestamptz;
ALTER TABLE comparison_dimensions ADD COLUMN deleted_at timestamptz;

CREATE INDEX idx_comparison_items_not_deleted ON comparison_items(content_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_comparison_dimensions_not_deleted ON comparison_dimensions(content_id) WHERE deleted_at IS NULL;
