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

-- name: ListWikiAnchorsBySpace :many
-- Anchors bound to a space's wiki pages — used when the space leaves
-- public/active so their public threads can be dropped (same leak gate as
-- GetWikiPageAnchorInfo's JOIN). Live threads only.
SELECT a.entity_key
FROM thread_anchors a
JOIN wiki_pages wp ON wp.id = a.entity_key::uuid
JOIN contents c ON c.id = a.content_id AND c.deleted_at IS NULL
WHERE a.entity_type = 'wiki_page' AND wp.space_id = @space_id;

-- name: ListContentAnchorsBySpace :many
-- Content anchors whose thread row files under a space — the mint copies
-- the parent's primary space onto the thread row, so a closing space takes
-- its hosted threads down too (same public+active gate as describeEntity).
SELECT a.entity_key
FROM thread_anchors a
JOIN contents c ON c.id = a.content_id AND c.deleted_at IS NULL
WHERE a.entity_type = 'content' AND c.space_id = @space_id;

-- name: ListAnchorKeysByType :many
-- Live entity_key list for one anchor class — the flag-off reconcile walks
-- these and drops each via DeleteAnchor so the full delete side-effects
-- (search-index sync, webhooks, edge purge) fire per thread.
SELECT a.entity_key
FROM thread_anchors a
JOIN contents c ON c.id = a.content_id AND c.deleted_at IS NULL
WHERE a.entity_type = @entity_type;

-- name: ListContentAnchorKeysByParentType :many
-- Content anchors whose PARENT row carries a gated content_type — a
-- disabled type flag (jobs.enabled, video.enabled, …) hides every page of
-- that type, so their public threads must die with it. entity_key list so
-- each drop runs the full DeleteAnchor side-effects.
SELECT a.entity_key
FROM thread_anchors a
JOIN contents c ON c.id = a.content_id AND c.deleted_at IS NULL
JOIN contents p ON p.id::text = a.entity_key
WHERE a.entity_type = 'content'
  AND p.content_type = @content_type;

-- name: CreateThreadAnchor :one
INSERT INTO thread_anchors (content_id, entity_type, entity_key, entity_url, entity_title, entity_summary)
VALUES (@content_id, @entity_type, @entity_key, @entity_url, @entity_title, sqlc.narg(entity_summary))

ON CONFLICT (entity_type, entity_key) DO UPDATE
 SET entity_url = EXCLUDED.entity_url,
 entity_title = EXCLUDED.entity_title,
 entity_summary = EXCLUDED.entity_summary,
 -- Rebind only when the anchor points at a dead thread (deleted content
 -- row — e.g. un-published entity or a deleted+recreated wiki page). A
 -- live binding keeps the existing thread so the get-or-create race still
 -- detects the winner via anchor.ContentID != row.ID.
 content_id = CASE
     WHEN EXISTS (
         SELECT 1 FROM contents
         WHERE id = thread_anchors.content_id AND deleted_at IS NULL
     ) THEN thread_anchors.content_id
     ELSE EXCLUDED.content_id
 END
RETURNING id, content_id, entity_type, entity_key, entity_url, entity_title, entity_summary, created_at;

-- name: SetThreadRowSpaceID :exec
-- Re-home a minted thread row to its parent's current primary space —
-- a public→public re-file (UpdateContentSpaces) otherwise leaves the
-- thread filed under the stale space. space_id only: geo_space_id follows
-- the parent's geo space, not the primary.
UPDATE contents SET space_id = @space_id, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: SetThreadRowTitle :exec
-- Retitle a minted thread row whose entity renamed since mint — applied
-- only when the stored title still carries a generated form (the " (A
-- Thread)" suffix or a legacy "Discussing …" template), so author-edited
-- titles are never clobbered.
UPDATE contents SET title = @title, updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: GetThreadParentByThreadID :one
-- The content a §49 linked-discussion row was started about — used by
-- /thread/{slug} to render the parent embed on discussion threads.
SELECT c.id, c.slug, c.title, c.content_type, c.meta_description, c.body,
 m.storage_bucket AS featured_storage_bucket, m.storage_path AS featured_storage_path
FROM content_thread_links l
JOIN contents c ON c.id = l.content_id AND c.deleted_at IS NULL AND c.status = 'published'
 AND c.visibility = 'public' AND c.auto_hidden = false
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
