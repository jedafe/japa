-- 000567_drop_bookmarks_entity_type_check.down.sql
-- Restoring a restrictive CHECK would risk rejecting rows inserted while the
-- constraint was dropped, so the down migration is intentionally a no-op.
SELECT 1;
