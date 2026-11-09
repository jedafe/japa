-- Restore the previous generated title format for rows still carrying
-- the new generated titles (same exact-match guard as the up migration).

UPDATE contents c
SET title = 'Discussing ' || d.display_title,
    updated_at = now()
FROM thread_anchors a
JOIN wiki_pages wp ON wp.id = a.entity_key::uuid
JOIN spaces s ON s.id = wp.space_id
CROSS JOIN LATERAL (
    SELECT (
        CASE WHEN (' ' || lower(REPLACE(wp.title, '{name}', s.name)) || ' ')
                  LIKE '% ' || lower(s.name) || ' %'
             THEN REPLACE(wp.title, '{name}', s.name)
             ELSE s.name || ' ' || REPLACE(wp.title, '{name}', s.name)
        END
    ) AS display_title
) d
WHERE a.content_id = c.id
  AND a.entity_type = 'wiki_page'
  AND c.deleted_at IS NULL
  AND c.title = d.display_title || ' (A Thread)';
