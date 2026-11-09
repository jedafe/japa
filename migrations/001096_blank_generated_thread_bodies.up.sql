-- 001096 — anchored thread bodies start empty (unified-discussion spec):
-- the entity excerpt (parent-article preview / embed card) supplies the
-- default context and the author adds their own text via inline edit —
-- no generated prose in the post body.
--
-- Blank only UNTOUCHED generated bodies: the row still carries the mint
-- prompt in `body` AND its `body_html` is one of the generated shapes —
-- '' (SQL-backfilled), a literal-JSON render (the pre-fix corruption), or
-- the two-paragraph mint output. Every author edit rewrites body_html, so
-- authored text survives even when it happens to quote the prompt.
UPDATE public.contents c
SET body = '', body_html = ''
FROM thread_anchors ta
WHERE ta.content_id = c.id
  AND c.body LIKE '%join the discussion below%'
  AND (
      c.body_html = ''
      OR c.body_html LIKE '%&#34;%'
      OR c.body_html ~ '^<p>[^<]*</p><p>Questions, experiences, and tips about [^<]* — join the discussion below\.</p>$'
  );

-- Give wiki_page anchors the page's real excerpt as entity_summary — minted
-- anchors carried a synthetic "Discussing a page in the X wiki." sentence.
-- Latest APPROVED revision only (same visibility rule as the reader
-- surface); pages without one keep the synthetic fallback.
UPDATE public.thread_anchors ta
SET entity_summary = x.excerpt
FROM wiki_pages wp
LEFT JOIN LATERAL (
    SELECT COALESCE(wr.summary, left(btrim(COALESCE(t.excerpt_text, '')), 200), '') AS excerpt
    FROM wiki_revisions wr
    LEFT JOIN LATERAL (
        SELECT string_agg(j.txt #>> '{}', ' ') AS excerpt_text
        FROM jsonb_path_query(wr.content, '$.**.text ? (@.type() == "string")') AS j(txt)
    ) t ON true
    WHERE wr.wiki_page_id = wp.id AND wr.deleted_at IS NULL
      AND wr.status = 'approved'
    ORDER BY wr.created_at DESC
    LIMIT 1
) x ON true
WHERE ta.entity_type = 'wiki_page'
  AND wp.id::text = ta.entity_key
  AND wp.deleted_at IS NULL
  AND NULLIF(btrim(x.excerpt), '') IS NOT NULL;
