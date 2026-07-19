-- Universal Ratings & Reviews — polymorphic reviews with aggregate rollup.

-- =====================
-- Reviews CRUD
-- =====================

-- name: CreateReview :one
INSERT INTO reviews (
    tenant_id, entity_type, entity_id, reviewer_user_id,
    rating, title, body
) VALUES (
    @tenant_id, @entity_type, @entity_id, @reviewer_user_id,
    @rating, sqlc.narg(title), sqlc.narg(body)
)
RETURNING id, tenant_id, entity_type, entity_id, reviewer_user_id,
          rating, title, body, status, created_at, updated_at;

-- name: GetReviewByID :one
SELECT id, tenant_id, entity_type, entity_id, reviewer_user_id,
       rating, title, body, status, created_at, updated_at
FROM reviews
WHERE id = @id AND tenant_id = @tenant_id;

-- name: ListReviewsByEntity :many
SELECT id, tenant_id, entity_type, entity_id, reviewer_user_id,
       rating, title, body, status, created_at, updated_at
FROM reviews
WHERE tenant_id = @tenant_id
  AND entity_type = @entity_type
  AND entity_id = @entity_id
  AND status = 'published'
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviewsByReviewer :many
SELECT id, tenant_id, entity_type, entity_id, reviewer_user_id,
       rating, title, body, status, created_at, updated_at
FROM reviews
WHERE tenant_id = @tenant_id
  AND reviewer_user_id = @reviewer_user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListReviews :many
SELECT id, tenant_id, entity_type, entity_id, reviewer_user_id,
       rating, title, body, status, created_at, updated_at
FROM reviews
WHERE tenant_id = @tenant_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateReview :one
UPDATE reviews
SET rating = @rating, title = sqlc.narg(title), body = sqlc.narg(body),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
  AND reviewer_user_id = @reviewer_user_id
RETURNING id, tenant_id, entity_type, entity_id, reviewer_user_id,
          rating, title, body, status, created_at, updated_at;

-- name: UpdateReviewStatus :one
UPDATE reviews
SET status = @status, updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING id, tenant_id, entity_type, entity_id, reviewer_user_id,
          rating, title, body, status, created_at, updated_at;

-- name: DeleteReview :exec
DELETE FROM reviews
WHERE id = @id AND tenant_id = @tenant_id;

-- name: HasUserReviewedEntity :one
SELECT EXISTS(
    SELECT 1 FROM reviews
    WHERE tenant_id = @tenant_id
      AND entity_type = @entity_type
      AND entity_id = @entity_id
      AND reviewer_user_id = @reviewer_user_id
      AND status != 'removed'
);

-- =====================
-- Review Aggregates
-- =====================

-- name: UpsertReviewAggregate :one
INSERT INTO review_aggregates (
    tenant_id, entity_type, entity_id, review_count, average_rating
) VALUES (
    @tenant_id, @entity_type, @entity_id,
    (SELECT count(*)::int FROM reviews
     WHERE tenant_id = @tenant_id AND entity_type = @entity_type
       AND entity_id = @entity_id AND status = 'published'),
    COALESCE(
        (SELECT avg(rating)::numeric(3,2) FROM reviews
         WHERE tenant_id = @tenant_id AND entity_type = @entity_type
           AND entity_id = @entity_id AND status = 'published'),
        0
    )
)
ON CONFLICT (tenant_id, entity_type, entity_id) DO UPDATE
SET review_count = EXCLUDED.review_count,
    average_rating = EXCLUDED.average_rating,
    updated_at = now()
RETURNING tenant_id, entity_type, entity_id, review_count,
          average_rating, updated_at;

-- name: GetReviewAggregate :one
SELECT tenant_id, entity_type, entity_id, review_count,
       average_rating, updated_at
FROM review_aggregates
WHERE tenant_id = @tenant_id
  AND entity_type = @entity_type
  AND entity_id = @entity_id;

-- name: RecalculateReviewAggregate :exec
DELETE FROM review_aggregates
WHERE tenant_id = @tenant_id
  AND entity_type = @entity_type
  AND entity_id = @entity_id;
