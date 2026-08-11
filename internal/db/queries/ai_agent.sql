-- ai_agent.sql — AI Agent Executor queries (Section 103, ADR-111).
-- Covers agent session lifecycle, per-tool-call audit logging, and the
-- per-entity AI privacy-exemption lookups used by the PrivacyChecker.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
--      japaconnect-engineering/adr/ADR-111-ai-agent-executor.md

-- =====================================================================
-- Agent sessions — lifecycle.
-- One row per agent request. Created at start, updated on
-- completion/failure/cancellation. geo_space_id is nullable (agent may
-- run without a resolved geo context).
-- =====================================================================

-- name: CreateAgentSession :one
INSERT INTO ai_agent_sessions (
    tenant_id, user_id, prompt, geo_space_id, presearch_level, status, started_at
) VALUES (
    @tenant_id, @user_id, @prompt, sqlc.narg(geo_space_id), @presearch_level, @status, now()
)
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM ai_agent_sessions
WHERE id = @id AND tenant_id = @tenant_id;

-- Get a session scoped to the owning user (user-facing detail route
-- enforces ownership at the DB layer rather than only in handler logic).
-- name: GetAgentSessionForUser :one
SELECT * FROM ai_agent_sessions
WHERE id = @id AND tenant_id = @tenant_id AND user_id = @user_id;

-- Mark a session completed. tool_call_count, total_tokens, and
-- estimated_cost are written here (the agent accumulates them in memory
-- during the loop and flushes once at the end).
-- name: CompleteAgentSession :one
UPDATE ai_agent_sessions SET
    status          = 'completed',
    result_summary  = sqlc.narg(result_summary),
    tool_call_count = @tool_call_count,
    total_tokens    = @total_tokens,
    estimated_cost  = @estimated_cost,
    completed_at    = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: FailAgentSession :one
UPDATE ai_agent_sessions SET
    status          = 'failed',
    error_message   = sqlc.narg(error_message),
    tool_call_count = @tool_call_count,
    total_tokens    = @total_tokens,
    estimated_cost  = @estimated_cost,
    completed_at    = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: CancelAgentSession :one
UPDATE ai_agent_sessions SET
    status       = 'cancelled',
    completed_at = now()
WHERE id = @id AND tenant_id = @tenant_id
  AND status = 'running'
RETURNING *;

-- User-facing history (newest first), offset-paginated.
-- name: ListAgentSessionsByUser :many
SELECT * FROM ai_agent_sessions
WHERE tenant_id = @tenant_id AND user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAgentSessionsByUser :one
SELECT COUNT(*) FROM ai_agent_sessions
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- Admin list (newest first), offset-paginated, optional status filter.
-- The optional-filter pattern mirrors SearchContentEmbeddings in rag.sql.
-- name: ListAgentSessionsForAdmin :many
SELECT * FROM ai_agent_sessions
WHERE tenant_id = @tenant_id
  AND (@status_filter::text = '' OR status = @status_filter)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAgentSessionsForAdmin :one
SELECT COUNT(*) FROM ai_agent_sessions
WHERE tenant_id = @tenant_id
  AND (@status_filter::text = '' OR status = @status_filter);

-- =====================================================================
-- Agent tool calls — audit log.
-- One row per tool call. Created when a call is dispatched; updated with
-- result/status/latency once the handler returns. hitl_review_id is set
-- when a call is queued for human review (links to ai_hitl_reviews).
-- =====================================================================

-- name: CreateAgentToolCall :one
INSERT INTO ai_agent_tool_calls (
    tenant_id, session_id, tool_name, arguments, status,
    rbac_checked, privacy_checked
) VALUES (
    @tenant_id, @session_id, @tool_name, @arguments, @status,
    @rbac_checked, @privacy_checked
)
RETURNING *;

-- name: GetAgentToolCall :one
SELECT * FROM ai_agent_tool_calls
WHERE id = @id AND tenant_id = @tenant_id;

-- All tool calls for a session, oldest first (audit trail order).
-- name: ListAgentToolCallsBySession :many
SELECT * FROM ai_agent_tool_calls
WHERE session_id = @session_id AND tenant_id = @tenant_id
ORDER BY created_at ASC;

-- Record the outcome of a tool call. result is nullable jsonb (an error
-- call may have no result payload). hitl_review_id is set only when the
-- call was queued for HITL (status = 'hitl_queued').
-- name: UpdateAgentToolCallResult :one
UPDATE ai_agent_tool_calls SET
    result         = sqlc.narg(result),
    status         = @status,
    error_message  = sqlc.narg(error_message),
    latency_ms     = sqlc.narg(latency_ms),
    hitl_review_id = sqlc.narg(hitl_review_id)
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- =====================================================================
-- Privacy exemption lookups (§103 Privacy Exemption Columns).
-- The PrivacyChecker calls these to decide whether the AI may read/write
-- a given entity. All three are tenant-scoped at the SQL layer for
-- defense-in-depth (BUG-014 pattern): contents and users filter directly
-- on tenant_id; custom_field_values has no tenant_id column, so the lookup
-- JOINs custom_fields to enforce tenancy transitively (same pattern as
-- GetCustomFieldValue above).
-- =====================================================================

-- name: GetContentAIExempt :one
SELECT ai_exempt FROM contents
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetCustomFieldValueAIExempt :one
SELECT cfv.ai_exempt
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.id = @id AND cf.tenant_id = @tenant_id;

-- name: GetUserAIExempt :one
SELECT ai_exempt FROM users
WHERE id = @id AND tenant_id = @tenant_id;
