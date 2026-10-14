-- ai_agent.sql — AI Agent Executor queries (Section 103, ADR-111).
-- Covers agent session lifecycle, per-tool-call audit logging, and the
-- per-entity AI privacy-exemption lookups used by the PrivacyChecker.
--
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md
-- japaconnect-engineering/adr/ADR-111-ai-agent-executor.md

-- =====================================================================
-- Agent sessions — lifecycle.
-- One row per agent request. Created at start, updated on
-- completion/failure/cancellation. geo_space_id is nullable (agent may
-- run without a resolved geo context).
-- =====================================================================

-- name: CreateAgentSession :one
-- conversation_id is the ADR-130 link to the unified transcript thread;
-- NULL for legacy /ai/agent calls that predate or bypass the merge.
INSERT INTO ai_agent_sessions (user_id, prompt, geo_space_id, presearch_level, status, started_at, conversation_id) VALUES (@user_id, @prompt, sqlc.narg(geo_space_id), @presearch_level, @status, now(), sqlc.narg(conversation_id))
RETURNING *;

-- name: GetAgentSession :one
SELECT * FROM ai_agent_sessions
WHERE id = @id;

-- Get a session scoped to the owning user (user-facing detail route
-- enforces ownership at the DB layer rather than only in handler logic).
-- name: GetAgentSessionForUser :one
SELECT * FROM ai_agent_sessions
WHERE id = @id AND user_id = @user_id;

-- Mark a session completed. tool_call_count, total_tokens, and
-- estimated_cost are written here (the agent accumulates them in memory
-- during the loop and flushes once at the end).
-- name: CompleteAgentSession :one
UPDATE ai_agent_sessions SET
 status = 'completed',
 result_summary = sqlc.narg(result_summary),
 tool_call_count = @tool_call_count,
 total_tokens = @total_tokens,
 estimated_cost = @estimated_cost,
 completed_at = now()
WHERE id = @id
RETURNING *;

-- name: FailAgentSession :one
UPDATE ai_agent_sessions SET
 status = 'failed',
 error_message = sqlc.narg(error_message),
 tool_call_count = @tool_call_count,
 total_tokens = @total_tokens,
 estimated_cost = @estimated_cost,
 completed_at = now()
WHERE id = @id
RETURNING *;

-- name: CancelAgentSession :one
UPDATE ai_agent_sessions SET
 status = 'cancelled',
 completed_at = now()
WHERE id = @id
 AND status = 'running'
RETURNING *;

-- ADR-130: sweep "running" rows older than the agent timeout for one
-- conversation. A crashed/killed turn leaves a permanent 'running' row
-- that the one-running-per-conversation unique index keeps enforcing even
-- though the busy pre-check treats it as stale — every subsequent send to
-- that thread would fail with a spurious conversation_busy. Failing rows
-- past the timeout frees the index slot; a genuinely live turn started
-- within the window is untouched.
-- name: FailStaleRunningAgentSessionsForConversation :execrows
UPDATE ai_agent_sessions SET
 status = 'failed',
 error_message = 'agent: session timed out',
 completed_at = now()
WHERE conversation_id = @conversation_id
  AND status = 'running'
  AND started_at < @stale_before;

-- User-facing history (newest first), offset-paginated.
-- name: ListAgentSessionsByUser :many
SELECT * FROM ai_agent_sessions
WHERE user_id = @user_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAgentSessionsByUser :one
SELECT COUNT(*) FROM ai_agent_sessions
WHERE user_id = @user_id;

-- Admin list (newest first), offset-paginated, optional status filter.
-- The optional-filter pattern mirrors SearchContentEmbeddings in rag.sql.
-- name: ListAgentSessionsForAdmin :many
SELECT * FROM ai_agent_sessions
WHERE (@status_filter::text = '' OR status = @status_filter)
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountAgentSessionsForAdmin :one
SELECT COUNT(*) FROM ai_agent_sessions
WHERE (@status_filter::text = '' OR status = @status_filter);

-- =====================================================================
-- Agent tool calls — audit log.
-- One row per tool call. Created when a call is dispatched; updated with
-- result/status/latency once the handler returns. hitl_review_id is set
-- when a call is queued for human review (links to ai_hitl_reviews).
-- =====================================================================

-- name: CreateAgentToolCall :one
INSERT INTO ai_agent_tool_calls (session_id, tool_name, arguments, status, rbac_checked, privacy_checked) VALUES (@session_id, @tool_name, @arguments, @status, @rbac_checked, @privacy_checked)
RETURNING *;

-- name: GetAgentToolCall :one
SELECT * FROM ai_agent_tool_calls
WHERE id = @id;

-- All tool calls for a session, oldest first (audit trail order).
-- name: ListAgentToolCallsBySession :many
SELECT * FROM ai_agent_tool_calls
WHERE session_id = @session_id
ORDER BY created_at ASC;

-- Record the outcome of a tool call. result is nullable jsonb (an error
-- call may have no result payload). hitl_review_id is set only when the
-- call was queued for HITL (status = 'hitl_queued').
-- name: UpdateAgentToolCallResult :one
UPDATE ai_agent_tool_calls SET
 result = sqlc.narg(result),
 status = @status,
 error_message = sqlc.narg(error_message),
 latency_ms = sqlc.narg(latency_ms),
 hitl_review_id = sqlc.narg(hitl_review_id)
WHERE id = @id
RETURNING *;

-- =====================================================================
-- Privacy exemption lookups (§103 Privacy Exemption Columns).
-- The PrivacyChecker calls these to decide whether the AI may read/write
-- a given entity. All three are platform-scoped at the SQL layer for
-- defense-in-depth (BUG-014 pattern): contents and users filter directly
-- on the id; custom_field_values has no the id column, so the lookup
-- JOINs custom_fields to enforce platform scoping transitively (same pattern as
-- GetCustomFieldValue above).
-- =====================================================================

-- name: GetContentAIExempt :one
SELECT ai_exempt FROM contents
WHERE id = @id;

-- name: GetCustomFieldValueAIExempt :one
SELECT cfv.ai_exempt
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.id = @id;

-- name: GetCustomFieldValueEntity :one
-- Returns the entity_type and entity_id for a custom_field_values row,
-- platform-scoped via the custom_fields JOIN. Used by the PUT /me/ai-privacy
-- handler to verify that the calling user owns the entity the field value
-- is attached to before allowing them to set ai_exempt.
-- Ref: japaconnect-engineering/sections/103-ai-agent-executor.md (Privacy Exemption)
SELECT cfv.entity_type, cfv.entity_id
FROM custom_field_values cfv
JOIN custom_fields cf ON cfv.field_id = cf.id
WHERE cfv.id = @id;

-- name: GetUserAIExempt :one
SELECT ai_exempt FROM users
WHERE id = @id;

-- =====================================================================
-- Privacy exemption writes (§103 Privacy Exemption Columns).
-- Used by the user-facing PUT /me/ai-privacy route (Phase 6). A user may
-- only set the flag on their OWN data; the handler enforces ownership
-- (entity ID == authenticated user ID for "profile", or entity author ==
-- user for "content"/"custom_field_value") before calling these. All three
-- are platform-scoped at the SQL layer for defense-in-depth (BUG-014).
-- =====================================================================

-- name: SetContentAIExempt :one
UPDATE contents SET ai_exempt = @ai_exempt, updated_at = now()
WHERE id = @id AND deleted_at IS NULL
RETURNING id, author_id, ai_exempt;

-- name: SetCustomFieldValueAIExempt :one
-- custom_field_values has no the id column; JOIN custom_fields to
-- enforce platform scoping transitively (same pattern as GetCustomFieldValueAIExempt).
UPDATE custom_field_values cfv
SET ai_exempt = @ai_exempt
FROM custom_fields cf
WHERE cfv.field_id = cf.id
 AND cfv.id = @id
RETURNING cfv.id, cfv.field_id, cfv.ai_exempt;

-- name: SetUserAIExempt :one
UPDATE users SET ai_exempt = @ai_exempt
WHERE id = @id
RETURNING id, ai_exempt;

-- =====================================================================
-- Admin aggregates (Phase 6 — GET /admin/agent/tool-stats, cost-report).
-- =====================================================================

-- Per-tool usage statistics: call count, success rate, average latency.
-- Filters by platform and an optional date range (from_date / to_date, both
-- nullable). Success rate is the fraction of calls with status = 'success'.
-- name: GetAgentToolStats :many
SELECT tool_name,
 COUNT(*) AS total_calls,
 COUNT(*) FILTER (WHERE status = 'success') AS success_count,
 COUNT(*) FILTER (WHERE status = 'error') AS error_count,
 COUNT(*) FILTER (WHERE status = 'unauthorized') AS unauthorized_count,
 COUNT(*) FILTER (WHERE status = 'privacy_blocked') AS privacy_blocked_count,
 COUNT(*) FILTER (WHERE status = 'hitl_queued') AS hitl_queued_count,
 COALESCE(AVG(latency_ms), 0) AS avg_latency_ms,
 COALESCE(MAX(latency_ms), 0) AS max_latency_ms
FROM ai_agent_tool_calls
WHERE (@from_date::timestamptz IS NULL OR created_at >= @from_date)
 AND (@to_date::timestamptz IS NULL OR created_at < @to_date)
GROUP BY tool_name
ORDER BY total_calls DESC;

-- Cost breakdown by user for the admin cost report. Joins
-- ai_agent_sessions to users to surface the username. Filters by platform
-- and an optional date range. estimated_cost is numeric(10,4); we cast to
-- double precision for JSON-friendly aggregation.
-- name: GetAgentCostReport :many
SELECT s.user_id,
 u.username,
 COUNT(*) AS session_count,
 COALESCE(SUM(s.total_tokens), 0) AS total_tokens,
 COALESCE(SUM(s.tool_call_count), 0) AS total_tool_calls,
 COALESCE(SUM(CAST(s.estimated_cost AS double precision)), 0) AS total_cost_usd
FROM ai_agent_sessions s
LEFT JOIN users u ON u.id = s.user_id
WHERE (@from_date::timestamptz IS NULL OR s.created_at >= @from_date)
 AND (@to_date::timestamptz IS NULL OR s.created_at < @to_date)
GROUP BY s.user_id, u.username
ORDER BY total_cost_usd DESC;
