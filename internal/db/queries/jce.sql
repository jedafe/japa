-- =====================
-- Data sources (JCE Sub-spec 4)
-- =====================

-- name: CreateDataSource :one
INSERT INTO comparison_data_sources (entity_type, entity_id, dimension_slug, source_name, source_url, source_kind, trust_level, config, source_id) VALUES (sqlc.narg(entity_type), sqlc.narg(entity_id), sqlc.narg(dimension_slug), @source_name, sqlc.narg(source_url), @source_kind, @trust_level, @config, sqlc.narg(source_id))
RETURNING *;

-- name: ListDataSources :many
SELECT * FROM comparison_data_sources
WHERE (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
 AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid)
ORDER BY source_name ASC
LIMIT NULLIF(@page_limit, 0) OFFSET @page_offset;

-- name: CountDataSources :one
SELECT count(*)::int FROM comparison_data_sources
WHERE (sqlc.narg(entity_type)::text IS NULL OR entity_type = sqlc.narg(entity_type)::text)
 AND (sqlc.narg(entity_id)::uuid IS NULL OR entity_id = sqlc.narg(entity_id)::uuid);

-- name: GetDataSource :one
SELECT * FROM comparison_data_sources
WHERE id = @id;

-- name: UpdateDataSource :one
UPDATE comparison_data_sources
SET entity_type = sqlc.narg(entity_type),
 entity_id = sqlc.narg(entity_id),
 dimension_slug = sqlc.narg(dimension_slug),
 source_name = @source_name,
 source_url = sqlc.narg(source_url),
 source_kind = @source_kind,
 trust_level = @trust_level,
 freshness_score = COALESCE(sqlc.narg(freshness_score), freshness_score),
 last_verified_at = sqlc.narg(last_verified_at),
 config = @config,
 source_id = COALESCE(sqlc.narg(source_id), source_id),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: DeleteDataSource :exec
DELETE FROM comparison_data_sources
WHERE id = @id;

-- name: TouchDataSource :exec
-- Updates the freshness bookkeeping columns for a data source after it has
-- contributed to a successful refresh.
UPDATE comparison_data_sources
SET last_verified_at = COALESCE(sqlc.narg(last_verified_at), last_verified_at),
 freshness_score = COALESCE(sqlc.narg(freshness_score), freshness_score),
 updated_at = now()
WHERE id = @id;

-- name: MatchDataSource :one
-- Finds the most specific data source for the given scope, falling back to more general scopes.
-- An empty @source_name matches any source_name. A NULL @source_kind matches any source_kind.
SELECT * FROM comparison_data_sources
WHERE (source_name = @source_name OR @source_name = '')
 AND (entity_type IS NOT DISTINCT FROM sqlc.narg(entity_type)::text)
 AND (entity_id IS NOT DISTINCT FROM sqlc.narg(entity_id)::uuid)
 AND (dimension_slug IS NOT DISTINCT FROM sqlc.narg(dimension_slug)::text)
 AND (sqlc.narg(source_kind)::text IS NULL OR source_kind = sqlc.narg(source_kind)::text)
ORDER BY
 (entity_id IS NOT NULL)::int + (dimension_slug IS NOT NULL)::int DESC,
 trust_level DESC,
 freshness_score DESC,
 created_at ASC
LIMIT 1;

-- =====================
-- Ingestion jobs (JCE Sub-spec 4)
-- =====================

-- name: CreateIngestionJob :one
INSERT INTO comparison_data_ingestion_jobs (content_id, source_kind) VALUES (@content_id, sqlc.narg(source_kind))
RETURNING *;

-- name: GetIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE id = @id;

-- name: UpdateIngestionJob :one
UPDATE comparison_data_ingestion_jobs
SET status = @status,
 refreshed_count = @refreshed_count,
 error = sqlc.narg(error),
 started_at = sqlc.narg(started_at),
 completed_at = sqlc.narg(completed_at),
 updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListIngestionJobsByContent :many
SELECT * FROM comparison_data_ingestion_jobs
WHERE content_id = @content_id
ORDER BY created_at DESC
LIMIT @limit_val OFFSET @offset_val;

-- name: GetLatestCompletedIngestionJob :one
SELECT * FROM comparison_data_ingestion_jobs
WHERE content_id = @content_id AND status = 'completed'
ORDER BY completed_at DESC
LIMIT 1;
