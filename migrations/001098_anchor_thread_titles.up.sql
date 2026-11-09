-- Rename minted anchor threads to the new title spec:
--   content → "{Content Title} (A Thread)"
--   tool    → "{Tool Name} Tool (A Thread)"
-- The exact-match guard only renames rows still carrying the generated
-- "Discussing: {entity}" title — an author-edited title never matches the
-- pattern and is left alone. Content titles come from the live parent row
-- (entity_title on the anchor can drift behind renames).

UPDATE contents c
SET title = COALESCE(p.title, a.entity_title) || ' (A Thread)',
    updated_at = now()
FROM thread_anchors a
LEFT JOIN contents p ON p.id::text = a.entity_key
WHERE a.content_id = c.id
  AND a.entity_type = 'content'
  AND c.deleted_at IS NULL
  AND c.title IN (
      'Discussing: ' || a.entity_title,
      'Discussing: ' || COALESCE(p.title, a.entity_title)
  );

UPDATE contents c
SET title = a.entity_title || ' Tool (A Thread)',
    updated_at = now()
FROM thread_anchors a
WHERE a.content_id = c.id
  AND a.entity_type = 'tool'
  AND c.deleted_at IS NULL
  AND c.title = 'Discussing: ' || a.entity_title;
