-- 000906_scholarships_slug.up.sql
-- JCE ↔ wiki mapping review fix: scholarships had no slug column, forcing
-- UUID addressing for entity-bound wiki pages. Add slug (unique, NOT NULL
-- once backfilled) derived from title, and point the wiki entity-kind
-- registry at it so /wiki/e/scholarship/{slug}/... addresses work.

BEGIN;

ALTER TABLE scholarships ADD COLUMN IF NOT EXISTS slug text;

-- Backfill: slugify title, dedupe with a short id suffix on collision.
WITH base AS (
    SELECT id,
           lower(regexp_replace(regexp_replace(coalesce(nullif(title,''), 'scholarship'),
                                                '[^a-zA-Z0-9]+', '-', 'g'),
                                '(^-+|-+$)', '', 'g')) AS base_slug
    FROM scholarships
),
ranked AS (
    SELECT b.id,
           CASE WHEN b.base_slug = '' THEN 'scholarship' ELSE b.base_slug END
           || CASE WHEN row_number() OVER (
                       PARTITION BY CASE WHEN b.base_slug = '' THEN 'scholarship' ELSE b.base_slug END
                       ORDER BY b.id) > 1
                   THEN '-' || row_number() OVER (
                       PARTITION BY CASE WHEN b.base_slug = '' THEN 'scholarship' ELSE b.base_slug END
                       ORDER BY b.id)::text
                   ELSE '' END AS slug
    FROM base b
)
UPDATE scholarships s
SET slug = r.slug
FROM ranked r
WHERE s.id = r.id AND s.slug IS NULL;

ALTER TABLE scholarships ALTER COLUMN slug SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS scholarships_slug_key ON scholarships (slug);

-- Registry now has a slug column for this kind.
UPDATE wiki_entity_kinds
SET slug_column = 'slug'
WHERE entity_kind = 'scholarship';

COMMIT;
