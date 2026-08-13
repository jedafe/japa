-- 000434_add_trigram_indexes_for_ilike.down.sql
-- Drops the pg_trgm GIN indexes added in the up migration.
-- Does NOT drop the pg_trgm extension (other indexes may depend on it).

DROP INDEX IF EXISTS idx_organizations_slug_trgm;
DROP INDEX IF EXISTS idx_organizations_name_trgm;
DROP INDEX IF EXISTS idx_forum_threads_title_trgm;
DROP INDEX IF EXISTS idx_tags_slug_trgm;
DROP INDEX IF EXISTS idx_tags_name_trgm;
DROP INDEX IF EXISTS idx_users_username_trgm;
DROP INDEX IF EXISTS idx_contents_slug_trgm;
DROP INDEX IF EXISTS idx_contents_title_trgm;
