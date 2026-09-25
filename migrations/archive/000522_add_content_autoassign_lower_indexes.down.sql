-- 000522_add_content_autoassign_lower_indexes.down.sql
-- Revert the indexes added for FindSpacesByKeywords auto-assignment.

DROP INDEX IF EXISTS idx_cfv_value_text_lower;
DROP INDEX IF EXISTS idx_spaces_name_lower;
