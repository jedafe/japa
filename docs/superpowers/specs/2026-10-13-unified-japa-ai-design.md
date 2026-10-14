# Unified Japa AI — Single Chat Interface (JapaBot + AI Agent Merge)

**Status:** Implemented — Phases 1–4 landed (token streaming, cross-surface unification, Honcho backfill)
**Date:** 2026-10-13
**Refs:** §20 (JapaBot), §69 (RAG Knowledge Layer), §103/ADR-111 (AI Agent Executor), §19/ADR-014 (AI consent), ADR-069, ADR-025 (AI context), ADR-114 (persona voices), §42 (feature flags / Control → Extensions)

## 1. Background

The `/ai` page in `apps/main` currently renders two independent AI surfaces behind a `?mode=chat|agent` tab switcher:

| Surface | Endpoint | Transport | Memory/history | Tools |
|---|---|---|---|---|
| JapaBot | `POST /api/v1/ai/chat` | JSON request/response | Honcho context entries grouped by `metadata.conversation_id` (no DB table) | none |
| AI Agent | `POST /api/v1/ai/agent` | SSE `AgentEvent` stream | `ai_agent_sessions` + `ai_agent_tool_calls` (single-turn; `conversation_id` accepted but ignored) | RBAC-filtered catalog from `ai_tools` |

Key facts that shape the design:

- The agent loop (`AgentHandler.HandleAgentRequest` → `runLoop`) already performs a server-side **pre-search** (content FTS + RAG vector retrieval, privacy-filtered) before reasoning — a superset of JapaBot's RAG step.
- `ai_tools` rows carry `is_enabled`, `required_permission`, and `category` — admin-level tool control and grouping already exist.
- `buildToolCatalog` caches the RBAC-filtered catalog per user in `jc:agent:tools:{user_id}` (10 min TTL).
- The agent already writes into the shared §20 budget (`jc:ai:queries`, `jc:ai:cost`) via `Preflight`, in addition to its own `jc:agent:rate`/`jc:agent:cost` counters.
- `user_preferences` exists (dark_mode, language) with an established read-merge-upsert pattern (BUG-003).
- All gateway providers are request/response only — no streaming methods exist.

## 2. Goal

One chat interface ("Japa AI") where:

- Every user gets JapaBot-class conversation (RAG grounding, Honcho memory, citations, model/voice pickers, humanize).
- Users with `agent.use` additionally see **tool controls**: a composer toggle per message, and user-level settings for default mode + per-category/per-tool activation.
- Chat and tool turns share one conversation thread and one history — context flows across mode switches, like ChatGPT/Claude.

Non-goal: changing the LLM gateway contract beyond adding streaming (Phase 3).

## 3. Toggle stack

Four layers; each can only **narrow**, never widen:

1. `feature.japabot` (feature_flags) — surface on/off for everyone. Renamed conceptually to the Japa AI surface flag; keeps reading the same key so no flag migration is needed.
2. `feature.ai_agent` — global tools kill-switch. When off, the surface still works as pure chat for all users (agent loop runs with an empty catalog).
3. `ai_tools.is_enabled` + `required_permission` (RBAC) — admin catalog control. Unchanged.
4. **User layer (new)** — `user_preferences.ai_prefs` defaults + per-message composer override.

`agent.use` RBAC remains the eligibility gate for tools: users without it never see the toggle and always receive an empty catalog.

## 4. Data model (new migrations)

```sql
ai_conversations (
  id uuid PK, user_id uuid NOT NULL,
  title text,                 -- auto-titled from first user message
  created_at, updated_at
)

ai_messages (
  id uuid PK, conversation_id uuid NOT NULL REFERENCES ai_conversations,
  role text NOT NULL CHECK (role IN ('user','assistant')),
  content text NOT NULL,
  model text, voice_id uuid,
  tokens integer DEFAULT 0, cost_usd numeric(10,4) DEFAULT 0,
  agent_session_id uuid REFERENCES ai_agent_sessions,  -- NULL for the user message
  created_at
)

ALTER TABLE ai_agent_sessions
  ADD COLUMN conversation_id uuid REFERENCES ai_conversations,
  ADD COLUMN message_id uuid REFERENCES ai_messages;

ALTER TABLE user_preferences
  ADD COLUMN ai_prefs jsonb NOT NULL DEFAULT '{}';
```

`ai_prefs` shape:

```json
{
  "default_mode": "chat | agent",
  "disabled_categories": ["forum", "content"],
  "disabled_tools": ["content.delete"],
  "enabled_tools": ["content.search"]
}
```

`enabled_tools` acts as a per-tool exception overriding a disabled category. `disabled_tools` always wins.

Semantics:

- **conversation** = user-facing thread (replaces Honcho-entry grouping for new chats).
- **ai_agent_session** = per-turn audit record (tool calls, cost, pre-search level) — one per assistant turn that enters the agent loop, linked via `conversation_id`/`message_id`. Admin dashboards (`/admin/agent/*` handlers) keep working.
- Honcho remains the **long-term memory** store (peer card: `japa_stage`, `japa_intent`, prior_memory injection) but stops being the transcript store for new conversations.

## 5. API surface

```
POST   /api/v1/ai/chat                    → unified endpoint, content-negotiated:
                                            Accept: text/event-stream → SSE stream
                                            otherwise → JSON (engine runs, events drained,
                                            {reply, conversation_id, sources} — same contract as today)
GET    /api/v1/ai/conversations           → new table; union with legacy Honcho entries flagged "legacy"
GET    /api/v1/ai/conversations/{id}      → ordered messages, tool calls inlined via agent_session_id
DELETE /api/v1/ai/conversations/{id}      → deletes thread (Honcho entries for it removed best-effort)
GET    /api/v1/me/ai-preferences          → returns ai_prefs (merged with defaults)
PUT    /api/v1/me/ai-preferences          → validates + upserts ai_prefs (read-merge-upsert, BUG-003 pattern)
GET    /api/v1/ai/agent/capabilities      → extended: tools grouped by category for composer/settings UI
```

Content negotiation keeps `POST /ai/chat` backward-compatible: existing clients sending no `Accept: text/event-stream` get the same JSON response shape as today, now served by the unified engine.

`POST /ai/chat` request body (superset of today's):

```json
{
  "message": "...",
  "conversation_id": "uuid?",
  "model": "...", "voice_id": "uuid?", "humanize": false,
  "tools": {
    "enabled": true,
    "categories": ["content", "calculators"],
    "tools": ["content.search", "forum.threads.create"]
  }
}
```

- `tools` omitted → `ai_prefs` defaults apply.
- `tools.enabled: false` → empty catalog for that turn (pure chat).
- `tools.categories`/`tools.tools` → whitelist within what layers 1–4 already allow.

SSE events (extends `AgentEvent`): existing `session_start`, `progress`, `tool_call_start`, `tool_call_result`, `final_response`, `error`, plus:

- `sources` — RAG citation list from `preSearchResult.RAGChunks` (entity_type, title, slug, url, similarity — same shape as JapaBot's `sources`).
- `conversation` — `{conversation_id}` emitted at `session_start` so the client can attach the thread.
- `message` — persisted `ai_messages.id` for the assistant turn (with `final_response`).

All new routes must be registered in `conventions/api-routes-catalog.md` before implementation (per AGENTS.md).

## 6. Backend changes

### 6.1 Agent loop → unified engine

`HandleAgentRequest` signature gains a request struct:

```go
type ChatRequest struct {
    Prompt         string
    ConversationID *uuid.UUID
    Model, VoiceID string
    Humanize       bool
    ToolOverride   *ToolSelection // nil → user prefs
}
```

Changes inside the loop:

- **Multi-turn**: resolve/create `ai_conversations` row; load last N messages (N from `ai.history_window` system setting, default 20) into `messages[]` after the system prompt; persist user + assistant turns to `ai_messages`. Token-window safety: trim oldest turns when history exceeds a configurable budget.
- **Catalog filter order**: `is_enabled ∩ RBAC` (cached as today) → apply `ai_prefs` → apply message `tools` override. Post-cache filtering keeps the 10-min catalog cache valid.
- **Memory**: reuse `buildUserAIContext` (Honcho `prior_memory` + `ai.context.*` site-activity context) injected into the system prompt; `persistChatTurn` per completed turn (raw user text only — never the RAG-augmented prompt, same pollution guard as today).
- **Citations**: emit `sources` event after pre-search.
- **Model/voice/humanize**: port `applyModelAndVoice`/`resolveVoicePrompt` logic — voice prompt prepends to system prompt; model overrides gateway dispatch category selection; humanize post-processes `final_response` content.

### 6.2 Gating

| Check | Behavior |
|---|---|
| `feature.japabot` off | surface disabled entirely (503/disabled response) |
| `ai.context.use` AND `agent.use` both missing | cannot chat at all (OR-gate — `/ai/agent` historically gates `agent.use` alone, so agent-only users must keep working) |
| `feature.ai_agent` off | surface works, catalog forced empty |
| `agent.use` missing | user chats normally; no tool UI; `tools` request fields ignored (not an error) |

`jc:agent:rate`/`jc:agent:cost` counters remain enforced **only when the resolved catalog is non-empty** (tool turns); pure-chat turns consume only the shared `jc:ai:*` budget — matching today's per-surface accounting.

### 6.3 Legacy endpoints

- `POST /api/v1/ai/chat` — same route serves both clients via content negotiation (§5); the legacy JSON path routes through the unified engine and drains its events into the existing `{reply, conversation_id, sources}` contract.
- `POST /api/v1/ai/agent` — kept for back-compat; maps to the unified engine with `tools.enabled=true`.
- `/ai/agent/sessions*` admin + user history endpoints unchanged.

## 7. Frontend (`apps/main`)

- **`JapaAIChat.svelte`** replaces `JapaBotChat` + `AgentChat` + the `?mode` switcher: transcript renders user/assistant messages, inline tool-call cards (reusing `agentStream.ts` event handling), `sources` citations under assistant replies, cancel, typing indicator.
- **Composer tools popover** (`AiToolsMenu.svelte`): master switch + category chips + expandable per-tool list; initialized from `ai_prefs`, overrides per message. Rendered only when `capabilities.tools.length > 0`.
- **`/settings/ai`** — default mode + the same category/tool toggles, persisted via `/me/ai-preferences`.
- **History sidebar** — unified conversation list; Honcho-era conversations listed under "Previous chats" (read-only).
- `/ai/chat`, `/ai/agent` → 308 redirects to `/ai`; `/ai/sessions` folds into the conversations list.
- All data via API per Rule 16 — no hardcoded categories/tools lists (categories come from `/ai/agent/capabilities`).

## 8. Previously deferred items — now in scope, phased

### Phase 2 — Token-level streaming

- Add `ChatStream` to the gateway contract: each provider adapter (OpenAI, Anthropic, Perplexity, Groq, Xiaomi, Ollama-cloud, Devin) gets a streaming variant using the provider's native stream API, emitting normalized `delta` chunks.
- New SSE event `delta` `{content}` between `session_start`/`final_response`; `final_response` still carries the complete text for non-streaming consumers.
- OpenAI-compatible providers must pass `stream_options: {include_usage: true}` so `jc:ai:cost` accounting stays accurate; providers that can't report usage mid-stream fall back to the existing `CostEstimator`.
- Non-streaming providers keep working — the loop accumulates deltas and falls back to the single-shot path.

### Phase 3 — Cross-surface unification

- **Mobile** (`apps/mobile`): unified chat screen consuming the SSE endpoint (Flutter SSE client), tools toggle per its v2.1 screen contract.
- **MCP** (`internal/mcp`): map unified chat params onto the existing MCP tool surface where applicable.
- **studio-desktop**: same unified endpoint.

### Phase 4 — Legacy conversation migration

- Backfill job: group Honcho entries by `metadata.conversation_id` → insert into `ai_conversations`/`ai_messages` (role from entry kind, `agent_session_id` NULL). Idempotent via a `migrated_at`/marker key so reruns skip migrated threads. Runs as an Asynq task registered in §11 catalog first (Rule 7).

## 9. JEOS / spec updates required before implementation

Per AGENTS.md ("JEOS is always updated before code"):

- New ADR for the merge + new/updated section (or amendment to §103 + §20).
- `conventions/api-routes-catalog.md` — new/changed routes.
- `conventions/valkey-keys-catalog.md` — any new key (e.g. ai prefs cache, if added).
- `conventions/system-settings-catalog.md` — `ai.history_window`, any new limits.
- `sections/03-rbac.md` — confirm `agent.use`/`ai.context.use` semantics under the unified surface.
- `sections/11-job-queue-asynq.md` — Phase 4 migration task name.
- Migration ordering per `schemas/MIGRATION-ORDER.md`; sqlc queries added to `internal/db/queries/` then `make sqlc-gen` (Rule 2).

## 10. Testing

- **Go**: catalog filter matrix (RBAC × prefs × override × flags), multi-turn history injection + window trimming, `sources` event emission, gating matrix (each flag/perm combination), prefs GET/PUT merge, migration backfill idempotency.
- **Frontend**: toggle states render rules (no `agent.use` → no popover), per-message override vs settings default, SSE event → transcript rendering, legacy conversation read-only display.

## 11. Rollout

1. Phase 1: data model + unified endpoint + gating + frontend single chat (feature-flagged; `/ai` page switches when `feature.japa_ai_unified` is on — new flag registered in `features.Registry` per §42, not a system_setting).
2. Phase 2: token streaming.
3. Phase 3: cross-surface.
4. Phase 4: Honcho backfill + legacy endpoint deprecation.
