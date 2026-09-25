-- 000672_restore_user_source_blocks_unique.down.sql
-- Reverses 000672: drops the restored unique constraint and the two
-- de-tenant indexes. (Note: this restores the broken state — block toggles
-- will 500 again — which is what a true reversal of this fix means.)

BEGIN;

ALTER TABLE user_source_blocks
    DROP CONSTRAINT IF EXISTS user_source_blocks_blocker_source_key;

DROP INDEX IF EXISTS idx_user_source_blocks_blocker;
DROP INDEX IF EXISTS idx_user_source_blocks_source;

COMMIT;
