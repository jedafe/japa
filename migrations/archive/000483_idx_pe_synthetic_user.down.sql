-- 000483_idx_pe_synthetic_user.down.sql
-- Reverses 000483_idx_pe_synthetic_user.up.sql.

BEGIN;

DROP INDEX IF EXISTS idx_pe_synthetic_user;

COMMIT;
