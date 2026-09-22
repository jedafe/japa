-- 000688_ownership_claims_unique_pending.down.sql
-- Renumbered from 000412 (duplicate version prefix). See the .up file.
-- 000412 down: Remove the unique index on pending ownership claims.
DROP INDEX IF EXISTS idx_ownership_claims_pending_unique;
