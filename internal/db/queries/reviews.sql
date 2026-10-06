-- name: CreateReview :one
INSERT INTO reviews (entity_type, entity_id, reviewer_user_id, rating, title, body) VALUES (@entity_type, @entity_id, @reviewer_user_id, @rating, sqlc.narg(title), sqlc.narg(body))
RETURNING id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by;

-- name: GetReviewByID :one
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews
WHERE id = @id;

-- name: ListReviewsByEntity :many
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews
WHERE entity_type = @entity_type
 AND entity_id = @entity_id
 AND status = 'published'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviewsByEntityHighestRating :many
-- Sort by highest rating first (spec §29: ?sort=highest).
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews
WHERE entity_type = @entity_type
 AND entity_id = @entity_id
 AND status = 'published'
ORDER BY rating DESC, created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviewsByEntityLowestRating :many
-- Sort by lowest rating first (spec §29: ?sort=lowest).
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews
WHERE entity_type = @entity_type
 AND entity_id = @entity_id
 AND status = 'published'
ORDER BY rating ASC, created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviewsByEntityWithAuthor :many
-- Reviews joined to users + user_profiles so the learning track reviews list
-- (GET /academy/tracks/{id}/reviews) can render author username, display name,
-- and avatar without N+1 lookups. Mirrors the polymorphic reviews table
-- (entity_type/entity_id) — the caller filters by entity_type='academy_track'.
-- Ref: ADR-060 (Track Detail — Reviews tab), Section 29 (polymorphic reviews).
SELECT r.id, r.entity_type, r.entity_id, r.reviewer_user_id,
 r.rating, r.title, r.body, r.status, r.created_at, r.updated_at, r.removed_by,
 u.username AS author_username,
 up.display_name AS author_display_name,
 up.avatar_url AS author_avatar_url
FROM reviews r
LEFT JOIN users u ON u.id = r.reviewer_user_id
LEFT JOIN user_profiles up ON up.user_id = u.id
WHERE r.entity_type = @entity_type
 AND r.entity_id = @entity_id
 AND r.status = 'published'
ORDER BY r.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviewsByReviewer :many
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews
WHERE reviewer_user_id = @reviewer_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviews :many
SELECT id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by
FROM reviews

ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAdminReviews :many
SELECT r.id, r.entity_type, r.entity_id, r.reviewer_user_id,
 r.rating, r.title, r.body, r.status, r.created_at, r.updated_at, r.removed_by,
 COALESCE(
 CASE r.entity_type
 WHEN 'organization' THEN (SELECT o.name FROM organizations o WHERE o.id = r.entity_id)
 WHEN 'university' THEN (SELECT s.name FROM spaces s WHERE s.id = r.entity_id)
 WHEN 'service' THEN (SELECT s.name FROM spaces s WHERE s.id = r.entity_id)
 WHEN 'mentor' THEN (SELECT COALESCE(up.display_name, u.username) FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id WHERE u.id = r.entity_id)
 END,
 r.entity_type
 )::text AS entity_name
FROM reviews r

ORDER BY r.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListAdminReviewsFiltered :many
SELECT r.id, r.entity_type, r.entity_id, r.reviewer_user_id,
 r.rating, r.title, r.body, r.status, r.created_at, r.updated_at, r.removed_by,
 COALESCE(
 CASE r.entity_type
 WHEN 'organization' THEN (SELECT o.name FROM organizations o WHERE o.id = r.entity_id)
 WHEN 'university' THEN (SELECT s.name FROM spaces s WHERE s.id = r.entity_id)
 WHEN 'service' THEN (SELECT s.name FROM spaces s WHERE s.id = r.entity_id)
 WHEN 'mentor' THEN (SELECT COALESCE(up.display_name, u.username) FROM users u LEFT JOIN user_profiles up ON up.user_id = u.id WHERE u.id = r.entity_id)
 END,
 r.entity_type
 )::text AS entity_name
FROM reviews r
WHERE (@status::text = '' OR r.status = @status)
 AND (@entity_type::text = '' OR r.entity_type = @entity_type)
ORDER BY r.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAdminReviews :one
SELECT count(*)::int AS total
FROM reviews
;

-- name: CountAdminReviewsFiltered :one
SELECT count(*)::int AS total
FROM reviews
WHERE (@status::text = '' OR status = @status)
 AND (@entity_type::text = '' OR entity_type = @entity_type);

-- name: UpdateReview :one
UPDATE reviews
SET rating = @rating, title = sqlc.narg(title), body = sqlc.narg(body),
 updated_at = now()
WHERE id = @id
 AND reviewer_user_id = @reviewer_user_id
RETURNING id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by;

-- name: UpdateReviewStatus :one
-- Sets status and records who removed the review (spec §29 L55: removed_by).
-- Pass removed_by = NULL for non-removal status changes (flag/publish).
UPDATE reviews
SET status = @status, removed_by = sqlc.narg(removed_by), updated_at = now()
WHERE id = @id
RETURNING id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by;

-- name: SoftDeleteOwnReview :one
-- Owner-scoped soft delete: sets status to 'removed' only if the requesting
-- user is the review's author. Prevents any user with reviews.delete_own
-- from deleting other users' reviews (BUG-001). Records removed_by (spec §29).
UPDATE reviews
SET status = 'removed', removed_by = @reviewer_user_id, updated_at = now()
WHERE id = @id
 AND reviewer_user_id = @reviewer_user_id
RETURNING id, entity_type, entity_id, reviewer_user_id,
 rating, title, body, status, created_at, updated_at, removed_by;

-- name: DeleteReview :exec
DELETE FROM reviews
WHERE id = @id;

-- name: HasUserReviewedEntity :one
SELECT EXISTS(
 SELECT 1 FROM reviews
 WHERE entity_type = @entity_type
 AND entity_id = @entity_id
 AND reviewer_user_id = @reviewer_user_id
 AND status != 'removed'
);

-- =====================
-- Review Aggregates
-- =====================

-- name: UpsertReviewAggregate :one
INSERT INTO review_aggregates (entity_type, entity_id, review_count, average_rating) VALUES (@entity_type, @entity_id, (SELECT count(*)::int FROM reviews
 WHERE entity_type = @entity_type
 AND entity_id = @entity_id AND status = 'published'), COALESCE(
 (SELECT avg(rating)::numeric(3,2) FROM reviews
 WHERE entity_type = @entity_type
 AND entity_id = @entity_id AND status = 'published'),
 0
 ))
ON CONFLICT (entity_type, entity_id) DO UPDATE
SET review_count = EXCLUDED.review_count,
 average_rating = EXCLUDED.average_rating,
 updated_at = now()
RETURNING entity_type, entity_id, review_count,
 average_rating, updated_at;

-- name: GetReviewAggregate :one
SELECT entity_type, entity_id, review_count,
 average_rating, updated_at
FROM review_aggregates
WHERE entity_type = @entity_type
 AND entity_id = @entity_id;

-- name: RecalculateReviewAggregate :exec
DELETE FROM review_aggregates
WHERE entity_type = @entity_type
 AND entity_id = @entity_id;
