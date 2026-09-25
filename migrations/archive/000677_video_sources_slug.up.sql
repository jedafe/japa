BEGIN;

-- 000677_video_sources_slug.up.sql
-- §106 — /video/{slug} is the canonical public URL for a video_sources row
-- (source-directory items and profile video cards link in-app). Adds a
-- unique slug column and backfills existing rows from their titles with a
-- numeric suffix on collisions (same scheme as contents.slug).
--
-- Ref: japaconnect-engineering/sections/106-video-content-and-import.md

ALTER TABLE video_sources ADD COLUMN slug text;

WITH ranked AS (
  SELECT id,
         COALESCE(
           NULLIF(btrim(regexp_replace(lower(title), '[^a-z0-9]+', '-', 'g'), '-'), ''),
           'video'
         ) AS base,
         ROW_NUMBER() OVER (
           PARTITION BY COALESCE(
             NULLIF(btrim(regexp_replace(lower(title), '[^a-z0-9]+', '-', 'g'), '-'), ''),
             'video'
           )
           ORDER BY created_at, id
         ) AS rn
  FROM video_sources
)
UPDATE video_sources vs
SET slug = CASE WHEN r.rn = 1 THEN r.base ELSE r.base || '-' || r.rn END
FROM ranked r
WHERE r.id = vs.id;

ALTER TABLE video_sources ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS video_sources_slug_key ON video_sources (slug);

COMMIT;
