-- 000318_add_ama_to_forum_threads.up.sql
-- "Ask an Expat" Thread Format (§88, ADR-096) — adds ama_config JSONB
-- column to forum_threads for AMA-type threads.
-- Ref: japaconnect-engineering/sections/88-ask-an-expat.md

BEGIN;

ALTER TABLE forum_threads
    ADD COLUMN IF NOT EXISTS ama_config jsonb;
-- ama_config is only populated when thread_type = 'ama'.
-- Structure: {open_duration_hours, question_approval_required, closes_at,
--   host_profession, host_dest_space_id, host_years_abroad}

CREATE INDEX IF NOT EXISTS idx_forum_threads_ama
    ON forum_threads(tenant_id, created_at DESC)
    WHERE ama_config IS NOT NULL;

COMMIT;
