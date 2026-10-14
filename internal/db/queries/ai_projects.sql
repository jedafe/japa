-- ai_projects.sql — AI Projects (Section 103 amendment, 2026-10).
-- ai_projects groups unified conversations and agent sessions under a
-- user-owned collection carrying instructions (merged into the thread's
-- AI context server-side) and pinned picker defaults.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
--      (Amendment — AI Projects)

-- name: CreateAIProject :one
INSERT INTO ai_projects (user_id, name, instructions, default_model,
                         default_tone, default_voice_id, default_tools)
VALUES (@user_id, @name, @instructions, @default_model, @default_tone,
        @default_voice_id, @default_tools)
RETURNING *;

-- Owner-scoped fetch — a foreign id returns no rows (404 semantics).
-- name: GetAIProjectForUser :one
SELECT * FROM ai_projects
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL;

-- Sidebar list, newest first, with member counts for the group header.
-- name: ListAIProjectsForUser :many
SELECT p.*,
    (SELECT COUNT(*) FROM ai_conversations c
     WHERE c.project_id = p.id AND c.deleted_at IS NULL) AS conversation_count,
    (SELECT COUNT(*) FROM ai_agent_sessions s
     WHERE s.project_id = p.id) AS session_count
FROM ai_projects p
WHERE p.user_id = @user_id AND p.deleted_at IS NULL
ORDER BY p.updated_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAIProjectsForUser :one
SELECT COUNT(*) FROM ai_projects
WHERE user_id = @user_id AND deleted_at IS NULL;

-- Full-row update; empty strings clear a field. RETURNING lets the
-- service distinguish updated vs not-found/not-owned.
-- name: UpdateAIProjectForUser :one
UPDATE ai_projects SET
    name             = @name,
    instructions     = @instructions,
    default_model    = @default_model,
    default_tone     = @default_tone,
    default_voice_id = @default_voice_id,
    default_tools    = @default_tools,
    updated_at       = now()
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL
RETURNING *;

-- Soft delete (Rule 11). Members' project_id clears via ON DELETE SET NULL
-- only on a hard delete — a soft delete must un-group explicitly so the
-- sidebar and the context injector stop seeing the project.
-- name: SoftDeleteAIProjectForUser :one
UPDATE ai_projects SET deleted_at = now()
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL
RETURNING id;

-- Soft-deleted conversations keep their historical grouping — only live
-- threads are un-grouped on project delete.
-- name: ClearAIConversationProjectForProject :exec
UPDATE ai_conversations SET project_id = NULL
WHERE project_id = @project_id AND deleted_at IS NULL;

-- name: ClearAIAgentSessionProjectForProject :exec
UPDATE ai_agent_sessions SET project_id = NULL
WHERE project_id = @project_id;

-- =====================================================================
-- Membership assignment (validated for ownership at the service layer)
-- =====================================================================

-- Membership change is organizational, not activity — it must not bump
-- updated_at (the sidebar orders threads by recency).
-- name: SetAIConversationProject :one
UPDATE ai_conversations SET project_id = @project_id
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL
RETURNING id;

-- name: SetAIAgentSessionProject :one
UPDATE ai_agent_sessions SET project_id = @project_id
WHERE id = @id AND user_id = @user_id
RETURNING id;

-- Moving a conversation cascades to its linked sessions — a session's
-- grouping mirrors the thread it audited so /ai/sessions never shows a
-- stale project badge after the conversation is re-grouped.
-- name: SetAIAgentSessionsProjectByConversation :exec
UPDATE ai_agent_sessions SET project_id = @project_id
WHERE conversation_id = @conversation_id;
