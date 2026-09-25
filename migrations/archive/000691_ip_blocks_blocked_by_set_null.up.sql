-- 000691_ip_blocks_blocked_by_set_null.up.sql
-- Renumbered from 000420 — it collided with 000420_ai_hitl_reviews_source_studio.
-- Tail-position body; databases that applied the 000420 file skip this via
-- the 000000 registry marker. Unchanged: already idempotent (DROP IF EXISTS).
-- BUG-011: ip_blocks.blocked_by references users(id) with no ON DELETE
-- clause, so erasing an admin who created IP blocks leaves a dangling FK.
-- Per §15 (GDPR) and §41, change the FK to ON DELETE SET NULL so the IP
-- block survives (it is tenant safety config, not user content) while the
-- blocked_by reference is cleared.
ALTER TABLE ip_blocks
    DROP CONSTRAINT IF EXISTS ip_blocks_blocked_by_fkey,
    ADD CONSTRAINT ip_blocks_blocked_by_fkey
        FOREIGN KEY (blocked_by) REFERENCES users(id) ON DELETE SET NULL;
