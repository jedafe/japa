-- 000798_roadmap_shadow_backfill.up.sql
-- Backfills the §113 shadow contents row (content_type='roadmap') for public
-- roadmap items that predate the ensure-on-publish path — notably the
-- 'spaces-content-creation' flagship seeded by 000796 — so their §49
-- comments thread works without waiting for an admin edit.
--
-- contents.author_id is NOT NULL: use submitted_by, then approved_by, then
-- the oldest user account. Items with no resolvable author are skipped.
-- content_hash approximates the Rule 12 normalization in SQL (seeded bodies
-- are plain text — no markdown stripping required). Slug collisions are
-- skipped; the code path retries with numeric suffixes on the next save.
--
-- Ref: .jeos/sections/113-public-product-roadmap.md
-- Ref: .jeos/adr/ADR-127-public-product-roadmap.md

BEGIN;

INSERT INTO contents (
    author_id, content_type, title, slug, body, body_html,
    field_values, content_hash, seo_indexable, status, visibility,
    read_time_minutes, published_at
)
SELECT
    COALESCE(i.submitted_by, i.approved_by, (SELECT id FROM users ORDER BY created_at ASC LIMIT 1)),
    'roadmap',
    i.title,
    'roadmap-' || i.slug,
    COALESCE(i.body, ''),
    '',
    '{}'::jsonb,
    encode(sha256(lower(regexp_replace(btrim(COALESCE(i.body, '')), '\s+', ' ', 'g'))::bytea), 'hex'),
    false,
    'published',
    'public',
    1,
    now()
FROM roadmap_items i
WHERE i.content_id IS NULL
  AND i.deleted_at IS NULL
  AND i.status IN ('under_review', 'planned', 'in_progress', 'shipped')
  AND NOT EXISTS (SELECT 1 FROM contents c WHERE c.slug = 'roadmap-' || i.slug)
  AND COALESCE(i.submitted_by, i.approved_by, (SELECT id FROM users ORDER BY created_at ASC LIMIT 1)) IS NOT NULL;

UPDATE roadmap_items i
SET content_id = c.id, updated_at = now()
FROM contents c
WHERE c.content_type = 'roadmap'
  AND c.slug = 'roadmap-' || i.slug
  AND i.content_id IS NULL;

COMMIT;
