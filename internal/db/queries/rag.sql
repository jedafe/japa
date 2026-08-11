-- rag.sql — JapaBot RAG Knowledge Layer queries (Section 69, ADR-069).
-- Covers chunked embedding storage, HNSW cosine retrieval, entity-scoped
-- deletes, and index stats. The content_embeddings table is append-only per
-- (tenant_id, entity_type, entity_id, chunk_index) — UPSERT replaces chunks
-- on re-index, never duplicates them.
-- Ref: japaconnect-engineering/sections/69-rag-knowledge-layer.md
--      japaconnect-engineering/adr/ADR-069-japabot-rag-knowledge-layer.md

-- =====================================================================
-- Upsert (insert-or-replace) a single chunk embedding.
-- The ON CONFLICT clause replaces the chunk_text + embedding for an
-- existing (tenant_id, entity_type, entity_id, chunk_index) tuple,
-- implementing the ADR-069 "UPSERTS, never raw INSERT" rule.
-- =====================================================================

-- name: UpsertContentEmbedding :one
INSERT INTO content_embeddings (
    tenant_id, entity_type, entity_id, chunk_index, chunk_text, embedding, language
)
VALUES (@tenant_id, @entity_type, @entity_id, @chunk_index, @chunk_text, @embedding, @language)
ON CONFLICT (tenant_id, entity_type, entity_id, chunk_index)
DO UPDATE SET
    chunk_text  = EXCLUDED.chunk_text,
    embedding   = EXCLUDED.embedding,
    language    = EXCLUDED.language,
    created_at  = now()
RETURNING *;

-- =====================================================================
-- Delete all chunks for a specific entity (used on unpublish/delete and
-- by the admin DELETE /admin/rag/entity/:type/:id route).
-- =====================================================================

-- name: DeleteContentEmbeddingsByEntity :exec
DELETE FROM content_embeddings
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- =====================================================================
-- Delete chunks for an entity that exceed the current chunk count.
-- Used after re-indexing with fewer chunks: stale tail chunks must be
-- removed so the table never holds orphaned chunks from a shorter re-index.
-- =====================================================================

-- name: DeleteContentEmbeddingsAboveIndex :exec
DELETE FROM content_embeddings
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id
  AND chunk_index >= @max_index;

-- =====================================================================
-- HNSW cosine similarity retrieval (the core RAG query).
-- Returns the top-k most similar chunks for a query embedding, optionally
-- filtered by language. The <=> operator is cosine distance; we ORDER BY
-- ascending distance and LIMIT to top_k.
-- =====================================================================

-- name: SearchContentEmbeddings :many
SELECT
    id,
    tenant_id,
    entity_type,
    entity_id,
    chunk_index,
    chunk_text,
    language,
    created_at,
    (1.0 - (embedding <=> @query_embedding))::float8 AS similarity
FROM content_embeddings
WHERE tenant_id = @tenant_id
  AND (@language::text = '' OR language = @language)
ORDER BY embedding <=> @query_embedding
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
SELECT
    ce.id,
    ce.tenant_id,
    ce.entity_type,
    ce.entity_id,
    ce.chunk_index,
    ce.chunk_text,
    ce.language,
    ce.created_at,
    (1.0 - (ce.embedding <=> @query_embedding))::float8 AS similarity
FROM content_embeddings ce
JOIN contents c ON c.id = ce.entity_id AND ce.entity_type = 'content'
WHERE ce.tenant_id = @tenant_id
  AND (@language::text = '' OR ce.language = @language)
  AND (
      sqlc.narg(geo_space_id)::uuid IS NULL
      OR c.geo_space_id = sqlc.narg(geo_space_id)
      OR c.geo_space_id IN (SELECT id FROM spaces WHERE parent_id = sqlc.narg(geo_space_id))
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
WHERE tenant_id = @tenant_id;

-- name: GetContentEmbeddingStatsByType :many
SELECT
    entity_type,
    COUNT(*) AS chunk_count,
    COUNT(DISTINCT entity_id) AS entity_count
FROM content_embeddings
WHERE tenant_id = @tenant_id
GROUP BY entity_type
ORDER BY entity_type;

-- =====================================================================
-- Count chunks for an entity (used to determine if stale tail cleanup
-- is needed after a re-index with fewer chunks).
-- =====================================================================

-- name: CountContentEmbeddingsByEntity :one
SELECT COUNT(*) AS count
FROM content_embeddings
WHERE tenant_id = @tenant_id AND entity_type = @entity_type AND entity_id = @entity_id;

-- =====================================================================
-- List all published content IDs for a tenant (for full corpus re-index).
-- Only 'published' content is indexed in the RAG layer (Section 69).
-- =====================================================================

-- name: ListPublishedContentIDsForReindex :many
SELECT id, tenant_id FROM contents
WHERE tenant_id = @tenant_id
  AND status = 'published'
  AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;
