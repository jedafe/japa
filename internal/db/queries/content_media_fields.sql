-- name: DeleteContentMediaFields :exec
-- Full replace pattern — clears a content row's media refs before the
-- caller re-inserts the set derived from the values being saved.
DELETE FROM content_media_fields WHERE content_id = @content_id;

-- name: InsertContentMediaFields :exec
-- Batch insert via parallel unnests — the caller passes equal-length
-- media_id/field_key/position arrays.
INSERT INTO content_media_fields (content_id, media_id, field_key, position)
SELECT @content_id, v.m, v.k, v.p
FROM (SELECT unnest(@media_ids::uuid[]) AS m,
             unnest(@field_keys::text[]) AS k,
             unnest(@positions::int[]) AS p) v
ON CONFLICT (content_id, field_key, position) DO NOTHING;

-- name: CountContentMediaRefsByMedia :one
-- Contents referencing a media row — powers "in use" reporting and any
-- future delete-warning surface. Counts refs from soft-deleted contents
-- too: ListMediaDeletedForCleanup blocks purge on any cmf row, so the
-- report must reflect what the sweep actually sees.
SELECT count(*)::int AS ref_count
FROM content_media_fields cmf
WHERE cmf.media_id = @media_id;

-- name: ListContentsByMedia :many
-- Which contents reference a media row (admin "used in" surface). Same
-- soft-delete semantics as the count — a ref keeps the media from purge
-- whether or not its content is deleted. c.deleted_at is surfaced so the
-- caller can badge deleted entries.
SELECT c.id, c.content_type, c.title, c.slug, c.deleted_at, cmf.field_key
FROM content_media_fields cmf
JOIN contents c ON c.id = cmf.content_id
WHERE cmf.media_id = @media_id
ORDER BY c.deleted_at NULLS FIRST, cmf.created_at DESC
LIMIT 50;

-- name: ListMediaIDsByStoragePaths :many
-- Resolves /media/serve/{bucket}/{path} URL pairs extracted from body text
-- back to media rows. No deleted_at filter — soft-deleted media still
-- serves (ServeMedia bypasses the row check), so its refs stay tracked
-- and keep blocking the purge sweep.
SELECT id, storage_bucket, storage_path
FROM media
WHERE (storage_bucket, storage_path) IN (
    SELECT v.b, v.p
    FROM (SELECT unnest(@buckets::text[]) AS b,
                 unnest(@paths::text[]) AS p) v
);

-- name: ListExistingMediaIDs :many
-- Existence check for content_media_fields tracking — soft-deleted rows
-- count as existing: the cmf FK still holds, soft-deleted media keeps
-- serving, and dropping the ref would let the cleanup sweep purge an
-- object the content still renders. Only truly-missing rows are absent.
SELECT id FROM media WHERE id = ANY(@ids::uuid[]);

-- name: ListMediaFieldKeysByEntityClasses :many
-- Keys of media-typed custom fields (image_single/image_multi/media) whose
-- groups are assigned to any of the given entity classes — e.g.
-- 'content_type', 'content_type:guide', 'content_type:guide:visa'.
SELECT DISTINCT f.key
FROM custom_fields f
JOIN custom_field_group_assignments a ON a.group_id = f.group_id
WHERE a.entity_class = ANY(@classes::text[])
  AND f.field_type IN ('image_single', 'image_multi', 'media');
