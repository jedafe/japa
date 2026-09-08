-- Video sources: canonical external video records for imports and uploads.

-- name: CreateOrUpdateVideoSource :one
INSERT INTO video_sources (
    tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    field_values, status
) VALUES (
    @tenant_id, @author_user_id, @source_type, @source_url, @external_id, @title,
    @description, @category, @thumbnail_url, @thumbnail_media_id, @duration_secs,
    COALESCE(@field_values, '{}'::jsonb), COALESCE(@status, 'active')
)
ON CONFLICT (tenant_id, source_url) DO UPDATE SET
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
    status = 'active',
    updated_at = now()
RETURNING
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at;

-- name: GetVideoSourceByID :one
SELECT
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at
FROM video_sources
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetVideoSourceByURL :one
SELECT
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at
FROM video_sources
WHERE tenant_id = @tenant_id AND source_url = @source_url;

-- name: GetVideoSourceByExternalID :one
SELECT
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at
FROM video_sources
WHERE tenant_id = @tenant_id AND source_type = @source_type AND external_id = @external_id;

-- name: ListVideoSourcesByAuthor :many
SELECT
    vs.id, vs.tenant_id, vs.author_user_id, vs.source_type, vs.source_url, vs.external_id,
    vs.title, vs.description, vs.category, vs.thumbnail_url, vs.thumbnail_media_id,
    vs.duration_secs, vs.status, vs.field_values, vs.created_at, vs.updated_at
FROM video_sources vs
JOIN users u ON u.id = vs.author_user_id AND u.tenant_id = vs.tenant_id
WHERE vs.tenant_id = @tenant_id AND u.username = @username AND vs.status = 'active'
ORDER BY vs.created_at DESC, vs.id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListVideoSourcesByAuthorUserID :many
SELECT
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at
FROM video_sources
WHERE tenant_id = @tenant_id AND author_user_id = @author_user_id
ORDER BY created_at DESC, id DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ReassignVideoSourcesAuthor :exec
-- Transfer all sources from a synthetic channel user to a real claimant.
UPDATE video_sources
SET author_user_id = @new_author_user_id, updated_at = now()
WHERE tenant_id = @tenant_id AND author_user_id = @old_author_user_id;

-- name: UpdateVideoSourceStatus :one
-- Used by the takedown approval flow to hide a specific video.
UPDATE video_sources
SET status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING
    id, tenant_id, author_user_id, source_type, source_url, external_id, title,
    description, category, thumbnail_url, thumbnail_media_id, duration_secs,
    status, field_values, created_at, updated_at;
