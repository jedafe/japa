-- 000396_seed_ai_agent_draft_tool.up.sql
-- Section 103 (AI Agent Executor) — Phase 8 Content Request Flow & Authorship.
--
-- Seeds the content.create_draft tool into the ai_tools table. This tool is
-- the draft-request path for users without content.create permission: it is
-- gated by agent.draft_request (seeded by migration 000372), forces
-- status='draft', forces author_id to the AI persona's backing users.id
-- (resolved by the agent loop from agent.default_persona_id), and creates
-- a HITL review with source='agent_draft_request' (migration 000395).
--
-- The existing content.create tool (migration 000388) remains gated by
-- content.create and uses the calling user as author. The two tools are
-- mutually exclusive in a user's catalog: a user has either content.create
-- (→ content.create) or agent.draft_request (→ content.create_draft), not
-- both, unless an admin grants both.
--
-- NOTE: ROADMAP-AI-AGENT.md Phase 8 does not number this migration; it is
-- 000396 because 000395 is the ai_hitl_reviews.source migration and the
-- latest applied migration is 000394. Flagged per AGENTS.md Rule 1.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Content Request Flow, HITL Source Tracking)
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-AI-AGENT.md (Phase 8, tasks 8.01, 8.02, 8.05)

BEGIN;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
('00000000-0000-0000-0000-000000000001', 'content.create_draft',
 'Create a draft content row for admin review. Use this when pre-search found nothing and the user lacks content.create permission. The author is the AI persona (not the calling user); status is always draft. A HITL review is created so an admin can approve, edit, reject, or reassign the author before publishing.',
 'agent_content',
 '{
    "function_name": "content.create_draft",
    "description": "Create a draft content row for admin review (agent.draft_request path). Author is the AI persona; status is forced to draft; a HITL review is created.",
    "parameters": {
        "type": "object",
        "properties": {
            "title": { "type": "string", "description": "Content title" },
            "body": { "type": "string", "description": "Content body (markdown), synthesized from external search results + RAG" },
            "content_type": { "type": "string", "description": "Content type (e.g. article, guide)" },
            "author_id": { "type": "string", "description": "AI persona backing user UUID (resolved by the agent loop from agent.default_persona_id; the tool ignores any other value)" },
            "ai_persona_id": { "type": "string", "description": "The AI persona UUID (from agent.default_persona_id). Pass this so the draft records which persona authored it." },
            "geo_space_id": { "type": "string", "description": "Optional geo scope" },
            "space_id": { "type": "string", "description": "Optional space" },
            "field_values": { "type": "object", "description": "Custom field values" },
            "original_prompt": { "type": "string", "description": "The original user prompt that triggered this draft (for provenance)" }
        },
        "required": ["title", "content_type", "body"]
    },
    "handler": "ContentCreateDraft",
    "reads_user_data": false,
    "writes_user_data": true,
    "requires_hitl": true,
    "is_blocked": false
}',
 true, 'agent.draft_request')
ON CONFLICT (tenant_id, name) DO NOTHING;

COMMIT;
