-- rag.sql — JapaBot RAG Knowledge Layer queries (Section 69, ADR-069).
-- Covers chunked embedding storage, HNSW cosine retrieval, entity-scoped
-- deletes, and index stats. The content_embeddings table is append-only per
-- ( entity_type, entity_id, chunk_index) — UPSERT replaces chunks
-- on re-index, never duplicates them.
-- Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
-- japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md

-- =====================================================================
-- Upsert (insert-or-replace) a single chunk embedding.
-- The ON CONFLICT clause replaces the chunk_text + embedding for an
-- existing ( entity_type, entity_id, chunk_index) tuple,
-- implementing the ADR-069 "UPSERTS, never raw INSERT" rule.
-- =====================================================================

-- name: UpsertContentEmbedding :one
-- Scoped retrieval (§69 amendment): every chunk row carries the entity's
-- visibility/space_id/author_id so SearchContentEmbeddings can filter by
-- the caller's view without joins. Re-index refreshes scope metadata too.
INSERT INTO content_embeddings (entity_type, entity_id, chunk_index, chunk_text, embedding, language, visibility, space_id, author_id) VALUES (@entity_type, @entity_id, @chunk_index, @chunk_text, @embedding, @language, @visibility, @space_id, @author_id)

ON CONFLICT (entity_type, entity_id, chunk_index)
DO UPDATE SET
 chunk_text = EXCLUDED.chunk_text,
 embedding = EXCLUDED.embedding,
 language = EXCLUDED.language,
 visibility = EXCLUDED.visibility,
 space_id = EXCLUDED.space_id,
 author_id = EXCLUDED.author_id,
 created_at = now()
RETURNING *;

-- =====================================================================
-- Delete all chunks for a specific entity (used on unpublish/delete and
-- by the admin DELETE /admin/rag/entity/:type/:id route).
-- =====================================================================

-- name: DeleteContentEmbeddingsByEntity :exec
DELETE FROM content_embeddings
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- =====================================================================
-- Delete all chunks for every entity of a given type within a platform.
-- Used by the admin by-type aggregate purge (DELETE /admin/rag/entity/:type/all).
-- =====================================================================

-- name: DeleteContentEmbeddingsByType :exec
DELETE FROM content_embeddings
WHERE entity_type = @entity_type;

-- =====================================================================
-- Delete chunks for an entity that exceed the current chunk count.
-- Used after re-indexing with fewer chunks: stale tail chunks must be
-- removed so the table never holds orphaned chunks from a shorter re-index.
-- =====================================================================

-- name: DeleteContentEmbeddingsAboveIndex :exec
DELETE FROM content_embeddings
WHERE entity_type = @entity_type AND entity_id = @entity_id
 AND chunk_index >= @max_index;

-- =====================================================================
-- HNSW cosine similarity retrieval (the core RAG query).
-- Returns the top-k most similar chunks for a query embedding, optionally
-- filtered by language. The <=> operator is cosine distance; we ORDER BY
-- ascending distance and LIMIT to top_k.
-- =====================================================================

-- name: SearchContentEmbeddings :many
-- BUG-001 fix: filter out chunks whose source content is soft-deleted or not
-- published. An EXISTS subquery (only for entity_type = 'content' chunks)
-- ensures that deleted/unpublished/archived/draft content passages are never
-- returned in RAG retrieval. Non-content entity types (forum_thread, space,
-- academy_track) pass through the entity_type != 'content' branch.
-- Ref: ADR-069 §2 — "Content is unpublished or deleted — deletes its
-- content_embeddings rows" implies only published content is retrievable.
--
-- BUG-001 (rag-audit): return the content_type for content entities so the
-- store layer can classify the source tier correctly (editorial vs community).
-- The entity_type column is "content" (generic) for all content entities, not
-- the specific content_type (guide, blog_post, comparison, discussion).
-- ADR-069 §4 requires tier classification by content_type, not entity_type.
--
-- BUG-004 (rag-audit): add status/deleted filters for non-content entity types
-- as defense-in-depth. The forum/spaces/academy handlers purge embeddings on
-- delete, but those are fire-and-forget — a failed purge leaves orphaned
-- embeddings. The CASE expression checks each entity type's status condition.
-- §69 Citation Source Contract: entity_title/entity_slug let the API emit a
-- resolvable {title, slug, url} per source so citations link to the entity.
-- Scalar subqueries only evaluate for emitted rows (post-LIMIT), so the
-- polymorphic CASE costs nothing on filtered-out candidates.
SELECT
 ce.id,
 ce.entity_type,
 ce.entity_id,
 ce.chunk_index,
 ce.chunk_text,
 ce.language,
 ce.created_at,
 (1.0 - (ce.embedding <=> @query_embedding))::float8 AS similarity,
 CASE WHEN ce.entity_type = 'content' THEN
 (SELECT c.content_type FROM contents c
 WHERE c.id = ce.entity_id)
 ELSE NULL END AS content_type,
 CASE ce.entity_type
 WHEN 'content' THEN (SELECT c.title FROM contents c WHERE c.id = ce.entity_id)
 WHEN 'forum_thread' THEN (SELECT ft.title FROM forum_threads ft WHERE ft.id = ce.entity_id)
 WHEN 'space' THEN (SELECT s.name FROM spaces s WHERE s.id = ce.entity_id)
 WHEN 'academy_track' THEN (SELECT lt.title FROM academy_tracks lt WHERE lt.id = ce.entity_id)
 ELSE NULL END AS entity_title,
 CASE ce.entity_type
 WHEN 'content' THEN (SELECT c.slug FROM contents c WHERE c.id = ce.entity_id)
 WHEN 'forum_thread' THEN (SELECT ft.slug FROM forum_threads ft WHERE ft.id = ce.entity_id)
 WHEN 'space' THEN (SELECT s.slug FROM spaces s WHERE s.id = ce.entity_id)
 WHEN 'academy_track' THEN (SELECT lt.slug FROM academy_tracks lt WHERE lt.id = ce.entity_id)
 ELSE NULL END AS entity_slug
FROM content_embeddings ce
WHERE (@language::text = '' OR ce.language = @language)
 AND CASE ce.entity_type
 WHEN 'content' THEN EXISTS (
 SELECT 1 FROM contents c
 WHERE c.id = ce.entity_id
 AND c.status = 'published' AND c.deleted_at IS NULL
 )
 WHEN 'forum_thread' THEN EXISTS (
 SELECT 1 FROM forum_threads ft
 WHERE ft.id = ce.entity_id
 AND ft.deleted_at IS NULL
 )
 WHEN 'space' THEN EXISTS (
 SELECT 1 FROM spaces s
 WHERE s.id = ce.entity_id
 AND s.status = 'active'
 )
 WHEN 'academy_track' THEN EXISTS (
 SELECT 1 FROM academy_tracks lt
 WHERE lt.id = ce.entity_id
 AND lt.is_published = true
 )
 ELSE true
 END
 -- Scoped retrieval (§69 amendment): the caller sees public rows, their own
 -- rows, members rows when authenticated, private rows when privileged, and
 -- private space/forum_thread rows for approved member spaces. Mirrors the
 -- content.visibility contract used by ListContents.
 AND (
 ce.visibility = 'public'
 OR ce.author_id = @viewer_id
 OR (ce.visibility = 'members' AND @viewer_id::uuid IS NOT NULL)
 OR (ce.visibility = 'private' AND @can_see_private::boolean)
 OR (ce.entity_type IN ('space','forum_thread')
 AND ce.visibility = 'private'
 AND ce.space_id = ANY(@member_space_ids::uuid[]))
 )
ORDER BY ce.embedding <=> @query_embedding
LIMIT @top_k;

-- =====================================================================
-- Geo-filtered HNSW cosine retrieval (§103 Geo-Filtered RAG amendment to §69).
-- Same shape as SearchContentEmbeddings but JOINs to contents and restricts
-- to chunks whose source content is geo-scoped to the given space or its
-- direct children. When geo_space_id is NULL the geo filter is a no-op
-- (backward-compatible with §69 unfiltered retrieval). Only entity_type =
-- 'content' chunks are geo-filterable (other entity types have no
-- geo_space_id); non-content chunks are excluded when a geo scope is set.
-- Used by the AI agent's rag.search tool and the agent pre-search step.
-- =====================================================================

-- name: SearchContentEmbeddingsGeoFiltered :many
-- BUG-001 fix: also filter by c.status = 'published' AND c.deleted_at IS NULL
-- so that soft-deleted or unpublished content is never returned in geo-filtered
-- RAG retrieval. The JOIN to contents already exists for the geo filter, so
-- the status/deleted_at conditions are added to the WHERE clause.
--
-- BUG-001 (rag-audit): return c.content_type so the store layer can classify
-- the source tier correctly (editorial vs community). The geo-filtered query
-- already JOINs to contents, so c.content_type is always available.
SELECT
 ce.id,
 ce.entity_type,
 ce.entity_id,
 ce.chunk_index,
 ce.chunk_text,
 ce.language,
 ce.created_at,
 (1.0 - (ce.embedding <=> @query_embedding))::float8 AS similarity,
 c.content_type,
 -- §69 Citation Source Contract: content-only query — title/slug come
 -- straight from the joined contents row.
 c.title AS entity_title,
 c.slug AS entity_slug
FROM content_embeddings ce
JOIN contents c ON c.id = ce.entity_id AND ce.entity_type = 'content'
WHERE (@language::text = '' OR ce.language = @language)
 AND c.status = 'published'
 AND c.deleted_at IS NULL
 AND (
 sqlc.narg(geo_space_id)::uuid IS NULL
 OR c.geo_space_id = sqlc.narg(geo_space_id)
 OR c.geo_space_id IN (SELECT id FROM spaces WHERE parent_id = sqlc.narg(geo_space_id))
 )
 -- Scoped retrieval (§69 amendment): content-only clause — content
 -- visibility is self-contained (no space_membership join for content).
 AND (
 ce.visibility = 'public'
 OR ce.author_id = @viewer_id
 OR (ce.visibility = 'members' AND @viewer_id::uuid IS NOT NULL)
 OR (ce.visibility = 'private' AND @can_see_private::boolean)
 )
ORDER BY ce.embedding <=> @query_embedding
LIMIT @top_k;

-- =====================================================================
-- Index stats for GET /admin/rag/status.
-- =====================================================================

-- name: GetContentEmbeddingStats :one
SELECT
 COUNT(*) AS total_chunks,
 COUNT(DISTINCT (entity_type, entity_id)) AS total_entities,
 MAX(created_at)::timestamptz AS last_indexed_at
FROM content_embeddings
;

-- name: GetContentEmbeddingStatsByType :many
SELECT
 entity_type,
 COUNT(*) AS chunk_count,
 COUNT(DISTINCT entity_id) AS entity_count
FROM content_embeddings

GROUP BY entity_type
ORDER BY entity_type;

-- =====================================================================
-- Count chunks for an entity (used to determine if stale tail cleanup
-- is needed after a re-index with fewer chunks).
-- =====================================================================

-- name: CountContentEmbeddingsByEntity :one
SELECT COUNT(*) AS count
FROM content_embeddings
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- =====================================================================
-- List all published content IDs for a platform (for full corpus re-index).
-- Only 'published' content is indexed in the RAG layer (Section 69).
-- =====================================================================

-- name: ListPublishedContentIDsForReindex :many
-- Scoped retrieval (§69 amendment): returns scope metadata so re-index
-- writes correct visibility/space_id/author_id on each chunk row.
SELECT id, visibility, space_id, author_id FROM contents
WHERE status = 'published'
 AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- List all non-deleted forum thread IDs for a platform (for full corpus
-- re-index). Forum threads are part of the RAG corpus (ADR-069 §2).
-- =====================================================================

-- name: ListForumThreadIDsForReindex :many
-- Scoped retrieval: threads inherit their parent space's visibility.
-- Threads with no space (space deleted, SET NULL) fail closed to private.
SELECT ft.id, COALESCE(sp.visibility, 'private') AS visibility,
 ft.space_id, ft.author_user_id
FROM forum_threads ft
LEFT JOIN spaces sp ON sp.id = ft.space_id
WHERE ft.deleted_at IS NULL
ORDER BY ft.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- List all active space IDs for a platform (for full corpus re-index).
-- Spaces are part of the RAG corpus (ADR-069 §2).
-- =====================================================================

-- name: ListSpaceIDsForReindex :many
-- Scoped retrieval: returns each space's visibility for scope metadata.
SELECT id, visibility FROM spaces
WHERE status = 'active'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- List all published academy track IDs for a platform (for full corpus
-- re-index). Academy tracks are part of the RAG corpus (ADR-069 §2).
-- Only published tracks are indexed; academy_tracks has no deleted_at
-- column (tracks are hard-deleted).
-- =====================================================================

-- name: ListAcademyTrackIDsForReindex :many
SELECT id FROM academy_tracks
WHERE is_published = true
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Scoped retrieval (§69 amendment) — viewer scope + scope maintenance.
-- =====================================================================

-- name: ListMemberSpaceIDs :many
-- Approved memberships only — mirrors CheckSpaceMembership
-- (spaces.sql.go: status = 'approved'). Pending members gain no scope.
SELECT space_id FROM space_members
WHERE user_id = @user_id AND status = 'approved';

-- name: UpdateEmbeddingScope :exec
-- Refresh scope metadata for one entity's chunks (visibility/status
-- transitions, thread space reassignment).
UPDATE content_embeddings
SET visibility = @visibility, space_id = @space_id, author_id = @author_id
WHERE entity_type = @entity_type AND entity_id = @entity_id;

-- name: UpdateThreadEmbeddingsBySpace :exec
-- Bulk scope refresh for all forum_thread rows in a space — used when the
-- space's visibility flips (threads inherit space visibility).
UPDATE content_embeddings
SET visibility = @visibility
WHERE entity_type = 'forum_thread' AND space_id = @space_id;

-- name: GetContentEmbeddingMeta :one
-- Scope metadata + source text for the content embed worker task.
SELECT title, body, status, visibility, space_id, author_id
FROM contents
WHERE id = @id AND deleted_at IS NULL;

-- name: OrphanThreadEmbeddingsBySpace :exec
-- Space deletion: threads survive via ON DELETE SET NULL, but their
-- embedding rows must be orphaned BEFORE the delete commits (the FK makes
-- them unmatchable afterward). Fail closed to private + clear space_id.
UPDATE content_embeddings
SET visibility = 'private', space_id = NULL
WHERE entity_type = 'forum_thread' AND space_id = @space_id;

-- name: ReconcileOrphanThreadEmbeddings :exec
-- Self-healing sweep (§69 amendment): any forum_thread chunk whose source
-- thread lost its space (ON DELETE SET NULL) must fail closed. Runs at the
-- start of reindex so a missed pre-delete reconcile cannot leak.
UPDATE content_embeddings ce
SET visibility = 'private', space_id = NULL
FROM forum_threads ft
WHERE ce.entity_type = 'forum_thread' AND ce.entity_id = ft.id
  AND ft.space_id IS NULL
  AND (ce.visibility <> 'private' OR ce.space_id IS NOT NULL);

-- =====================================================================
-- Index failure records (Task 2: RAG indexing retry + failure visibility).
-- Fire-and-forget index/purge calls retry in the background; when all
-- attempts are exhausted the call site upserts a row here so the admin
-- status route can surface it and the retry route can re-enqueue it.
-- =====================================================================

-- name: UpsertRAGIndexFailure :exec
INSERT INTO rag_index_failures (entity_type, entity_id, operation, error, attempted_at)
VALUES (@entity_type, @entity_id, @operation, @error, now())
ON CONFLICT (entity_type, entity_id, operation)
DO UPDATE SET error = EXCLUDED.error, attempted_at = now();

-- name: ListRAGIndexFailures :many
SELECT entity_type, entity_id, operation, error, attempted_at
FROM rag_index_failures
ORDER BY attempted_at DESC
LIMIT @page_limit;

-- name: CountRAGIndexFailures24h :one
SELECT COUNT(*) AS count
FROM rag_index_failures
WHERE attempted_at >= now() - interval '24 hours';

-- name: ListContentRAGIndexFailures :many
SELECT entity_id
FROM rag_index_failures
WHERE entity_type = 'content' AND operation = 'index';

-- name: DeleteRAGIndexFailures :exec
DELETE FROM rag_index_failures
WHERE (@entity_type::text = '' OR entity_type = @entity_type);

-- name: DeleteRAGIndexFailure :exec
DELETE FROM rag_index_failures
WHERE entity_type = @entity_type AND entity_id = @entity_id AND operation = @operation;

-- name: DeleteNonContentRAGIndexFailures :execrows
DELETE FROM rag_index_failures
WHERE entity_type <> 'content';
