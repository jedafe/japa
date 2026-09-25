-- 000474_comparison_items_content_id_nullable.up.sql
-- Make comparison_items.content_id and comparison_dimensions.content_id
-- nullable so the Intelligence cost_of_living promoter can create
-- projection rows WITHOUT a backing contents row (Approach B).
--
-- Under Approach B (ADR-076 Amendment 1):
--   1. Intelligence extracts cost data → writes comparison_items with
--      content_id=NULL → sets intel_records.status='extracted'
--   2. Admin clicks "Promote to Studio" → Studio generates an editorial
--      comparison article (contents row)
--   3. The editorial contents row is linked back to the projection via
--      guide_id on comparison_items (added by migration 000468) or by
--      updating content_id after Studio publishes
--
-- The ON DELETE CASCADE constraint is preserved; a NULL content_id simply
-- means the projection row exists without an editorial article yet.

BEGIN;

ALTER TABLE comparison_items
    ALTER COLUMN content_id DROP NOT NULL;

ALTER TABLE comparison_dimensions
    ALTER COLUMN content_id DROP NOT NULL;

-- Update the existing index to be a partial index (only non-null content_id)
DROP INDEX IF EXISTS idx_comparison_items_content;
CREATE INDEX idx_comparison_items_content
    ON comparison_items(content_id) WHERE content_id IS NOT NULL;

DROP INDEX IF EXISTS idx_comparison_dimensions_content;
CREATE INDEX idx_comparison_dimensions_content
    ON comparison_dimensions(content_id) WHERE content_id IS NOT NULL;

COMMIT;
