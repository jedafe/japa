-- Rename minted wiki anchor threads to the display-label title spec:
--   wiki_page → "{Space Name} {Page Title} (A Thread)"
-- ("Argentina Cost of Living (A Thread)"). {name} tokens in the stored
-- page title expand to the space name first; a title already carrying
-- the space name at a word boundary isn't doubled. The exact-match
-- guard only renames rows still carrying a generated template title —
-- an author-edited title never matches and is left alone. Both the
-- anchor's stored entity_title and the live page title are tried since
-- entity_title can drift behind page renames.

UPDATE contents c
SET title = d.new_title,
    updated_at = now()
FROM thread_anchors a
JOIN wiki_pages wp ON wp.id::text = a.entity_key
JOIN spaces s ON s.id = wp.space_id
CROSS JOIN LATERAL (
    SELECT (
        CASE WHEN (' ' || lower(REPLACE(wp.title, '{name}', s.name)) || ' ')
                  LIKE '% ' || lower(s.name) || ' %'
             THEN REPLACE(wp.title, '{name}', s.name)
             ELSE s.name || ' ' || REPLACE(wp.title, '{name}', s.name)
        END
    ) || ' (A Thread)' AS new_title
) d
WHERE a.content_id = c.id
  AND a.entity_type = 'wiki_page'
  AND c.deleted_at IS NULL
  AND (
      c.title IN (
          'Discussing ' || a.entity_title,
          'Thread about ' || a.entity_title,
          a.entity_title || ' — community thread',
          a.entity_title || ' — Q&A thread',
          a.entity_title || ' — local thread',
          'Discussing ' || wp.title,
          'Thread about ' || wp.title,
          wp.title || ' — community thread',
          wp.title || ' — Q&A thread',
          wp.title || ' — local thread'
      )
      -- Page renamed after mint: the stored title is a generated form of
      -- the OLD page title, so pattern-match the generated shapes too.
      OR c.title LIKE 'Discussing %'
      OR c.title LIKE 'Discussing: %'
      OR c.title LIKE 'Thread about %'
      OR c.title LIKE '% — community thread'
      OR c.title LIKE '% — Q&A thread'
      OR c.title LIKE '% — local thread'
  );

-- Anchors' stored entity_title follows the same display label — the
-- embed card and thread summary read it directly (heal-on-resolve would
-- eventually fix it; this keeps stored state consistent now).
UPDATE thread_anchors a
SET entity_title = d.display_title
FROM wiki_pages wp
JOIN spaces s ON s.id = wp.space_id
CROSS JOIN LATERAL (
    SELECT CASE WHEN (' ' || lower(REPLACE(wp.title, '{name}', s.name)) || ' ')
                  LIKE '% ' || lower(s.name) || ' %'
             THEN REPLACE(wp.title, '{name}', s.name)
             ELSE s.name || ' ' || REPLACE(wp.title, '{name}', s.name)
        END AS display_title
) d
WHERE a.entity_type = 'wiki_page' AND wp.id::text = a.entity_key
  AND a.entity_title <> d.display_title;
