-- Reverses 000745_soft_delete_slug_partial_uniques: restores full uniqueness
-- including soft-deleted rows. Fails while a live row and a soft-deleted row
-- share a key — resolve by force-deleting the stale rows first.

BEGIN;

DROP INDEX IF EXISTS idx_spaces_slug_global;
CREATE UNIQUE INDEX idx_spaces_slug_global ON spaces (slug);

DROP INDEX IF EXISTS idx_contents_slug;
CREATE UNIQUE INDEX idx_contents_slug ON contents (slug);

DROP INDEX IF EXISTS idx_contents_content_hash;
CREATE UNIQUE INDEX idx_contents_content_hash
    ON contents (content_hash) WHERE content_hash IS NOT NULL;

DROP INDEX IF EXISTS organizations_slug_key;
CREATE UNIQUE INDEX organizations_slug_key ON organizations (slug);

DROP INDEX IF EXISTS blog_series_slug_key;
CREATE UNIQUE INDEX blog_series_slug_key ON blog_series (slug);

DROP INDEX IF EXISTS university_course_catalog_slug_key;
CREATE UNIQUE INDEX university_course_catalog_slug_key
    ON university_course_catalog (slug);

DROP INDEX IF EXISTS university_course_offerings_slug_key;
CREATE UNIQUE INDEX university_course_offerings_slug_key
    ON university_course_offerings (slug);

DROP INDEX IF EXISTS university_course_offerings_course_id_university_key;
CREATE UNIQUE INDEX university_course_offerings_course_id_university_key
    ON university_course_offerings (course_id, university_id);

DROP INDEX IF EXISTS forum_tags_slug_key;
CREATE UNIQUE INDEX forum_tags_slug_key ON forum_tags (slug);

DROP INDEX IF EXISTS community_organisations_slug_key;
CREATE UNIQUE INDEX community_organisations_slug_key
    ON community_organisations (slug);

COMMIT;
