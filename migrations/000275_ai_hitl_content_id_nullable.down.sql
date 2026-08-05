-- Revert: make content_id NOT NULL again. Rows with NULL content_id must be
-- cleaned up first (they correspond to admin-generated reviews without a
-- backing content row).
DELETE FROM ai_hitl_reviews WHERE content_id IS NULL;
ALTER TABLE ai_hitl_reviews ALTER COLUMN content_id SET NOT NULL;
