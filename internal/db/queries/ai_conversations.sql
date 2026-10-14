-- ai_conversations.sql — Unified Japa AI transcript store (ADR-130).
-- ai_conversations is the user-facing thread; ai_messages its turns.
-- ai_agent_sessions stays the per-turn audit record, linked via
-- conversation_id / message_id.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (ADR-130 amendment)
-- japaconnect-engineering/adr/ADR-130-unified-japa-ai.md

-- =====================================================================
-- Conversations
-- =====================================================================

-- name: CreateAIConversation :one
INSERT INTO ai_conversations (user_id, title) VALUES (@user_id, @title)
RETURNING *;

-- Owner-scoped fetch — a foreign or unknown id returns no rows so the
-- caller can fall back to creating a fresh conversation (legacy clients
-- may send Honcho-era ids that never existed in this table).
-- name: GetAIConversationForUser :one
SELECT * FROM ai_conversations
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL;

-- name: TouchAIConversation :exec
UPDATE ai_conversations SET updated_at = now()
WHERE id = @id AND deleted_at IS NULL;

-- User-facing list, newest-active first, offset-paginated. turn_count +
-- last_message feed the sidebar preview (title is auto-derived from the
-- first turn and often empty for very new threads).
-- name: ListAIConversationsForUser :many
SELECT c.*,
    (SELECT COUNT(*) FROM ai_messages m WHERE m.conversation_id = c.id) AS turn_count,
    COALESCE((SELECT m.content FROM ai_messages m WHERE m.conversation_id = c.id
     ORDER BY m.created_at DESC LIMIT 1), '')::text AS last_message
FROM ai_conversations c
WHERE c.user_id = @user_id AND c.deleted_at IS NULL
ORDER BY c.updated_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAIConversationsForUser :one
SELECT COUNT(*) FROM ai_conversations
WHERE user_id = @user_id AND deleted_at IS NULL;

-- Soft delete (Rule 11). RETURNING id lets the service distinguish
-- "deleted" from "not found / not owned".
-- name: SoftDeleteAIConversationForUser :one
UPDATE ai_conversations SET deleted_at = now()
WHERE id = @id AND user_id = @user_id AND deleted_at IS NULL
RETURNING id;

-- Existence check that ignores deleted_at — used to keep soft-deleted
-- threads out of the legacy Honcho fallback in GET /ai/conversations/:id
-- (a deleted unified thread's memory entries must not resurrect it).
-- name: AIConversationExistsIncludingDeleted :one
SELECT EXISTS(
    SELECT 1 FROM ai_conversations
    WHERE id = @id AND user_id = @user_id
) AS present;

-- =====================================================================
-- Messages
-- =====================================================================

-- name: InsertAIMessage :one
INSERT INTO ai_messages (conversation_id, role, content, model, voice_id,
                         tokens_used, cost_usd, agent_session_id)
VALUES (@conversation_id, @role, @content, sqlc.narg(model),
        sqlc.narg(voice_id), @tokens_used, @cost_usd,
        sqlc.narg(agent_session_id))
RETURNING *;

-- Link an assistant message to its agent audit session after both exist
-- (session is created before the assistant message is persisted).
-- name: SetAIMessageAgentSession :exec
UPDATE ai_messages SET agent_session_id = @agent_session_id
WHERE id = @id;

-- Bounded history window for prompt building — the newest @row_limit
-- messages returned oldest-first. The inner DESC+LIMIT picks the window;
-- the outer ASC restores chronological order.
-- name: ListAIConversationHistory :many
SELECT * FROM (
    SELECT * FROM ai_messages
    WHERE conversation_id = @conversation_id
    ORDER BY created_at DESC
    LIMIT @row_limit
) t
ORDER BY created_at ASC;

-- Full transcript for the detail endpoint, chronological.
-- name: ListAIConversationMessages :many
SELECT * FROM ai_messages
WHERE conversation_id = @conversation_id
ORDER BY created_at ASC
LIMIT @page_limit OFFSET @page_offset;

-- =====================================================================
-- Agent session links (audit trail)
-- =====================================================================

-- After a unified turn, point the session at the conversation and the
-- assistant message it produced.
-- name: LinkAgentSessionToConversation :exec
UPDATE ai_agent_sessions SET
    conversation_id = @conversation_id,
    message_id = @message_id
WHERE id = @id;

-- Conversation deletion keeps the audit rows (Rule 11 carve-out) but
-- severs the link so admin views don't expose a deleted thread.
-- name: UnlinkAgentSessionsForConversation :exec
UPDATE ai_agent_sessions SET conversation_id = NULL
WHERE conversation_id = @conversation_id;

-- ADR-130: is a turn already running on this thread? Concurrent sends
-- against one conversation would interleave their transcript. The
-- @running_since cutoff mirrors agent.timeout_seconds — a "running" row
-- older than the loop's own timeout is dead, not busy.
-- name: HasRunningAgentTurnForConversation :one
SELECT EXISTS(
    SELECT 1 FROM ai_agent_sessions
    WHERE conversation_id = @conversation_id
      AND user_id = @user_id
      AND status = 'running'
      AND started_at > @running_since
) AS busy;

-- =====================================================================
-- Per-user AI preferences (user_preferences.ai_prefs jsonb)
-- =====================================================================


-- name: GetAIPreferences :one
SELECT ai_prefs FROM user_preferences
WHERE user_id = @user_id;

-- Standalone upsert — does not touch dark_mode/language. INSERT supplies
-- only user_id + ai_prefs; the other columns fall back to defaults.
-- name: UpsertAIPreferences :exec
INSERT INTO user_preferences (user_id, ai_prefs) VALUES (@user_id, @ai_prefs)
ON CONFLICT (user_id) DO UPDATE SET
    ai_prefs = EXCLUDED.ai_prefs,
    updated_at = now();
