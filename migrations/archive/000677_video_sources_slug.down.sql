BEGIN;

-- 000677_video_sources_slug.down.sql — drop the slug column + index.

DROP INDEX IF EXISTS video_sources_slug_key;
ALTER TABLE video_sources DROP COLUMN IF EXISTS slug;

COMMIT;
