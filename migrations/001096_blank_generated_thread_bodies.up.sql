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
-- ~150 words from the latest APPROVED revision (same visibility rule as the
-- reader surface); pages without one keep the synthetic fallback.
UPDATE public.thread_anchors ta
SET entity_summary = x.excerpt
FROM wiki_pages wp
LEFT JOIN LATERAL (
    SELECT (CASE
        WHEN array_length(t.words, 1) > 150
            THEN array_to_string(t.words[1:150], ' ') || '…'
        ELSE COALESCE(array_to_string(t.words, ' '), '')
    END)::text AS excerpt
    FROM wiki_revisions wr
    LEFT JOIN LATERAL (
        SELECT regexp_split_to_array(btrim(COALESCE(
            NULLIF(btrim(wr.summary), ''),
            (SELECT string_agg(j.txt #>> '{}', ' ')
             FROM jsonb_path_query(wr.content, '$.**.text ? (@.type() == "string")') AS j(txt)),
            '')), '\s+') AS words
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

-- Re-title wiki_page anchor threads with varied, category-aware lead-ins —
-- the uniform "Discussing: {title}" reads templated at scale. Mirrors
-- anchorThreadTitle's template sets in internal/comments/anchors.go
-- (deterministic per page via hashtext; the two pickers needn't agree).
-- Only generated titles are touched — author-renamed threads keep theirs.
UPDATE public.contents c
SET title = format(
    (CASE
        WHEN wp.category IN ('visa', 'documents', 'admissions', 'scholarships', 'applications')
            THEN (ARRAY['Thread about %s', 'Discussing %s', 'Discussing %s', '%s — Q&A thread'])
        WHEN wp.category IN ('cities', 'states', 'countries', 'overview', 'neighborhoods',
                             'nearby', 'attractions', 'history', 'culture')
            THEN (ARRAY['Discussing %s', 'Thread about %s', '%s — local thread', 'Discussing %s'])
        ELSE (ARRAY['Discussing %s', 'Thread about %s', 'Discussing %s', '%s — community thread'])
    END)[1 + (abs(hashtext(ta.entity_key)::bigint) % 4)],
    wp.title)
FROM thread_anchors ta
JOIN wiki_pages wp ON wp.id::text = ta.entity_key AND wp.deleted_at IS NULL
WHERE ta.entity_type = 'wiki_page'
  AND ta.content_id = c.id
  -- The 'Discussing: ' prefix is the generated signature — covers pages
  -- renamed after minting; author-renamed titles without it are kept.
  AND c.title LIKE 'Discussing: %';

-- Anchor threads minted before EnsureContentScore ran at mint lack
-- content_scores rows — INNER JOIN feed queries (explore/top/rising/
-- space/profile) invisibly skip them. Backfill zero-default rows.
INSERT INTO public.content_scores (content_id)
SELECT ta.content_id
FROM public.thread_anchors ta
WHERE NOT EXISTS (
    SELECT 1 FROM public.content_scores cs
    WHERE cs.content_id = ta.content_id
);
