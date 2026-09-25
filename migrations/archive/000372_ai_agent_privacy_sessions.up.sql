-- 000372_ai_agent_privacy_sessions.up.sql
-- Section 103 (AI Agent Executor) — Schema & Privacy Foundation.
--
-- Adds the AI privacy-exemption columns (contents, custom_field_values,
-- users), the agent session + tool-call audit tables, the 9 agent.*
-- system_settings, and the 4 new RBAC permission nodes.
--
-- NOTE: ROADMAP-AI-AGENT.md named this migration 000369, but the repo had
-- already advanced past 000368 (latest applied is 000371, and 000369 was
-- already used twice — a pre-existing collision). This migration therefore
-- uses 000372, the next contiguous number. Flagged per AGENTS.md Rule 1
-- (section file is source of truth; numbering follows the runner).
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
--      japaconnect-engineering/adr/ADR-111-ai-agent-executor.md
--      japaconnect-engineering/conventions/system-settings-catalog.md
--      japaconnect-engineering/conventions/valkey-keys-catalog.md
--      japaconnect-engineering/conventions/permission-key-naming.md

BEGIN;

-- =====================================================================
-- Privacy exemption columns.
-- Users can set these on their own data; admins on any entity. Checked at
-- context building (pre-search/RAG exclusion), tool execution, and field
-- reading. AI can never remove an exemption (enforced in code, §103).
-- All three columns are additive NOT NULL DEFAULT false — zero downtime.
-- =====================================================================

ALTER TABLE contents ADD COLUMN IF NOT EXISTS ai_exempt boolean NOT NULL DEFAULT false;
ALTER TABLE custom_field_values ADD COLUMN IF NOT EXISTS ai_exempt boolean NOT NULL DEFAULT false;
ALTER TABLE users ADD COLUMN IF NOT EXISTS ai_exempt boolean NOT NULL DEFAULT false;

-- =====================================================================
-- Agent session tracking (§103 Schema — Agent Session Tracking).
-- One row per agent request; updated on completion/failure/cancellation.
-- =====================================================================

CREATE TABLE IF NOT EXISTS ai_agent_sessions (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    prompt          text NOT NULL,
    geo_space_id    uuid REFERENCES spaces(id) ON DELETE SET NULL,
    presearch_level text NOT NULL DEFAULT 'none'
                      CHECK (presearch_level IN ('full','partial','none')),
    status          text NOT NULL DEFAULT 'running'
                      CHECK (status IN ('running','completed','failed','cancelled')),
    tool_call_count int  NOT NULL DEFAULT 0,
    result_summary  text,
    error_message   text,
    total_tokens    int  NOT NULL DEFAULT 0,
    estimated_cost  numeric(10,4) NOT NULL DEFAULT 0,
    started_at      timestamptz NOT NULL DEFAULT now(),
    completed_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_agent_sessions_user
    ON ai_agent_sessions (tenant_id, user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_ai_agent_sessions_status
    ON ai_agent_sessions (tenant_id, status, created_at DESC);

-- =====================================================================
-- Agent tool-call audit log (§103 Schema).
-- One row per tool call executed by the agent. session_id CASCADEs so
-- purging a session purges its audit trail. hitl_review_id links to the
-- existing ai_hitl_reviews table (migration 000036) when a call is queued
-- for human review.
-- =====================================================================

CREATE TABLE IF NOT EXISTS ai_agent_tool_calls (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    session_id      uuid NOT NULL REFERENCES ai_agent_sessions(id) ON DELETE CASCADE,
    tool_name       text NOT NULL,
    arguments       jsonb NOT NULL DEFAULT '{}',
    result          jsonb,
    status          text NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','success','error','unauthorized','privacy_blocked','hitl_queued')),
    error_message   text,
    latency_ms      int,
    rbac_checked    boolean NOT NULL DEFAULT true,
    privacy_checked boolean NOT NULL DEFAULT true,
    hitl_review_id  uuid REFERENCES ai_hitl_reviews(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_ai_agent_tool_calls_session
    ON ai_agent_tool_calls (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_ai_agent_tool_calls_tool
    ON ai_agent_tool_calls (tenant_id, tool_name, status, created_at DESC);

-- =====================================================================
-- system_settings: 9 agent.* keys (§103 system_settings table).
-- Seeded against the default tenant (00000000-0000-0000-0000-000000000001)
-- following the established pattern (migration 000169). Values are JSONB
-- literals. agent.external_search_api_key is intentionally empty here —
-- §103 states the real key lives in secret management, not settings.
-- =====================================================================

INSERT INTO system_settings (tenant_id, key, value) VALUES
    ('00000000-0000-0000-0000-000000000001', 'agent.enabled',                    'true'),
    ('00000000-0000-0000-0000-000000000001', 'agent.max_tool_calls',             '10'),
    ('00000000-0000-0000-0000-000000000001', 'agent.timeout_seconds',            '60'),
    ('00000000-0000-0000-0000-000000000001', 'agent.default_persona_id',         '""'),
    ('00000000-0000-0000-0000-000000000001', 'agent.external_search_provider',   '"tavily"'),
    ('00000000-0000-0000-0000-000000000001', 'agent.external_search_api_key',    '""'),
    ('00000000-0000-0000-0000-000000000001', 'agent.presearch_full_threshold',   '0.85'),
    ('00000000-0000-0000-0000-000000000001', 'agent.cost_limit_per_request_usd', '0.50'),
    ('00000000-0000-0000-0000-000000000001', 'agent.guest_enabled',              'false')
ON CONFLICT (tenant_id, key) DO NOTHING;

-- =====================================================================
-- RBAC permission nodes: 4 new nodes per §103 RBAC Permissions.
-- agent.use / agent.draft_request / agent.external_search / content.assign_author
-- Granted to super_admin so the default admin can exercise them. The
-- agent.* nodes are also granted to the 'member' role per the catalog
-- (member+ baseline) so authenticated members can use the agent.
-- =====================================================================

INSERT INTO permissions (key, description) VALUES
    ('agent.use',             'AI Agent — use the agent to perform operations'),
    ('agent.draft_request',   'AI Agent — request content creation as draft for admin review'),
    ('agent.external_search', 'AI Agent — allow AI to search external web sources on user behalf'),
    ('content.assign_author', 'Content — reassign content authorship to another user or persona')
ON CONFLICT (key) DO NOTHING;

-- Grant the four new nodes to super_admin.
-- Note: the roles table is `user_roles` (from 000006_create_rbac_tables.sql);
-- an earlier draft referenced a non-existent `roles` table, which errored
-- on databases that apply migrations transactionally (e.g. the testdb harness).
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'super_admin'
  AND p.key IN ('agent.use', 'agent.draft_request', 'agent.external_search', 'content.assign_author')
ON CONFLICT DO NOTHING;

-- Grant the member-baseline agent.* nodes to the default 'member' role
-- (agent.use / agent.draft_request / agent.external_search per catalog).
-- content.assign_author stays admin-only.
INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM user_roles r
CROSS JOIN permissions p
WHERE r.slug = 'member'
  AND p.key IN ('agent.use', 'agent.draft_request', 'agent.external_search')
ON CONFLICT DO NOTHING;

COMMIT;
