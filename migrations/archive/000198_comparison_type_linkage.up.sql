-- 000198_comparison_type_linkage.up.sql
-- Add type_id linkage from comparison_items/dimensions to comparison_type_definitions
-- so a comparison knows which type it was created against (Section 30, BUG-003).
-- Both columns are nullable for back-compat with existing rows.

ALTER TABLE comparison_items
    ADD COLUMN type_id uuid REFERENCES comparison_type_definitions(id) ON DELETE SET NULL;

ALTER TABLE comparison_dimensions
    ADD COLUMN type_id uuid REFERENCES comparison_type_definitions(id) ON DELETE SET NULL;

CREATE INDEX idx_comparison_items_type ON comparison_items(type_id) WHERE type_id IS NOT NULL;
CREATE INDEX idx_comparison_dimensions_type ON comparison_dimensions(type_id) WHERE type_id IS NOT NULL;
