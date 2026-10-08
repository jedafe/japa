-- 000807_unified_rollout.down.sql
-- Revert 'testing' from the roadmap status enum. Fail loudly if rows use it.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM roadmap_items WHERE status = 'testing') THEN
    RAISE EXCEPTION 'cannot revert: roadmap_items rows with status=testing exist';
  END IF;
END $$;

ALTER TABLE roadmap_items DROP CONSTRAINT IF EXISTS roadmap_items_status_check;
ALTER TABLE roadmap_items ADD CONSTRAINT roadmap_items_status_check
  CHECK (status IN ('pending','under_review','planned','in_progress',
                    'shipped','declined','archived'));

-- The feature_flags backfill rows are intentionally NOT removed on down —
-- they represent real operator intent and removing them could re-enable
-- features that were deliberately off.
