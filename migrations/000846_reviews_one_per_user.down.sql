-- Down migration for 000842: drop the one-review-per-user guard.

BEGIN;

DROP INDEX IF EXISTS reviews_entity_reviewer_unique;

COMMIT;
