-- 000412 down: Remove the unique index on pending ownership claims.
DROP INDEX IF EXISTS idx_ownership_claims_pending_unique;
