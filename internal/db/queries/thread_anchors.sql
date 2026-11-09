-- Thread anchors — a lazily-created `discussion` contents row bound to a
-- non-content entity (tool page, wiki page) so the §49 comment engine can
-- anchor a thread to it. Ref: 2026-11-08 thread-rename spec.

-- name: GetThreadAnchorByEntity :one
SELECT a.id, a.content_id, a.entity_type, a.entity_key, a.entity_url, a.entity_title, a.entity_summary, a.created_at,
 c.slug AS content_slug, c.title AS content_title, c.comment_count
FROM thread_anchors a
JOIN contents c ON c.id = a.content_id AND c.deleted_at IS NULL
WHERE a.entity_type = @entity_type AND a.entity_key = @entity_key;

-- name: GetThreadAnchorByContentID :one
SELECT a.id, a.content_id, a.entity_type, a.entity_key, a.entity_url, a.entity_title, a.entity_summary, a.created_at
FROM thread_anchors a
WHERE a.content_id = @content_id;

-- name: CreateThreadAnchor :one
INSERT INTO thread_anchors (content_id, entity_type, entity_key, entity_url, entity_title, entity_summary)
VALUES (@content_id, @entity_type, @entity_key, @entity_url, @entity_title, sqlc.narg(entity_summary))

ON CONFLICT (entity_type, entity_key) DO UPDATE
 SET entity_url = EXCLUDED.entity_url,
 entity_title = EXCLUDED.entity_title,
 entity_summary = EXCLUDED.entity_summary
RETURNING id, content_id, entity_type, entity_key, entity_url, entity_title, entity_summary, created_at;

-- name: GetThreadParentByThreadID :one
-- The content a §49 linked-discussion row was started about — used by
-- /thread/{slug} to render the parent embed on discussion threads.
SELECT c.id, c.slug, c.title, c.content_type, c.meta_description, c.body,
 m.storage_bucket AS featured_storage_bucket, m.storage_path AS featured_storage_path
FROM content_thread_links l
JOIN contents c ON c.id = l.content_id AND c.deleted_at IS NULL AND c.status = 'published'
LEFT JOIN media m ON m.id = c.featured_image_id AND m.deleted_at IS NULL
WHERE l.thread_id = @thread_id
ORDER BY l.created_at DESC
LIMIT 1;

-- name: GetWikiPageAnchorInfo :one
-- Display fields for binding a thread anchor to a wiki page: the page's
-- space slug + page slug build the canonical /wiki/{space}/{page} URL, and
-- the latest revision's summary/extracted text supplies the thread's
-- default excerpt. Only public, active spaces may bind a public thread —
-- anchoring a page from a private/inactive space would leak its title
-- via /thread/{slug}.
SELECT wp.id, wp.title, wp.slug AS page_slug, s.slug AS space_slug, s.name AS space_name,
       COALESCE(wp.category, '') AS category,
       COALESCE(lr.excerpt, '') AS excerpt
FROM wiki_pages wp
JOIN spaces s ON s.id = wp.space_id AND s.deleted_at IS NULL
    AND s.visibility = 'public' AND s.status = 'active'
LEFT JOIN LATERAL (
    -- ~150-word opening: revision summary when present, else the text
    -- nodes extracted from the TipTap doc — word-truncated with an
    -- ellipsis, matching the content-thread preview length.
    SELECT (CASE
        WHEN array_length(x.words, 1) > 150
            THEN array_to_string(x.words[1:150], ' ') || '…'
        ELSE COALESCE(array_to_string(x.words, ' '), '')
    END)::text AS excerpt
    FROM wiki_revisions wr
    LEFT JOIN LATERAL (
        SELECT regexp_split_to_array(btrim(COALESCE(
            NULLIF(btrim(wr.summary), ''),
            (SELECT string_agg(t.txt #>> '{}', ' ')
             FROM jsonb_path_query(wr.content, '$.**.text ? (@.type() == "string")') AS t(txt)),
            '')), '\s+') AS words
    ) x ON true
    WHERE wr.wiki_page_id = wp.id AND wr.deleted_at IS NULL
      AND wr.status = 'approved'
    ORDER BY wr.created_at DESC
    LIMIT 1
) lr ON true
WHERE wp.id = @id AND wp.deleted_at IS NULL;
