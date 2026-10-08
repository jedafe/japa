-- 000808_roadmap_flag_key_unique.up.sql
-- Ref: docs/superpowers/specs/2026-10-07-unified-feature-rollout-design.md §4.2
--
-- One flag → at most one linked roadmap item. Without this index two items
-- could share a feature_flag_key; GetRoadmapItemByFlagKey (:one) would then
-- reconcile only one while the other stayed locked by the status_derived
-- check yet never synced. Partial: many items have NULL key, and
-- soft-deleted rows must not block relinking the flag to a new item.
CREATE UNIQUE INDEX IF NOT EXISTS roadmap_items_feature_flag_key_uniq
  ON roadmap_items (feature_flag_key)
  WHERE feature_flag_key IS NOT NULL AND deleted_at IS NULL;
