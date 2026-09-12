-- Video sources: canonical external video records for imports and uploads.

-- name: CreateOrUpdateVideoSource :one
INSERT INTO video_sources (author_user_id, source_type, source_url, external_id, title, description, category, thumbnail_url, thumbnail_media_id, duration_secs, field_values, status, source_id) VALUES (@author_user_id, @source_type, @source_url, @external_id, @title, @description, @category, @thumbnail_url, @thumbnail_media_id, @duration_secs, COALESCE(@field_values, '{}'::jsonb), COALESCE(@status, 'active'), @source_id)
ON CONFLICT (source_url) DO UPDATE SET
 author_user_id = EXCLUDED.author_user_id,
 source_type = EXCLUDED.source_type,
 external_id = EXCLUDED.external_id,
 title = EXCLUDED.title,
 description = EXCLUDED.description,
 category = COALESCE(EXCLUDED.category, video_sources.category),
 thumbnail_url = EXCLUDED.thumbnail_url,
 thumbnail_media_id = EXCLUDED.thumbnail_media_id,
 duration_secs = EXCLUDED.duration_secs,
 field_values = EXCLUDED.field_values,
 source_id = COALESCE(EXCLUDED.source_id, video_sources.source_id),
 status = 'active',
 updated_at = now()
RETURNING *;

-- name: GetVideoSourceByID :one
SELECT *
FROM video_sources
WHERE id = @id;

-- name: GetVideoSourceByURL :one
SELECT *
FROM video_sources
WHERE source_url = @source_url;

-- name: GetVideoSourceByExternalID :one
SELECT *
FROM video_sources
WHERE source_type = @source_type AND external_id = @external_id;

-- name: ListVideoSourcesByAuthor :many
SELECT vs.*
FROM video_sources vs
JOIN users u ON u.id = vs.author_user_id
WHERE u.username = @username AND vs.status = 'active'
ORDER BY vs.created_at DESC, vs.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListVideoSourcesByAuthorUserID :many
SELECT *
FROM video_sources
WHERE author_user_id = @author_user_id
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ReassignVideoSourcesAuthor :exec
-- Transfer all sources from a synthetic channel user to a real claimant.
UPDATE video_sources
SET author_user_id = @new_author_user_id, updated_at = now()
WHERE author_user_id = @old_author_user_id;

-- name: ListVideoSourcesBySourceID :many
-- Returns active video sources linked to a canonical intel_sources row.
SELECT *
FROM video_sources
WHERE source_id = @source_id AND status = 'active'
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountVideoSourcesBySourceID :one
SELECT COUNT(*)::int AS total
FROM video_sources
WHERE source_id = @source_id AND status = 'active';

-- name: UpdateVideoSourceStatus :one
-- Used by the takedown approval flow to hide a specific video.
UPDATE video_sources
SET status = @status, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: ListVideoSourcesForAdmin :many
-- Admin queue with optional status/source_type filters, text search, and sorting.
SELECT
 vs.id, vs.author_user_id, vs.source_type, vs.source_url, vs.external_id,
 vs.title, vs.description, vs.category, vs.thumbnail_url, vs.thumbnail_media_id,
 vs.duration_secs, vs.status, vs.field_values, vs.source_id, vs.created_at, vs.updated_at,
 u.username AS author_username
FROM video_sources vs
JOIN users u ON u.id = vs.author_user_id
WHERE (@status::text = '' OR vs.status = @status)
 AND (@source_type::text = '' OR vs.source_type = @source_type)
 AND (@q::text = '' OR vs.title ILIKE '%' || @q || '%' OR vs.source_url ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%')
ORDER BY
 CASE WHEN @sort = 'title' AND @dir = 'asc' THEN vs.title END ASC NULLS LAST,
 CASE WHEN @sort = 'title' AND @dir = 'desc' THEN vs.title END DESC NULLS FIRST,
 CASE WHEN @sort = 'source_type' AND @dir = 'asc' THEN vs.source_type END ASC NULLS LAST,
 CASE WHEN @sort = 'source_type' AND @dir = 'desc' THEN vs.source_type END DESC NULLS FIRST,
 CASE WHEN @sort = 'status' AND @dir = 'asc' THEN vs.status END ASC NULLS LAST,
 CASE WHEN @sort = 'status' AND @dir = 'desc' THEN vs.status END DESC NULLS FIRST,
 CASE WHEN @sort = 'created_at' AND @dir = 'asc' THEN vs.created_at END ASC NULLS LAST,
 CASE WHEN @sort = 'created_at' AND @dir = 'desc' THEN vs.created_at END DESC NULLS FIRST,
 vs.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountVideoSourcesForAdmin :one
-- Total for admin video sources list (same filters as ListVideoSourcesForAdmin).
SELECT COUNT(*)::int AS total
FROM video_sources vs
JOIN users u ON u.id = vs.author_user_id
WHERE (@status::text = '' OR vs.status = @status)
 AND (@source_type::text = '' OR vs.source_type = @source_type)
 AND (@q::text = '' OR vs.title ILIKE '%' || @q || '%' OR vs.source_url ILIKE '%' || @q || '%' OR u.username ILIKE '%' || @q || '%');
