-- Backfill space_parent_spaces for existing spaces whose parent_id is set but
-- whose junction row is missing (e.g. spaces created before the junction table
-- was consistently maintained, or restored from dumps).
-- Ref: docs/superpowers/specs/2026-08-19-space-system-refactor-design.md

INSERT INTO space_parent_spaces (space_id, parent_space_id, created_at)
SELECT s.id, s.parent_id, s.created_at
FROM spaces s
WHERE s.parent_id IS NOT NULL
  AND s.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM space_parent_spaces sps
      WHERE sps.space_id = s.id AND sps.parent_space_id = s.parent_id
  )
ON CONFLICT (space_id, parent_space_id) DO NOTHING;
