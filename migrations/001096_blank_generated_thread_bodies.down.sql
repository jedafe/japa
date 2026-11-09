-- 001096 down — generated thread bodies cannot be reconstructed after
-- blanking (the prompt text is deterministic but per-row summaries are
-- not retained). No-op; re-running 001095's backfill shape would only
-- re-mint rows that no longer exist.
SELECT 1;
