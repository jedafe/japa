-- 000434_add_trigram_indexes_for_ilike.up.sql
-- Adds pg_trgm GIN indexes for the most-searched text columns used in ILIKE
-- filters. This makes every existing ILIKE query use an index scan instead
-- of a full table scan — typically 10-100x faster on tables with >1k rows.
-- Zero code changes required: the planner automatically uses these indexes
-- for ILIKE '%query%' predicates.
--
-- Ref: docs/PERFORMANCE_AUDIT.md §4 (Search Offload)

-- Enable the trigram extension (one-time, idempotent).
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Content listing search (ListContentsByTenant, ListBlogPosts, SearchContentsBySpace)
CREATE INDEX IF NOT EXISTS idx_contents_title_trgm ON contents USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_contents_slug_trgm ON contents USING gin (slug gin_trgm_ops);

-- User directory search (ListPublicUsers, admin user search)
CREATE INDEX IF NOT EXISTS idx_users_username_trgm ON users USING gin (username gin_trgm_ops);

-- Tag search (ListTags)
CREATE INDEX IF NOT EXISTS idx_tags_name_trgm ON tags USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_tags_slug_trgm ON tags USING gin (slug gin_trgm_ops);

-- Forum thread search (ListThreads)
CREATE INDEX IF NOT EXISTS idx_forum_threads_title_trgm ON forum_threads USING gin (title gin_trgm_ops);

-- Organization search
CREATE INDEX IF NOT EXISTS idx_organizations_name_trgm ON organizations USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_organizations_slug_trgm ON organizations USING gin (slug gin_trgm_ops);
