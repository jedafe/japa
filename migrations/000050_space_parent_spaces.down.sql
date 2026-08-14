-- Down migration for 000050_space_parent_spaces.sql
-- Drops the space_parent_spaces junction table for space hierarchy.
-- Note: the migrated data from spaces.parent_id is not restored;
-- the parent_id column was retained in the up migration for backward compatibility.
DROP TABLE IF EXISTS space_parent_spaces;
