-- 000745_soft_delete_slug_partial_uniques.up.sql
-- Applies the 000744 wiki_pages fix platform-wide: every table that has a
-- deleted_at column AND a non-PK unique index gets that index scoped to
-- live rows (WHERE deleted_at IS NULL). A soft-deleted row is retained for
-- audit/GDPR but no longer occupies its unique keys, so the same slug/hash
-- can be reused by a new live row without force-deleting history.
--
-- Affected tables:
--   spaces                      (idx_spaces_slug_global — slug)
--   contents                    (idx_contents_slug, idx_contents_content_hash)
--   organizations               (organizations_slug_key — slug)
--   blog_series                 (blog_series_slug_key — slug)
--   university_course_catalog   (university_course_catalog_slug_key — slug)
--   university_course_offerings (slug + course_id/university_id composite)
--   forum_tags                  (forum_tags_slug_key — slug; no soft-delete
--                               writer today, scoped preemptively)
--   community_organisations     (community_organisations_slug_key — same)
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md (soft-delete
-- model), Rule 11 (soft deletes only for user content in V1).

BEGIN;

DROP INDEX IF EXISTS idx_spaces_slug_global;
CREATE UNIQUE INDEX idx_spaces_slug_global
    ON spaces (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_contents_slug;
CREATE UNIQUE INDEX idx_contents_slug
    ON contents (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_contents_content_hash;
CREATE UNIQUE INDEX idx_contents_content_hash
    ON contents (content_hash)
    WHERE content_hash IS NOT NULL AND deleted_at IS NULL;

DROP INDEX IF EXISTS organizations_slug_key;
CREATE UNIQUE INDEX organizations_slug_key
    ON organizations (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS blog_series_slug_key;
CREATE UNIQUE INDEX blog_series_slug_key
    ON blog_series (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS university_course_catalog_slug_key;
CREATE UNIQUE INDEX university_course_catalog_slug_key
    ON university_course_catalog (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS university_course_offerings_slug_key;
CREATE UNIQUE INDEX university_course_offerings_slug_key
    ON university_course_offerings (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS university_course_offerings_course_id_university_key;
CREATE UNIQUE INDEX university_course_offerings_course_id_university_key
    ON university_course_offerings (course_id, university_id)
    WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS forum_tags_slug_key;
CREATE UNIQUE INDEX forum_tags_slug_key
    ON forum_tags (slug) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS community_organisations_slug_key;
CREATE UNIQUE INDEX community_organisations_slug_key
    ON community_organisations (slug) WHERE deleted_at IS NULL;

COMMIT;
