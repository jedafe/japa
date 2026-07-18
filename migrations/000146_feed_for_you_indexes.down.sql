-- 000146_feed_for_you_indexes.down.sql
-- Manual rollback for the For-You supporting indexes. NOTE: the JapaConnect
-- migrate runner (cmd/migrate) is forward-only and never applies .down.sql
-- files (see AUDIT-BE-003); run this by hand if a rollback is required.

DROP INDEX IF EXISTS idx_content_tags_tag;
DROP INDEX IF EXISTS idx_space_members_user;
DROP INDEX IF EXISTS idx_content_scores_velocity;
DROP INDEX IF EXISTS idx_contents_foryou_window;
