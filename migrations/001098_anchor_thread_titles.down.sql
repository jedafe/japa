-- Restore the previous generated title format for rows still carrying
-- the new generated titles (same exact-match guard as the up migration).

UPDATE contents c
SET title = 'Discussing: ' || COALESCE(p.title, a.entity_title),
    updated_at = now()
FROM thread_anchors a
LEFT JOIN contents p ON p.id::text = a.entity_key
WHERE a.content_id = c.id
  AND a.entity_type = 'content'
  AND c.deleted_at IS NULL
  AND c.title IN (
      a.entity_title || ' (A Thread)',
      COALESCE(p.title, a.entity_title) || ' (A Thread)'
  );

UPDATE contents c
SET title = 'Discussing: ' || a.entity_title,
    updated_at = now()
FROM thread_anchors a
WHERE a.content_id = c.id
  AND a.entity_type = 'tool'
  AND c.deleted_at IS NULL
  AND c.title = a.entity_title || ' Tool (A Thread)';
