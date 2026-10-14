-- R1: authoritative one-review-per-user-per-entity guard.
-- CreateReview's check-then-insert races under concurrent double
-- submit; the unique index is the backstop the service maps to
-- 409 already_reviewed (internal/reviews ErrAlreadyReviewed).
-- Down migration drops the guard; existing duplicate rows (if any)
-- are deduplicated first so the index can build cleanly.

BEGIN;

-- Keep the newest non-removed review per (entity_type, entity_id,
-- reviewer_user_id). reviews has no deleted_at — soft delete is
-- status='removed' (+ removed_by), matching HasUserReviewedEntity.
DELETE FROM reviews r1
USING reviews r2
WHERE r1.entity_type = r2.entity_type
  AND r1.entity_id = r2.entity_id
  AND r1.reviewer_user_id = r2.reviewer_user_id
  AND r1.status != 'removed'
  AND r2.status != 'removed'
  AND (r1.created_at < r2.created_at
       OR (r1.created_at = r2.created_at AND r1.id < r2.id));

CREATE UNIQUE INDEX reviews_entity_reviewer_unique
    ON reviews (entity_type, entity_id, reviewer_user_id)
    WHERE status != 'removed';

COMMIT;
