-- 001176_federation_relay_inbox_idx.down.sql
BEGIN;

DROP INDEX IF EXISTS idx_federation_relays_inbox;

COMMIT;
