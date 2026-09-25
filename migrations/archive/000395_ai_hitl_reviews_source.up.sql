-- 000395_ai_hitl_reviews_source.up.sql
-- Section 103 (AI Agent Executor) — Phase 8 Content Request Flow & Authorship.
--
-- Adds a `source` column to ai_hitl_reviews so the admin panel can filter
-- agent-sourced drafts from manual/admin-generated HITL reviews. The column
-- is additive: existing rows default to 'manual' and all existing callers
-- (ai.CreateHITLReview) continue to work unchanged — source is an optional
-- field that defaults to 'manual' when not specified.
--
-- Values:
--   'manual'              — default; admin-generated or pre-Phase 8 reviews
--   'agent_draft_request' — draft created by the AI agent via the
--                           content.create_draft tool or the
--                           agent:create_draft_request Asynq task
--   'agent_tool'          — HITL review created by the executor for a tool
--                           with requires_hitl=true (Phase 3)
--
-- The roadmap's mention of "§22 content_reviews queue with source =
-- 'agent_draft_request'" refers to this column on ai_hitl_reviews (the
-- actual HITL queue), not the content_reviews table (which is post-decision
-- editorial feedback, §24). Flagged per AGENTS.md Rule 1.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (HITL Source Tracking, Content Request Flow)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, task 8.05)

BEGIN;

ALTER TABLE ai_hitl_reviews
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'manual'
    CHECK (source IN ('manual', 'agent_draft_request', 'agent_tool'));

CREATE INDEX IF NOT EXISTS idx_ai_hitl_reviews_source
    ON ai_hitl_reviews (tenant_id, source, status, created_at DESC);

COMMIT;
