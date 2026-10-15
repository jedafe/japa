# Design — Background agent turns, return-UX polling, template intent

Date: 2026-10-15
Status: Approved (design review in session)
Surfaces: `/ai` unified surface (main app, port 3000), `internal/ai` agent executor
JEOS refs: §103 (ai-agent-executor), §75 (ai-content-studio), §11 (job-queue — follow-up note only), `conventions/api-routes-catalog.md`

## Problem

`/ai` agent-mode sends are bound to the HTTP request lifetime. `runLoop`
derives `loopCtx` from the request context (`agent.go`), so a client
disconnect cancels in-flight LLM calls and tool execution, and the
`ai_agent_sessions` row is failed. For ops-style usage ("create docs, write
posts, update wiki, fill custom fields") the user wants the agent to finish
server-side and leave a durable success/failure result.

Two related gaps:

- A failed terminal path never appends an assistant message — a detached
  failure would leave the conversation transcript silently unanswered.
- Inserted template text does not steer tool choice: `generate_draft` is
  text-only (renders on screen), while the ops goal needs persisted items.

Out of scope / already done: `agent.skip_hitl` bypass is fully implemented —
the executor honors it for `requires_hitl`/`ForceHITL` tools
(`executor.go`), `content.create_draft` publishes directly for holders
(`tools_content.go`), super_admin holds the node (migration 000616) and the
Admin Group role holds it (000791). Granting the node to other groups is a
RBAC admin action, not code.

## Approach chosen

- **Execution model:** detached goroutine now (in-process); an Asynq
  `ai:agent_run` deferred path is spec'd as a follow-up phase only.
- **Return UX:** client auto-polls a running session and lands the result.
- **Template steering:** an `intent` field on the template baked into the
  inserted text (`preview` | `create`).

## Design

### 1. Detached execution — `internal/ai/agent.go`

- `loopParams` gains `detached bool`. `HandleUnifiedChat` sets
  `detached = len(catalog) > 0` (tools-enabled turn). Pure-chat turns stay
  request-bound — a disconnected conversational reply is discarded, saving
  tokens; a task turn always completes.
- In `runLoop`: when `p.detached`, replace the working context with
  `context.WithoutCancel(ctx)` before deriving `loopCtx =
  WithTimeout(ctx, timeout)`. `WithoutCancel` preserves ctx values (slog
  fields etc.) while dropping request cancellation. The existing
  `agent.timeout_seconds` bound is unchanged — background ≠ unbounded.
- `sendEvent` gains detached behavior: `select { case events <- ev:
  default: }` — events drop silently when the buffer is full / nobody reads.
  Terminal events (`final_response`, `error`) fall back to a short blocking
  grace (~2s) so a connected-but-slow client still receives the end marker.
  Attached turns keep today's `ctx.Done()` select unchanged.
- Persistence paths already use `context.WithoutCancel` (session writes,
  tool-call audit, `AppendAssistantMessage`) — no change.
- Cancellation is unaffected: `isCancelled` polls
  `jc:agent:session:{id}` in Valkey; `POST /ai/agent/sessions/{id}/cancel`
  still aborts a detached run.
- One-running-turn-per-conversation stays: the partial unique index +
  `HasRunningTurn` busy check keep rejecting concurrent sends with 409 —
  a running background turn counts as busy.
- Applies uniformly to the legacy `/ai/agent` path (same `runLoop`, same
  gate on catalog presence).
- **Failure transcript notice:** on `loopErr`, `maxedOut`, and `cancelled`
  terminal paths, when `p.detached` and the turn is unified with a
  conversation, append a best-effort assistant message
  (`AppendAssistantMessage` with `WithoutCancel`, same style as the success
  path) summarizing the failure/cancellation. Attached turns keep SSE-only
  errors. Failures of this write are warn-logged and swallowed.

### 2. Return UX — auto-poll

Backend:

- `internal/db/queries/ai_conversations.sql` gains
  `GetRunningAgentSessionForConversation :one` — `SELECT id FROM
  ai_agent_sessions WHERE conversation_id = @conversation_id AND user_id =
  @user_id AND status = 'running' AND started_at > @running_since` (same
  staleness cutoff semantics as `HasRunningTurnForConversation`). Then
  `make sqlc-gen` (never hand-edit `internal/db/`).
- `serveUnifiedDetail` (`unified_handlers.go`) includes
  `running_session_id` in the conversation detail response when such a row
  exists (absent otherwise — matches `project_id` emission semantics).

Frontend (`JapaAIChat.svelte`):

- Conversation load: `data.running_session_id` set → render a
  "Agent working in background…" indicator on the latest turn area and
  start polling `GET /ai/agent/sessions/{id}` (interval ~3s, single
  in-flight request). The client does not know the server-side timeout, so
  the cap is a fixed 10 minutes; the server-side timeout + stale-session
  sweep bound the run itself.
- Terminal status (`completed`/`failed`/`cancelled`) → stop polling, refetch
  the conversation (assistant message lands), clear the indicator.
- Mid-stream drop: the client already captures `activeSessionId` from the
  `session_start` event; if the SSE stream ends without `final_response`,
  start the same poll on that session id instead of erroring immediately.
- Polling stops on conversation switch and component destroy.

### 3. Template `intent` field

- Frontend `ChatTemplate` gains `intent: 'preview' | 'create'` — normalized
  on load/save (absent/invalid → `preview`), persisted inside the existing
  `chat_templates` opaque blob like `folder`.
- Backend `internal/ai/prefs.go` `ChatTemplate` gains
  `Intent string json:"intent,omitempty"`; handler normalizes to the two
  values (anything else → empty/`preview`), consistent with the folder
  bound already added.
- Editor (`ChatTemplatePanel.svelte`): a compact two-option segmented
  control in the save footer beside Folder — `Preview in chat` /
  `Create content`.
- `buildTemplateInsertText(values, labels, intent)`:
  - `preview` — "present the drafts in chat; do not create content items"
    is added to the spec header so the outcome is deterministic rather than
    left to model discretion.
  - `create` — header instructs persistence: one `content.create_draft`
    call per topic (single) or `content.bulk_create` for the Topics list;
    the rest of the spec body is unchanged.
- `buildTemplateBlock` (attach-mode) switches likewise: `preview` keeps the
  `content.generate_draft` JSON-args block; `create` instructs per-topic
  creation via the create tools (bulk for multiple topics).
- i18n keys added to `en.json`/`fr.json` (`ai.templates.intent*`).

### 4. JEOS updates (before code, per AGENTS rules)

- `sections/103-ai-agent-executor.md`: document detached execution for
  tool-enabled unified turns (SSE = view of the run, not its lifecycle),
  the failure-transcript notice, and `running_session_id` on conversation
  detail.
- `conventions/api-routes-catalog.md`: `GET /ai/conversations/{id}` →
  `running_session_id`; `chat_templates[].intent` field note.
- `sections/11-job-queue-asynq.md` + §103 note: `ai:agent_run` deferred
  background task as a follow-up phase (survives API restart, retriable) —
  documented, not implemented.

## Error handling

- Detached run panic → existing `defer recover()` fails the session and
  tries `sendEvent` — unchanged; with the detached ctx the error event drops
  harmlessly if nobody listens; the failure notice still lands via the new
  transcript write.
- `sendEvent` drops are non-fatal by design (audit is in DB, not the
  stream).
- Poll endpoint failures (401/404/5xx) → back off once, then stop polling
  and clear the indicator; a transient failure must not loop forever.
- Template `intent` round-trip through PUT/GET `/me/ai-preferences` is
  covered by the prefs handler test (same as `folder`).

## Testing

- Go (`internal/ai`):
  - detached turn completes after the request ctx is cancelled (session →
    `completed`, assistant message persisted);
  - `sendEvent` non-blocking under a dead consumer (no goroutine stall);
  - terminal failure appends the failure notice message on detached turns;
  - `running_session_id` present/absent in conversation detail;
  - `intent` normalize + round-trip in prefs tests.
- Frontend (`vitest`): insert-text headers for `preview`/`create`,
  attach-block variants, `intent` normalization.
- Browser: send an agent command → navigate away → return → indicator →
  result lands; `create`-intent template send produces a persisted item
  (direct publish under `agent.skip_hitl`).

## Non-goals

- Asynq `ai:agent_run` task (follow-up phase — spec'd only).
- SSE resume/stream-replay for re-attached clients (needs Valkey event
  buffer on `jc:agent:session:{id}` — evaluated with the Asynq phase).
- Any change to `agent.skip_hitl` seeding or semantics — already live.
