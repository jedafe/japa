-- §14 referential integrity for media referenced by contents rows
-- (header/featured columns, list item media_id, TipTap/markdown/HTML body
-- image URLs, and media-typed custom field values). ON DELETE CASCADE on
-- both FKs matches the product_gallery convention: hard-deleting a media
-- row or a contents row drops the tracking rows with it, so GDPR erasure
-- and admin force-deletes never trip on this table.
CREATE TABLE content_media_fields (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    content_id uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
    media_id   uuid NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    field_key  text NOT NULL,   -- 'header_image' | 'featured_image' | 'item_media' | 'body_block' | custom field key
    position   int  NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(content_id, field_key, position)
);
CREATE INDEX idx_cmf_content ON content_media_fields (content_id);
CREATE INDEX idx_cmf_media ON content_media_fields (media_id);

-- ── Backfill ─────────────────────────────────────────────────────────────

-- Cover columns.
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, c.header_image_id, 'header_image', 0
FROM contents c
WHERE c.header_image_id IS NOT NULL;

INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, c.featured_image_id, 'featured_image', 0
FROM contents c
WHERE c.featured_image_id IS NOT NULL;

-- List items: field_values.items[].media_id, position = item index.
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, (it->>'media_id')::uuid, 'item_media', (t.ord - 1)::int
FROM contents c
CROSS JOIN LATERAL jsonb_array_elements(c.field_values->'items')
     WITH ORDINALITY AS t(it, ord)
WHERE c.content_type = 'list'
  AND jsonb_typeof(c.field_values->'items') = 'array'
  AND it->>'media_id' ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
  AND EXISTS (SELECT 1 FROM media m WHERE m.id = (it->>'media_id')::uuid);

-- Body image references: any /media/serve/{bucket}/{path} URL in body or
-- body_html resolves through storage coordinates to the media row
-- (TipTap image nodes, markdown images, and raw HTML <img> all reduce to
-- the same substring).
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, m.id, 'body_block', (t.ord - 1)::int
FROM contents c
CROSS JOIN LATERAL regexp_matches(
         coalesce(c.body, '') || ' ' || coalesce(c.body_html, ''),
         'media/serve/([a-z0-9_-]+)/([0-9A-Za-z_\-\./%]+)', 'g')
     WITH ORDINALITY AS t(mm, ord)
JOIN media m
  ON m.storage_bucket = mm[1]
 AND m.storage_path   = mm[2];

-- Media-typed custom field values: resolve the field keys applicable to
-- each row's entity classes, then pull every UUID out of that key's stored
-- value (covers bare id, {"value":…}, arrays, {"media_ids":…}, {id,url,alt}).
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT c.id, (ref.mm[1])::uuid, f.key, (ref.ord - 1)::int
FROM contents c
JOIN custom_field_group_assignments a
  ON a.entity_class IN (
       'content_type',
       'content_type:' || c.content_type,
       CASE WHEN c.content_type = 'guide' AND c.guide_category IS NOT NULL
            THEN 'content_type:guide:' || c.guide_category END)
JOIN custom_fields f
  ON f.group_id = a.group_id
 AND f.field_type IN ('image_single', 'image_multi', 'media')
CROSS JOIN LATERAL (
       SELECT mm, ord
       FROM regexp_matches(c.field_values->>f.key,
              '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}',
              'g') WITH ORDINALITY AS r(mm, ord)
     ) ref
WHERE c.field_values ? f.key
  AND EXISTS (SELECT 1 FROM media m WHERE m.id = (ref.mm[1])::uuid)
ON CONFLICT (content_id, field_key, position) DO NOTHING;
