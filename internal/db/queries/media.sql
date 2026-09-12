-- Media system queries (Section 34).
-- Two-step upload: pre-sign → client uploads to SeaweedFS → confirm.

-- =====================
-- Insert
-- =====================

-- name: InsertMedia :one
INSERT INTO media (uploader_user_id, original_filename, mime_type, file_size_bytes, storage_path, storage_bucket, source_platform, source_photographer, source_photographer_url, source_license, source_license_url, source_photo_id) VALUES (@uploader_user_id, @original_filename, @mime_type, @file_size_bytes, @storage_path, @storage_bucket, @source_platform, @source_photographer, @source_photographer_url, @source_license, @source_license_url, @source_photo_id)

RETURNING *;

-- =====================
-- Read
-- =====================

-- name: GetMediaByID :one
SELECT * FROM media WHERE id = @id AND deleted_at IS NULL;

-- name: GetMediaByStoragePath :one
SELECT * FROM media WHERE storage_path = @storage_path;

-- =====================
-- Update
-- =====================

-- name: ConfirmMedia :exec
UPDATE media SET confirmed = true WHERE id = @id;

-- name: UpdateMediaMetadata :exec
-- Admin metadata update — no ownership filter (gated by media.edit_any).
UPDATE media
SET alt_text = COALESCE(@alt_text, alt_text),
 caption = COALESCE(@caption, caption),
 width_px = COALESCE(@width_px, width_px),
 height_px = COALESCE(@height_px, height_px),
 duration_secs = COALESCE(@duration_secs, duration_secs),
 user_group_id = @user_group_id
WHERE id = @id;

-- name: UpdateMediaMetadataOwn :exec
-- User-facing metadata update — scoped to the uploader (gated by media.edit_own).
UPDATE media
SET alt_text = COALESCE(@alt_text, alt_text),
 caption = COALESCE(@caption, caption),
 width_px = COALESCE(@width_px, width_px),
 height_px = COALESCE(@height_px, height_px),
 duration_secs = COALESCE(@duration_secs, duration_secs),
 user_group_id = @user_group_id
WHERE id = @id
 AND uploader_user_id = @uploader_user_id
 AND deleted_at IS NULL;

-- name: UpdateMediaStoragePath :exec
UPDATE media SET storage_path = @storage_path WHERE id = @id;

-- name: SoftDeleteMedia :exec
-- Admin delete — no ownership filter (gated by media.delete_any).
UPDATE media SET deleted_at = now() WHERE id = @id;

-- name: SoftDeleteMediaOwn :exec
-- User-facing delete — scoped to the uploader (gated by media.delete_own).
-- Returns no rows if the media belongs to a different user, which the caller
-- treats as a 404/403 to avoid leaking existence information.
UPDATE media SET deleted_at = now()
WHERE id = @id
 AND uploader_user_id = @uploader_user_id
 AND deleted_at IS NULL;

-- name: RejectMediaByIDAndVirus :exec
UPDATE media
SET deleted_at = now(),
 alt_text = COALESCE(alt_text, '') || ' [REJECTED: ' || @virus_name || ']'
WHERE id = @id AND deleted_at IS NULL;

-- name: QuarantineMediaByID :exec
UPDATE media
SET quarantine_reason = @reason,
 quarantined_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- name: ApproveQuarantinedMedia :exec
UPDATE media
SET quarantine_reason = NULL,
 quarantined_at = NULL,
 reviewed_by = @reviewed_by,
 reviewed_at = now(),
 confirmed = true
WHERE id = @id AND confirmed = false AND deleted_at IS NULL;

-- name: RejectQuarantinedMedia :exec
UPDATE media
SET deleted_at = now(),
 reviewed_by = @reviewed_by,
 reviewed_at = now()
WHERE id = @id AND confirmed = false AND deleted_at IS NULL;

-- name: ListQuarantinedMedia :many
SELECT id, uploader_user_id, original_filename, mime_type,
 file_size_bytes, width_px, height_px, duration_secs, storage_path,
 storage_bucket, alt_text, caption, confirmed, deleted_at,
 quarantine_reason, quarantined_at, reviewed_by, reviewed_at,
 created_at, user_group_id
FROM media
WHERE quarantine_reason IS NOT NULL
 AND deleted_at IS NULL
ORDER BY quarantined_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Lists
-- =====================

-- name: ListMediaByUploader :many
SELECT * FROM media
WHERE uploader_user_id = @uploader_user_id AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListMediaByTenant :many
SELECT * FROM media
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListMediaByType :many
SELECT * FROM media
WHERE mime_type LIKE @mime_pattern || '%' AND deleted_at IS NULL
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: SearchMedia :many
SELECT * FROM media
WHERE deleted_at IS NULL
 AND (original_filename ILIKE '%' || @query || '%'
 OR alt_text ILIKE '%' || @query || '%'
 OR caption ILIKE '%' || @query || '%')
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- =====================
-- Counts
-- =====================

-- name: CountMediaByUploader :one
SELECT COUNT(*)::int AS count FROM media
WHERE uploader_user_id = @uploader_user_id AND deleted_at IS NULL;

-- name: CountMediaByTenant :one
SELECT COUNT(*)::int AS count FROM media
WHERE deleted_at IS NULL;

-- name: CountMediaByType :one
SELECT COUNT(*)::int AS count FROM media
WHERE mime_type LIKE @mime_pattern || '%' AND deleted_at IS NULL;

-- name: CountSearchMedia :one
SELECT COUNT(*)::int AS count FROM media
WHERE deleted_at IS NULL
 AND (original_filename ILIKE '%' || @query || '%'
 OR alt_text ILIKE '%' || @query || '%'
 OR caption ILIKE '%' || @query || '%');

-- =====================
-- Unified filtered list (admin) — combines uploader, type, search, and status
-- filters in a single query. All filter params are nullable; a NULL param
-- means "no filter on this field". Used by the admin media list endpoint.
-- =====================

-- name: ListMediaFiltered :many
SELECT * FROM media
WHERE deleted_at IS NULL
 AND (@uploader_user_id::uuid IS NULL OR uploader_user_id = @uploader_user_id)
 AND (@mime_pattern::text = '' OR mime_type LIKE @mime_pattern || '%')
 AND (@query::text = ''
 OR original_filename ILIKE '%' || @query || '%'
 OR alt_text ILIKE '%' || @query || '%'
 OR caption ILIKE '%' || @query || '%')
 AND (
 @status::text = '' OR
 (@status = 'approved' AND confirmed = true) OR
 (@status = 'pending' AND confirmed = false AND (quarantine_reason IS NULL OR quarantine_reason = '') AND rejected_at IS NULL) OR
 (@status = 'quarantined' AND confirmed = false AND quarantine_reason IS NOT NULL AND quarantine_reason != '') OR
 (@status = 'rejected' AND confirmed = false AND (quarantine_reason IS NULL OR quarantine_reason = '') AND rejected_at IS NOT NULL)
 )
 AND (@flagged::text = '' OR flagged = (@flagged = 'true'))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMediaFiltered :one
SELECT COUNT(*)::int AS count FROM media
WHERE deleted_at IS NULL
 AND (@uploader_user_id::uuid IS NULL OR uploader_user_id = @uploader_user_id)
 AND (@mime_pattern::text = '' OR mime_type LIKE @mime_pattern || '%')
 AND (@query::text = ''
 OR original_filename ILIKE '%' || @query || '%'
 OR alt_text ILIKE '%' || @query || '%'
 OR caption ILIKE '%' || @query || '%')
 AND (
 @status::text = '' OR
 (@status = 'approved' AND confirmed = true) OR
 (@status = 'pending' AND confirmed = false AND (quarantine_reason IS NULL OR quarantine_reason = '') AND rejected_at IS NULL) OR
 (@status = 'quarantined' AND confirmed = false AND quarantine_reason IS NOT NULL AND quarantine_reason != '') OR
 (@status = 'rejected' AND confirmed = false AND (quarantine_reason IS NULL OR quarantine_reason = '') AND rejected_at IS NOT NULL)
 )
 AND (@flagged::text = '' OR flagged = (@flagged = 'true'));

-- name: ListMediaFromPublicPostsByUser :many
-- Public profile Media tab: media referenced (featured/header image) by the
-- user's PUBLISHED PUBLIC posts. Excludes unconfirmed uploads, group-restricted
-- media, soft-deleted media, and non-image files (e.g. HTML error pages
-- mislabeled as images). Public-safe. Mirrors ListMediaFromOrgPublicPosts but
-- scoped to the author's posts (c.author_id) instead of the org.
SELECT m.* FROM media m
WHERE m.deleted_at IS NULL
 AND m.confirmed = true
 AND m.user_group_id IS NULL
 AND m.uploader_user_id = @author_id
 AND m.mime_type LIKE 'image/%'
 AND EXISTS (
 SELECT 1 FROM contents c
 WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
 )
ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMediaFromPublicPostsByUser :one
SELECT COUNT(*)::int AS count FROM media m
WHERE m.deleted_at IS NULL
 AND m.confirmed = true
 AND m.user_group_id IS NULL
 AND m.uploader_user_id = @author_id
 AND m.mime_type LIKE 'image/%'
 AND EXISTS (
 SELECT 1 FROM contents c
 WHERE c.author_id = @author_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
 );

-- name: ListMediaFromOrgPublicPosts :many
-- Org profile Media tab: media referenced (featured/header image) by the ORG's
-- PUBLISHED PUBLIC posts. Excludes unconfirmed uploads and group-restricted media.
SELECT m.* FROM media m
WHERE m.deleted_at IS NULL
 AND m.confirmed = true
 AND m.user_group_id IS NULL
 AND EXISTS (
 SELECT 1 FROM contents c
 WHERE c.organization_id = @organization_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
 )
ORDER BY m.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountMediaFromOrgPublicPosts :one
SELECT COUNT(*)::int AS count FROM media m
WHERE m.deleted_at IS NULL
 AND m.confirmed = true
 AND m.user_group_id IS NULL
 AND EXISTS (
 SELECT 1 FROM contents c
 WHERE c.organization_id = @organization_id
 AND c.status = 'published'
 AND c.visibility = 'public'
 AND (c.featured_image_id = m.id OR c.header_image_id = m.id)
 );

-- =====================
-- Bulk Operations
-- =====================

-- name: BulkSoftDeleteMedia :exec
-- Admin bulk delete — no ownership filter (gated by media.delete_any).
UPDATE media SET deleted_at = now()
WHERE id = ANY(@media_ids::uuid[]);

-- name: BulkSoftDeleteMediaOwn :exec
-- User-facing bulk delete — scoped to the uploader (gated by media.delete_own).
UPDATE media SET deleted_at = now()
WHERE id = ANY(@media_ids::uuid[])
 AND uploader_user_id = @uploader_user_id
 AND deleted_at IS NULL;

-- name: AdminUpdateMediaStatus :exec
UPDATE media
SET confirmed = @confirmed,
 quarantine_reason = @quarantine_reason::text,
 reviewed_by = @reviewed_by,
 reviewed_at = CASE WHEN @quarantine_reason IS NULL AND @confirmed THEN now() ELSE reviewed_at END,
 rejected_at = CASE WHEN @quarantine_reason IS NULL AND NOT @confirmed THEN now() ELSE rejected_at END,
 quarantined_at = CASE WHEN @quarantine_reason IS NOT NULL AND @quarantine_reason != '' THEN now() ELSE quarantined_at END
WHERE id = @id AND deleted_at IS NULL;

-- name: ListMediaForScanning :many
SELECT id, storage_path FROM media
WHERE confirmed = false AND deleted_at IS NULL AND quarantined_at IS NULL AND rejected_at IS NULL
ORDER BY created_at ASC
LIMIT 50;
