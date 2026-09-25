-- 000373_comparison_provenance.down.sql
-- Reverses 000373_comparison_provenance.up.sql.
-- Dropping a column that defaults to '{}' is safe — no data is held only
-- in metadata at this phase (the Comparison Engine that writes it lands
-- in Phase 7).
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md

BEGIN;

ALTER TABLE comparison_values DROP COLUMN IF EXISTS metadata;

COMMIT;
