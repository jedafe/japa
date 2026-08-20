-- Revert the backfill by removing junction rows that correspond one-to-one with
-- spaces.parent_id. This preserves any other parent associations added outside
-- this migration.

DELETE FROM space_parent_spaces
WHERE (space_id, parent_space_id) IN (
    SELECT s.id, s.parent_id
    FROM spaces s
    WHERE s.parent_id IS NOT NULL
);
