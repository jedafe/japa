-- 000691_ip_blocks_blocked_by_set_null.down.sql
-- Renumbered from 000420 (duplicate version prefix). See the .up file.
-- Reverse BUG-011: restore the original FK without ON DELETE SET NULL.
ALTER TABLE ip_blocks
    DROP CONSTRAINT IF EXISTS ip_blocks_blocked_by_fkey,
    ADD CONSTRAINT ip_blocks_blocked_by_fkey
        FOREIGN KEY (blocked_by) REFERENCES users(id);
