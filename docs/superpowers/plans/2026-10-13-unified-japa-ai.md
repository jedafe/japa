# Unified Japa AI — Phase 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Merge JapaBot (`/ai/chat`) and the AI Agent (`/ai/agent`) into one SSE-capable conversational endpoint backed by the agent loop, with per-user and per-message tool toggles.

**Architecture:** The agent loop becomes the single chat engine. A new `ai_conversations`/`ai_messages` pair gives multi-turn history (replacing Honcho-entry grouping for new chats); `ai_agent_sessions` stays the per-turn audit record linked to the conversation. Tool visibility = `ai_tools.is_enabled ∩ RBAC` (cached) ∩ `user_preferences.ai_prefs` ∩ per-message `tools` override. `POST /ai/chat` is content-negotiated: `Accept: text/event-stream` → SSE, otherwise the legacy JSON shape drained from the event stream.

**Tech Stack:** Go, chi, pgx/v5, sqlc `gen.Queries`, Valkey, Honcho, SvelteKit 2 + Svelte 5 + Tailwind v4.

**Spec:** `docs/superpowers/specs/2026-10-13-unified-japa-ai-design.md`

**Scope note:** Phases 2–4 of the spec (token streaming, cross-surface, Honcho backfill) are separate plans. This plan ends with the merged chat working end-to-end behind `feature.japa_ai_unified`.

## Global Constraints

- **JEOS first** (AGENTS.md): catalog/section updates in Task 1 land before any code that depends on them.
- **Rule 2:** Never hand-edit `internal/db/gen/*` — edit `internal/db/queries/*.sql`, then `make sqlc-gen`.
- **Rule 3:** No hardcoded tunables — history window reads `ai.history_window` (default 20) via the settings reader.
- **Rule 4:** No new Valkey keys in this phase (prefs live in Postgres; catalog cache reuses `jc:agent:tools:{user_id}`).
- **Rule 5:** Errors via `errpkg.WriteError(w, status, "snake_case_code", msg)` / `errpkg.WriteJSON`.
- **Rule 11:** No hard deletes of user content — `DELETE /ai/conversations/{id}` deletes `ai_conversations`/`ai_messages` rows (these are AI transcripts, not user content; hard delete is correct and required for GDPR erasure semantics), but `ai_agent_sessions` rows are kept with `conversation_id` set to NULL for audit retention.
- **Rule 13 / circular FK:** `ai_messages.agent_session_id → ai_agent_sessions` and `ai_agent_sessions.message_id → ai_messages` would be circular. Resolution: `ai_agent_sessions.message_id` is a plain `uuid` column (no FK constraint); only `conversation_id` gets a real FK.
- **Rule 17:** No AI attribution anywhere — plain commit messages, no "Generated with" trailers, no `// AI:` comments.
- **Rule 18:** Frontend tooling is `bun`/`bunx`/`bun run` only.
- Feature toggles live in `feature_flags` via `features.Registry` — never mint a `system_settings *.enabled` key. New flag: `feature.japa_ai_unified`.
- Tests follow the existing stub-based patterns in `internal/ai/agent_test.go` / `agent_handlers_test.go` — hand-rolled stub stores, `httptest`, no testdb.
- Commit to `main`. Do not create branches.

---

### Task 1: JEOS catalog + section updates (docs before code)

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/conventions/system-settings-catalog.md`
- Modify: `.jeos/sections/103-ai-agent-executor.md` (append "Unified Chat Merge" amendment)
- Modify: `.jeos/sections/20-*.md` (JapaBot section — append note that transcripts move to `ai_conversations`/`ai_messages`)
- Create: `.jeos/adr/` ADR file for the merge (follow existing ADR numbering — check `ls .jeos/adr | tail` for the next number)

- [ ] **Step 1: Append routes to `api-routes-catalog.md`**

Find the `/api/v1/ai/chat` row and the `/api/v1/ai/conversations` rows; update/append:

```markdown
POST   /api/v1/ai/chat                    unified chat — SSE when Accept: text/event-stream, else JSON {reply, conversation_id, sources} (auth, ai.context.use, feature.japabot)
GET    /api/v1/ai/conversations           list conversations — ai_conversations ∪ legacy Honcho entries (auth, ai.context.use, feature.japabot)
GET    /api/v1/ai/conversations/{id}      conversation detail — messages + inlined tool calls (auth, ai.context.use, feature.japabot)
DELETE /api/v1/ai/conversations/{id}      delete conversation thread (auth, ai.context.use, feature.japabot)
GET    /api/v1/me/ai-preferences          read ai_prefs (auth)
PUT    /api/v1/me/ai-preferences          upsert ai_prefs (auth)
```

- [ ] **Step 2: Append settings to `system-settings-catalog.md`**

In the AI section near `ai.daily_cost_limit_usd`:

```markdown
| `ai.history_window` | `20` | int | Max prior messages loaded into the unified chat prompt |
| `ai.conversation_title_length` | `80` | int | Chars of first user message used as auto-title |
```

- [ ] **Step 3: Mirror both settings into `internal/control/settings_catalog.go`**

Add entries to the catalog map (panel "control", matching existing `ai.*` rows around line 142):

```go
"ai.history_window":             {Type: "int", DefaultValue: "20", Description: "Max prior messages loaded into the unified chat prompt.", Panel: "control"},
"ai.conversation_title_length":  {Type: "int", DefaultValue: "80", Description: "Chars of first user message used as auto-title.", Panel: "control"},
```

- [ ] **Step 4: Amend §103 section + write the ADR**

Append to `.jeos/sections/103-ai-agent-executor.md`:

```markdown
## Amendment — Unified Japa AI Merge (2026-10)

The agent loop becomes the single chat engine for POST /ai/chat. Users without
agent.use receive an empty tool catalog (loop degenerates to JapaBot-class chat:
pre-search RAG + Honcho memory, no tools). feature.ai_agent off → catalog forced
empty globally while chat keeps working. Conversations persist in
ai_conversations/ai_messages; ai_agent_sessions gains conversation_id/message_id
as the per-turn audit link. Tool visibility = is_enabled ∩ RBAC ∩
user_preferences.ai_prefs ∩ per-message tools override. Design:
docs/superpowers/specs/2026-10-13-unified-japa-ai-design.md
```

Create the ADR in `.jeos/adr/` capturing: decision (merge into agent loop), why (single context across mode switches), alternatives rejected (dual-engine facade, frontend-only merge), consequences (new tables, flag consolidation).

- [ ] **Step 5: Commit**

```bash
git add .jeos/ internal/control/settings_catalog.go
git commit -m "docs(jeos): unified Japa AI routes, settings, §103 amendment, merge ADR"
```

---

### Task 2: Migration 000851 — conversations, messages, session link, ai_prefs

**Files:**
- Create: `migrations/000851_unified_ai_chat.up.sql`
- Create: `migrations/000851_unified_ai_chat.down.sql`

**Interfaces:**
- Produces: tables `ai_conversations`, `ai_messages`; columns `ai_agent_sessions.conversation_id` (FK), `ai_agent_sessions.message_id` (plain uuid, no FK — Rule 13 circular-FK resolution), `user_preferences.ai_prefs jsonb`.

- [ ] **Step 1: Write the up migration**

```sql
-- 000851_unified_ai_chat.up.sql — Unified Japa AI merge (§103 amendment,
-- spec docs/superpowers/specs/2026-10-13-unified-japa-ai-design.md).

CREATE TABLE ai_conversations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title       text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_conversations_user ON ai_conversations (user_id, updated_at DESC);

CREATE TABLE ai_messages (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id   uuid NOT NULL REFERENCES ai_conversations(id) ON DELETE CASCADE,
    role              text NOT NULL CHECK (role IN ('user', 'assistant')),
    content           text NOT NULL,
    model             text,
    voice_id          uuid,
    tokens            integer NOT NULL DEFAULT 0,
    cost_usd          numeric(10,4) NOT NULL DEFAULT 0,
    agent_session_id  uuid,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_ai_messages_conversation ON ai_messages (conversation_id, created_at);

-- Per-turn audit link. conversation_id gets a real FK; message_id stays a
-- plain uuid (no FK) — a FK here would be circular with
-- ai_messages.agent_session_id (Rule 13 pattern).
ALTER TABLE ai_agent_sessions
    ADD COLUMN conversation_id uuid REFERENCES ai_conversations(id) ON DELETE SET NULL,
    ADD COLUMN message_id uuid;

CREATE INDEX idx_ai_agent_sessions_conversation ON ai_agent_sessions (conversation_id);

ALTER TABLE user_preferences
    ADD COLUMN ai_prefs jsonb NOT NULL DEFAULT '{}'::jsonb;
```

Then add the deferred FK on `ai_messages.agent_session_id` AFTER `ai_agent_sessions` already exists (it does — baseline), so this can be a normal FK:

```sql
ALTER TABLE ai_messages
    ADD CONSTRAINT ai_messages_agent_session_fk
    FOREIGN KEY (agent_session_id) REFERENCES ai_agent_sessions(id) ON DELETE SET NULL;
```

- [ ] **Step 2: Write the down migration**

```sql
ALTER TABLE ai_messages DROP CONSTRAINT IF EXISTS ai_messages_agent_session_fk;
ALTER TABLE user_preferences DROP COLUMN IF EXISTS ai_prefs;
ALTER TABLE ai_agent_sessions DROP COLUMN IF EXISTS message_id;
ALTER TABLE ai_agent_sessions DROP COLUMN IF EXISTS conversation_id;
DROP TABLE IF EXISTS ai_messages;
DROP TABLE IF EXISTS ai_conversations;
```

- [ ] **Step 3: Apply + verify**

```bash
make migrate-up 2>/dev/null || go run ./cmd/migrate up   # use the repo's migration entrypoint
psql "$JC_DATABASE_URL" -c '\d ai_messages'              # columns + FK present
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000851_unified_ai_chat.*
git commit -m "feat(ai): migration 000851 — ai_conversations, ai_messages, session link, user ai_prefs"
```

---

### Task 3: sqlc queries + regen

**Files:**
- Create: `internal/db/queries/ai_conversations.sql`
- Modify: `internal/db/queries/ai_agent.sql` (add link query)
- Modify: `internal/db/queries/users.sql` (ai_prefs get/set)
- Regenerate: `internal/db/gen/*` via `make sqlc-gen`

**Interfaces:**
- Produces (names later tasks call):
  - `CreateAIConversation(ctx, {UserID, Title}) (AiConversation, error)`
  - `GetAIConversationForUser(ctx, {ID, UserID}) (AiConversation, error)`
  - `ListAIConversationsByUser(ctx, {UserID, Limit, Offset}) ([]AiConversation, error)`
  - `TouchAIConversation(ctx, {ID, Title})` — bumps updated_at (+ optional title set-once)
  - `DeleteAIConversation(ctx, {ID, UserID})`
  - `CreateAIMessage(ctx, {ConversationID, Role, Content, Model, VoiceID, Tokens, CostUsd, AgentSessionID}) (AiMessage, error)`
  - `ListAIMessages(ctx, {ConversationID, Limit}) ([]AiMessage)` — newest-last window for prompt building
  - `ListAIMessagesForDisplay(ctx, {ConversationID}) ([]AiMessage)` — full ascending list
  - `LinkAgentSessionToConversation(ctx, {ID, ConversationID, MessageID})`
  - `GetUserAIPrefs(ctx, userID) ([]byte, error)`
  - `UpsertUserAIPrefs(ctx, {UserID, AiPrefs})`

- [ ] **Step 1: Write `internal/db/queries/ai_conversations.sql`**

```sql
-- name: CreateAIConversation :one
INSERT INTO ai_conversations (user_id, title)
VALUES (@user_id, @title)
RETURNING *;

-- name: GetAIConversationForUser :one
SELECT * FROM ai_conversations
WHERE id = @id AND user_id = @user_id;

-- name: ListAIConversationsByUser :many
SELECT * FROM ai_conversations
WHERE user_id = @user_id
ORDER BY updated_at DESC
LIMIT @limit OFFSET @offset;

-- name: TouchAIConversation :exec
UPDATE ai_conversations
SET updated_at = now(),
    title = CASE WHEN title IS NULL THEN @title ELSE title END
WHERE id = @id;

-- name: DeleteAIConversation :exec
DELETE FROM ai_conversations
WHERE id = @id AND user_id = @user_id;

-- name: CreateAIMessage :one
INSERT INTO ai_messages
    (conversation_id, role, content, model, voice_id, tokens, cost_usd, agent_session_id)
VALUES
    (@conversation_id, @role, @content, @model, @voice_id, @tokens, @cost_usd, @agent_session_id)
RETURNING *;

-- name: ListAIMessages :many
-- Prompt-building window: newest N messages, returned oldest-first.
SELECT * FROM (
    SELECT * FROM ai_messages
    WHERE conversation_id = @conversation_id
    ORDER BY created_at DESC
    LIMIT @limit
) recent
ORDER BY created_at;

-- name: ListAIMessagesForDisplay :many
SELECT * FROM ai_messages
WHERE conversation_id = @conversation_id
ORDER BY created_at;
```

- [ ] **Step 2: Append to `internal/db/queries/ai_agent.sql`**

```sql
-- name: LinkAgentSessionToConversation :exec
UPDATE ai_agent_sessions
SET conversation_id = @conversation_id,
    message_id = @message_id
WHERE id = @id;
```

- [ ] **Step 3: Append to `internal/db/queries/users.sql`**

```sql
-- name: GetUserAIPrefs :one
SELECT ai_prefs FROM user_preferences WHERE user_id = @user_id;

-- name: UpsertUserAIPrefs :exec
INSERT INTO user_preferences (user_id, ai_prefs)
VALUES (@user_id, @ai_prefs)
ON CONFLICT (user_id) DO UPDATE SET
    ai_prefs = EXCLUDED.ai_prefs,
    updated_at = now();
```

- [ ] **Step 4: Regenerate + build**

```bash
make sqlc-gen
go build ./internal/db/gen/ ./internal/ai/
```

Expected: `AiConversation`, `AiMessage` models + query methods exist; `UserPreference` gains `AiPrefs []byte`.

- [ ] **Step 5: Commit**

```bash
git add internal/db/ migrations/
git commit -m "feat(ai): sqlc queries for unified conversations/messages + ai_prefs"
```

---

### Task 4: `AIPrefs` + tool selection resolution

**Files:**
- Create: `internal/ai/prefs.go`
- Test: `internal/ai/prefs_test.go`

**Interfaces:**
- Produces:
  - `type AIPrefs struct { DefaultMode string; DisabledCategories, DisabledTools, EnabledTools []string }`
  - `func ParseAIPrefs(raw []byte) AIPrefs` — nil/empty → zero value (`DefaultMode "chat"`)
  - `func (p AIPrefs) AllowsTool(functionName, category string) bool`
  - `type ToolSelection struct { Enabled *bool; Categories []string; Tools []string }` — `Enabled` tri-state: nil = inherit prefs, false = force-off, true = force-on (still bounded by prefs/RBAC — an override can only narrow the RBAC set, except `enabled:false` which empties it)
  - `func FilterCatalog(tools []ToolConfig, categories map[string]string, prefs AIPrefs, sel *ToolSelection) []ToolConfig`

- [ ] **Step 1: Write the failing test**

```go
func TestFilterCatalog_PrefsAndOverride(t *testing.T) {
	cat := map[string]string{
		"content.search": "content", "content.delete": "content",
		"forum.threads.create": "forum", "crs.calculate": "calculators",
	}
	tools := []ToolConfig{
		{FunctionName: "content.search"}, {FunctionName: "content.delete"},
		{FunctionName: "forum.threads.create"}, {FunctionName: "crs.calculate"},
	}
	prefs := AIPrefs{
		DisabledCategories: []string{"forum"},
		DisabledTools:      []string{"content.delete"},
	}

	got := FilterCatalog(tools, cat, prefs, nil)
	names := []string{got[0].FunctionName, got[1].FunctionName}
	if len(got) != 2 || names[0] != "content.search" || names[1] != "crs.calculate" {
		t.Fatalf("prefs filter wrong: %+v", got)
	}

	// Per-tool exception: re-enable one tool inside a disabled category.
	prefs2 := AIPrefs{DisabledCategories: []string{"content"}, EnabledTools: []string{"content.search"}}
	got = FilterCatalog(tools, cat, prefs2, nil)
	if len(got) != 2 { // content.search + crs.calculate
		t.Fatalf("exception filter wrong: %+v", got)
	}

	// Message override disables everything.
	off := false
	got = FilterCatalog(tools, cat, prefs, &ToolSelection{Enabled: &off})
	if len(got) != 0 {
		t.Fatalf("enabled:false should empty catalog, got %+v", got)
	}

	// Override whitelist narrows further.
	on := true
	got = FilterCatalog(tools, cat, AIPrefs{}, &ToolSelection{Enabled: &on, Categories: []string{"content"}})
	for _, tc := range got {
		if cat[tc.FunctionName] != "content" {
			t.Fatalf("override leaked tool %s", tc.FunctionName)
		}
	}
}
```

- [ ] **Step 2: Run to verify fail** — `go test ./internal/ai/ -run TestFilterCatalog -v` → undefined: FilterCatalog.

- [ ] **Step 3: Implement `internal/ai/prefs.go`**

```go
package ai

import "encoding/json"

// AIPrefs is the user_preferences.ai_prefs JSONB shape (§103 amendment).
// All fields optional; empty = default (chat default mode, no restrictions).
type AIPrefs struct {
	DefaultMode        string   `json:"default_mode,omitempty"` // "chat" | "agent"
	DisabledCategories []string `json:"disabled_categories,omitempty"`
	DisabledTools      []string `json:"disabled_tools,omitempty"`
	EnabledTools       []string `json:"enabled_tools,omitempty"` // per-tool exceptions vs disabled_categories
}

func ParseAIPrefs(raw []byte) AIPrefs {
	var p AIPrefs
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &p) // malformed prefs degrade to defaults
	}
	return p
}

func (p AIPrefs) AllowsTool(functionName, category string) bool {
	for _, t := range p.DisabledTools {
		if t == functionName {
			return false
		}
	}
	for _, c := range p.DisabledCategories {
		if c == category {
			for _, t := range p.EnabledTools {
				if t == functionName {
					return true // per-tool exception
				}
			}
			return false
		}
	}
	return true
}

// ToolSelection is the per-message `tools` override on POST /ai/chat.
type ToolSelection struct {
	Enabled    *bool    `json:"enabled,omitempty"`
	Categories []string `json:"categories,omitempty"`
	Tools      []string `json:"tools,omitempty"`
}

// FilterCatalog applies prefs then the message override to the RBAC-filtered
// catalog. Override whitelists can only narrow what prefs/RBAC already allow.
func FilterCatalog(tools []ToolConfig, categories map[string]string, prefs AIPrefs, sel *ToolSelection) []ToolConfig {
	if sel != nil && sel.Enabled != nil && !*sel.Enabled {
		return nil
	}
	out := tools[:0:0]
	for _, tc := range tools {
		if !prefs.AllowsTool(tc.FunctionName, categories[tc.FunctionName]) {
			continue
		}
		if sel != nil {
			if len(sel.Categories) > 0 && !contains(sel.Categories, categories[tc.FunctionName]) {
				continue
			}
			if len(sel.Tools) > 0 && !contains(sel.Tools, tc.FunctionName) {
				continue
			}
		}
		out = append(out, tc)
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run test → PASS.** Commit: `feat(ai): ai_prefs parsing + per-message tool selection filter`

---

### Task 5: Tool categories on the registry + grouped capabilities

**Files:**
- Modify: `internal/ai/executor.go` (loadedTool gains `category`; `RegisterConfigFull`; `CatalogForUser` unchanged; new `CatalogEntriesForUser`)
- Modify: `internal/ai/agent_handlers.go` (`HandleGetCapabilities` returns grouped categories)

**Interfaces:**
- Produces:
  - `func (r *ToolHandlerRegistry) RegisterConfigFull(name string, cfg ToolConfig, requiredPermission string, toolID pgtype.UUID, category string)`
  - `func (r *ToolHandlerRegistry) Categories() map[string]string` — functionName → category
  - capabilities response gains `categories: [{name, tools: [{name, description}]}]` alongside the flat `tools` array (back-compat).

- [ ] **Step 1: Failing test** — assert `Categories()["content.search"] == "content"` after `RegisterConfigFull`. Run → fail.
- [ ] **Step 2: Implement** — add `category string` to `loadedTool`; keep `RegisterConfig` delegating to `RegisterConfigFull` with `""`; `LoadFromDB` calls `RegisterConfigFull(..., t.Category)`.
- [ ] **Step 3: Capabilities handler** — in `HandleGetCapabilities` (agent_handlers.go:281), group the user's catalog by `registry.Categories()[fn]` into the new `categories` array. Keep `tools` field unchanged.
- [ ] **Step 4: Run tests → PASS.** Commit: `feat(ai): tool categories in registry + grouped capabilities response`

---

### Task 6: `ConversationService` — transcript store

**Files:**
- Create: `internal/ai/conversations.go`
- Test: `internal/ai/conversations_test.go`

**Interfaces:**
- Consumes: Task 3 gen methods.
- Produces:
  - `type ConversationStore interface` — subset of gen.Queries (the Task 3 methods); `*gen.Queries` satisfies it.
  - `type ConversationService struct` with:
    - `Resolve(ctx, userID pgtype.UUID, conversationID *pgtype.UUID, title string) (gen.AiConversation, error)` — get-or-create; `title` is the already-truncated auto-title (caller truncates `firstMessage` to `ai.conversation_title_length` chars; on create, title is stored; on existing conversation it's ignored).
    - `AppendUserMessage(ctx, convID, content) (gen.AiMessage, error)`
    - `AppendAssistantMessage(ctx, convID, content, model string, tokens int, cost float64, sessionID *pgtype.UUID) (gen.AiMessage, error)`
    - `History(ctx, convID, limit int) ([]ChatMessage, error)` — maps rows to `ChatMessage{Role, Content}` oldest-first
    - `List(ctx, userID, limit, offset) ([]gen.AiConversation, error)`
    - `Detail(ctx, userID, convID) (gen.AiConversation, []gen.AiMessage, error)` — 404 on wrong owner
    - `Delete(ctx, userID, convID) error` — also NULLs `ai_agent_sessions.conversation_id` (audit retention, Rule 11 carve-out)
    - `Prefs(ctx, userID) (AIPrefs, error)` / `SavePrefs(ctx, userID, AIPrefs) error`

- [ ] **Step 1: Failing tests** — stub `ConversationStore` (map-backed, mirroring `agent_test.go` stub style); cover: Resolve creates once then reuses, history window ordering (insert 25, window 20 → oldest 5 dropped), owner-scoping 404, auto-title truncation, prefs round-trip.
- [ ] **Step 2: Run → fail** (types undefined).
- [ ] **Step 3: Implement** — thin wrapper over the store; `floatToNumeric` already exists in package for cost.
- [ ] **Step 4: Run → PASS.** Commit: `feat(ai): conversation service — threads, messages, history window, prefs`

---

### Task 7: Unified request plumbing on `AgentHandler`

**Files:**
- Modify: `internal/ai/agent.go`
- Test: `internal/ai/agent_test.go` (extend)

**Interfaces:**
- Consumes: `ConversationService`, `FilterCatalog`, `ToolSelection`, `registry.Categories()`.
- Produces:
  - `type UnifiedChatRequest struct { Prompt string; ConversationID *pgtype.UUID; Model, VoiceID string; Humanize bool; Tools *ToolSelection }`
  - `func (h *AgentHandler) HandleUnifiedChat(ctx context.Context, userID pgtype.UUID, req UnifiedChatRequest) (<-chan AgentEvent, error)` — superset of `HandleAgentRequest`; the old method becomes a thin wrapper: `HandleUnifiedChat(ctx, userID, UnifiedChatRequest{Prompt: prompt, Tools: &ToolSelection{Enabled: &truth}})`.
  - New event types: `EventSources = "sources"` (payload `Sources []map[string]any` on AgentEvent — add field), `EventConversation = "conversation"` (reuse `SessionID` field for session + add `ConversationID`, `MessageID` fields to `AgentEvent`).
  - `WithAgentConversations(svc *ConversationService) AgentOption`, `WithAgentAIContext(fn func(ctx, userID, clientCtx) map[string]any) AgentOption` (injects `buildUserAIContext` from handlers.go — avoids an import cycle since both live in package `ai`, a plain func field works), `WithAgentChatPersist(fn func(ctx, userID, msg, reply, convID string)) AgentOption` (injects `persistChatTurn`).

- [ ] **Step 1: Failing test** — multi-turn: run `HandleUnifiedChat` twice on the same conversation with a stub gateway that records `req.Messages`; assert second call's messages contain turn-1 user+assistant. Also: `Tools{Enabled:false}` → `ChatWithToolsRequest.Tools` empty; `sources` event emitted before `final_response`; `conversation` event carries `conversation_id`.
- [ ] **Step 2: Run → fail.**
- [ ] **Step 3: Implement** — in `HandleUnifiedChat`, after the existing pre-checks (`agentEnabled` skipped when it would force-empty the catalog rather than erroring — see below), add:
  1. Surface gate: `feature.japabot` must be on (reuse checker; error `errAgentDisabled` maps to `chat_disabled` — add sentinel or reuse with new code).
  2. Chat permission: `ai.context.use` required (mirrors route middleware — the handler must still check because `/ai/agent` adapter path may differ).
  3. Tool eligibility: `agent.use` AND `feature.ai_agent` — if either fails, catalog = empty (no error).
  4. Resolve conversation + append user message; load `History` (window = `ai.history_window`, default 20).
  5. `runLoop` changes: `messages = system + history + current user`; build catalog → `FilterCatalog(tools, registry.Categories(), prefs, req.Tools)`; emit `conversation` event (with session_start), emit `sources` after pre-search (map `RAGChunks` → citation shape: entity_type, entity_id, title, slug, url, similarity, tier).
  6. Voice prompt (reuse `resolveVoicePrompt` via injected resolver) prepends to system prompt; `req.Model` → `ChatWithToolsRequest.Model`; `req.Humanize` → apply existing humanize post-processing to `finalResponse` before emit.
  7. On completion: `AppendAssistantMessage` + `LinkAgentSessionToConversation` + `TouchAIConversation` + injected `persistChatTurn` (Honcho, best-effort).
- [ ] **Step 4: Run → PASS.** Commit: `feat(ai): unified chat engine — multi-turn, prefs/override tool filter, citations + conversation events, memory`

---

### Task 8: HTTP layer — `POST /ai/chat` (SSE + JSON), prefs, conversations endpoints

**Files:**
- Create: `internal/ai/unified_handlers.go`
- Modify: `internal/ai/agent_handlers.go` (`HandleAgent` delegates to unified)
- Test: `internal/ai/unified_handlers_test.go`

**Interfaces:**
- Produces:
  - `func (h *AgentHTTPHandlers) HandleUnifiedChat(w, r)` — body `{message, conversation_id?, model?, voice_id?, humanize?, tools?}`; `Accept: text/event-stream` → SSE drain (same loop as `HandleAgent`); else drain events → `{data:{reply, conversation_id, sources}}` (legacy §20 shape).
  - `func (h *AgentHTTPHandlers) HandleGetAIPrefs(w, r)` / `HandlePutAIPrefs(w, r)` — validate: `default_mode ∈ {chat, agent}`; categories/tools arrays of strings; unknown categories allowed (forward-compat).
  - `func (h *AgentHTTPHandlers) HandleListConversationsV2` / `HandleGetConversationV2` / `HandleDeleteConversation` — list unions `ConversationService.List` with Honcho-derived legacy summaries (each entry gets `"legacy": true`); detail returns `{conversation, messages}` where each assistant message embeds its tool calls via `ListAgentToolCallsBySession(agent_session_id)`.
- Errors: `conversation_not_found` 404, `validation_error` 400, reuse `writeAgentError` sentinels.

- [ ] **Step 1: Failing tests** — SSE path emits `data:` lines ending with `final_response`; JSON path returns the §20 envelope; PUT prefs rejects bad `default_mode`; DELETE removes thread and is owner-scoped.
- [ ] **Step 2: Run → fail.**
- [ ] **Step 3: Implement** — content negotiation: `strings.Contains(r.Header.Get("Accept"), "text/event-stream")`.
- [ ] **Step 4: Run → PASS.** Commit: `feat(ai): unified chat HTTP handlers — SSE/JSON negotiation, ai-preferences, conversation CRUD`

---

### Task 9: Route wiring + `feature.japa_ai_unified` flag

**Files:**
- Modify: `internal/features/registry.go` (add flag, Category "AI", Default false)
- Modify: `internal/server/server.go` (~5424–5428 chat route; ~6661–6664 conversations; new prefs routes)

**Interfaces:**
- Consumes: `agentHTTPHandlers.HandleUnifiedChat`, `HandleGetAIPrefs`, `HandlePutAIPrefs`, `HandleListConversationsV2`, `HandleGetConversationV2`, `HandleDeleteConversation`.

- [ ] **Step 1: Registry entry**

```go
{Key: "feature.japa_ai_unified", Label: "Unified Japa AI", Description: "Merged JapaBot + AI agent single chat surface (§103 amendment)", Default: false, Category: "AI"},
```

Note: `TestRegistryCoversAllGatedKeys` will require the flag once it appears in a `features.Gate` call — add both together.

- [ ] **Step 2: Route changes in server.go**

```go
// Unified chat — feature.japa_ai_unified selects the merged engine;
// legacy HandleChat stays as the fallback path while the flag is off.
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use"), mw.RateLimit(vkClient.Redis(), "moderate")).
    Post("/ai/chat", func(w http.ResponseWriter, req *http.Request) {
        if featureChecker.IsEnabled(req.Context(), "feature.japa_ai_unified") {
            agentHTTPHandlers.HandleUnifiedChat(w, req)
            return
        }
        aiHandlers.HandleChat(w, req)
    })
```

Conversations routes swap `aiHandlers.HandleListConversations/HandleGetConversation` → the V2 handlers behind the same gates, plus new DELETE. New prefs routes:

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use")).
    Get("/me/ai-preferences", agentHTTPHandlers.HandleGetAIPrefs)
r.With(auth.RequireAuth(kr, vkClient.Redis()), features.Gate(featureChecker, "feature.japabot"), rbac.RequirePermission(rbacResolver, "ai.context.use")).
    Put("/me/ai-preferences", agentHTTPHandlers.HandlePutAIPrefs)
```

`/ai/agent` (`HandleAgent`) stays unchanged — it delegates to `HandleUnifiedChat` internally now.

- [ ] **Step 3: Wire `ConversationService` + context/persist injectors** in `NewServer` where `agentHTTPHandlers` is built (search `NewAgentHTTPHandlers` callsite): construct `NewConversationService(gen.New(db.Writer), honchoClient)`, pass `WithAgentConversations`, `WithAgentAIContext(aiHandlers.buildUserAIContext)`, `WithAgentChatPersist(aiHandlers.persistChatTurn)` into `NewAgentHandler` options.

- [ ] **Step 4: Build + targeted tests**

```bash
go build ./... && go test ./internal/ai/ ./internal/features/ ./internal/server/ -count=1
```

Expected: `TestRegistryCoversAllGatedKeys` and `TestNoDuplicateRoutes` pass.

- [ ] **Step 5: Commit** — `feat(ai): wire unified chat route, ai-preferences endpoints, feature.japa_ai_unified flag`

---

### Task 10: Frontend — unified chat UI

**Files:**
- Create: `apps/main/src/lib/ai/JapaAIChat.svelte`
- Create: `apps/main/src/lib/ai/AiToolsMenu.svelte`
- Modify: `apps/main/src/routes/(app)/ai/+page.svelte` (single component behind `data.unified` flag from load)
- Modify: `apps/main/src/routes/(app)/ai/+page.server.ts` (load `aiPrefs`, `capabilities.categories`; expose `unified` = `feature.japa_ai_unified`)
- Modify: `apps/main/src/routes/(app)/ai/sessions/+page.server.ts` + `+page.svelte` (unified conversation list, `legacy` badge)
- Create: `apps/main/src/routes/(app)/settings/ai/+page.svelte` + `+page.server.ts` (GET/PUT `/me/ai-preferences`; reuse `AiToolsMenu` in "defaults" mode)
- Modify: `apps/main/src/lib/i18n/en.json`, `fr.json`

**Interfaces:**
- Consumes: `POST /ai/chat` SSE (new `tools` field + `sources`/`conversation`/`message` events), `GET /ai/agent/capabilities` (`categories`), `GET/PUT /me/ai-preferences`.
- Request body sent by the composer:

```ts
{
  message, conversation_id, model, voice_id, humanize,
  tools: toolsOverrideEnabled ? { enabled, categories, tools } : undefined
}
```

- [ ] **Step 1: `agentStream.ts`** — extend `agentTypes.ts` with `sources`, `conversation`, `message` event types; parser already generic.
- [ ] **Step 2: `AiToolsMenu.svelte`** — popover: master switch + category chips + expandable per-tool checkboxes; props `{ categories, prefs, onchange }`; emit `ToolSelection` for composer, or full `AIPrefs` for settings (mode prop).
- [ ] **Step 3: `JapaAIChat.svelte`** — merge of `JapaBotChat.svelte` (364 lines: conversations sidebar, model/voice pickers, citations via `AiSourceCitations`) and `AgentChat.svelte` (513 lines: SSE via `agentStream.ts`, tool-call cards, cancel). Transcript renders: user msg → assistant msg (+ `sources` citation row) + tool-call cards inline. Composer gets `AiToolsMenu` when `capabilities.categories` non-empty.
- [ ] **Step 4: `/ai` page** — when `data.unified`, render `JapaAIChat` alone (no tab switcher); else today's behavior. `/settings/ai` page.
- [ ] **Step 5: i18n** — keys: `ai.tools.toggle`, `ai.tools.categories`, `ai.tools.settings_title`, `ai.mode.agent_default`, `ai.conversation.legacy` etc. in en + fr.
- [ ] **Step 6: Verify**

```bash
cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json && bun run test
```

- [ ] **Step 7: Commit** — `feat(main): unified Japa AI chat UI — tools popover, merged transcript, ai settings`

---

### Task 11: End-to-end verification + cleanup

- [ ] **Step 1: Manual matrix** (flag ON):
  - user with `ai.context.use` only → chat works, no tool UI, `tools` field ignored, `sources` render
  - `+ agent.use` → tools popover, tool calls inline, cancel works, session audit row linked to conversation
  - `feature.ai_agent` OFF → same as no-`agent.use` for everyone (chat keeps working)
  - `feature.japabot` OFF → surface 403/disabled as today
  - per-message `tools.enabled:false` mid-thread → pure-chat turn with full history context
  - legacy JSON client (no `Accept: text/event-stream`) → `{reply, conversation_id, sources}` unchanged
- [ ] **Step 2: `go test ./internal/ai/ ./internal/server/ ./internal/features/ -count=1` + `gosec` on changed packages**
- [ ] **Step 3: Update `.jeos/CHANGELOG.md`** and mark spec/plan status.
- [ ] **Step 4: Commit** — `feat(ai): unified Japa AI phase 1 complete`
