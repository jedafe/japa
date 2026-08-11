-- 000373_comparison_provenance.up.sql
-- Section 103 (Comparison Engine, extends §30) — provenance tracking.
--
-- Adds a `metadata` JSONB column to comparison_values so each value can
-- record its source (custom_field|rag|external|llm_knowledge), source_ref,
-- confidence, and retrieved_at timestamp. The Comparison Engine (Phase 7)
-- writes this metadata; the column is added here so the schema is ready.
--
-- NOTE: ROADMAP-AI-AGENT.md named this migration 000370, but 000370 was
-- already used (fix_org_boosts_fk). This migration uses 000373, the next
-- contiguous number after 000372. Flagged per AGENTS.md Rule 1.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Provenance)
--      japaconnect-engineering/sections/30-comparison-system.md

BEGIN;

-- Additive NULLable-with-default column — zero downtime. Existing rows and
-- queries that do not reference metadata continue to work unchanged.
ALTER TABLE comparison_values
    ADD COLUMN IF NOT EXISTS metadata jsonb NOT NULL DEFAULT '{}'::jsonb;

COMMIT;
