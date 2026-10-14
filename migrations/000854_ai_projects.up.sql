-- 000854_ai_projects.up.sql
-- AI Projects: named user collections grouping unified conversations and
-- agent sessions, each carrying per-project instructions (merged into the
-- thread's AI context server-side) and pinned picker defaults.
--
-- 1. ai_projects table (soft-deleted per Rule 11).
-- 2. ai_conversations.project_id — nullable membership FK; deleting a
--    project un-groups its threads (SET NULL), it never deletes them.
-- 3. ai_agent_sessions.project_id — same treatment for agent runs.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
--      (Amendment — AI Projects)

BEGIN;

CREATE TABLE ai_projects (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id          uuid        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name             text        NOT NULL DEFAULT '',
    instructions     text        NOT NULL DEFAULT '',
    default_model    text        NOT NULL DEFAULT '',
    default_tone     text        NOT NULL DEFAULT '',
    default_voice_id text        NOT NULL DEFAULT '',
    default_tools    jsonb       NOT NULL DEFAULT '{}'::jsonb,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    deleted_at       timestamptz
);

CREATE INDEX idx_ai_projects_user
    ON ai_projects (user_id, updated_at DESC)
    WHERE deleted_at IS NULL;

ALTER TABLE ai_conversations
    ADD COLUMN project_id uuid REFERENCES ai_projects(id) ON DELETE SET NULL;

CREATE INDEX idx_ai_conversations_project
    ON ai_conversations (project_id)
    WHERE project_id IS NOT NULL;

ALTER TABLE ai_agent_sessions
    ADD COLUMN project_id uuid REFERENCES ai_projects(id) ON DELETE SET NULL;

CREATE INDEX idx_ai_agent_sessions_project
    ON ai_agent_sessions (project_id)
    WHERE project_id IS NOT NULL;

COMMIT;
