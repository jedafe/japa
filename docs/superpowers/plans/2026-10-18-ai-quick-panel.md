# AI Quick Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A page-aware slide-in AI chat panel on all four user-facing apps (main, docs, academy, tools), anchored to one persistent conversation per page/entity, tightly integrated with the ADR-130 unified `/ai` surface.

**Architecture:** New `ai_conversations.anchor` column (+ `UNIQUE(user_id, anchor)` partial index) gives every page a durable thread; `POST /ai/chat` accepts an `anchor` field and `GET /ai/conversations` an `?anchor=` filter. A new `@japaconnect/ai-panel` workspace package holds `AiPanel.svelte`, the `aiPageContext` rune store, and a deps-injected `createStreamChat` factory (moved from `apps/main/src/lib/ai/unifiedStream.ts`). Each app wires the package with its own csrf/api/i18n deps and a route→context extractor table; the existing Sparkles FAB becomes the panel toggle.

**Tech Stack:** Go 1.x + chi + sqlc/pgx + golang-migrate (backend); SvelteKit 2 + Svelte 5 runes + Tailwind v4 + lucide-svelte (frontend); bun workspaces (monorepo).

**Spec:** `docs/superpowers/specs/2026-10-18-ai-quick-panel-design.md`

## Global Constraints

- **Rule 2:** never hand-edit `internal/db/gen/` — edit `.sql` files in `internal/db/queries/`, then `make sqlc-gen`.
- **Rule 11:** soft deletes only — the anchor index must be partial on `deleted_at IS NULL`.
- **Rule 16:** all entity ids in page-context extractors come from API-loaded page data — no hardcoded maps.
- **Rule 17:** no AI attribution anywhere — no "Generated with", no Co-Authored-By agent trailers, no `// AI:` comments. Plain commit messages only.
- **Rule 18:** bun only (`bun`, `bunx`, `bun run`) — never npm/npx/pnpm/yarn.
- **JEOS before code:** `.jeos/sections/115-ai-quick-panel.md` lands in Task 1 before any application code.
- All commits go to `main`. Do not create branches.
- Anchor format: `^[a-z0-9_.:/-]{1,200}$`, lowercased server-side; anchor keys use stable entity ids (`wikipage:{id}`, `content:{id}`, `space:{id}`, `doc:{id}`, `tool:{slug}`, `track:{id}`, `lesson:{id}`) with `path:{pathname}` as the fallback (pathnames are already charset-safe; `route.id` values like `/(app)/blog/[slug]` contain `()[]` and would be rejected server-side — do NOT use them).
- `context` payloads use ONLY keys in `allowedContextKeys` (internal/ai/handlers.go:506): `title`, `slug`, `content_id`, `space_id`, `geo_space_id`, `tags`, `tone`, `content_type`, `doc_category`, `doc_subcategory`, `audience`, `language`, etc.
- i18n: every UI string goes through injected `t(key, fallback)` with `ai.panel.*` keys added to each app's `en.json` + `fr.json`.

---

### Task 1: JEOS bookkeeping

**Files:**
- Create: `.jeos/sections/115-ai-quick-panel.md`
- Modify: `.jeos/conventions/api-routes-catalog.md` (add `?anchor=` param note on `GET /ai/conversations`, `anchor` field on `POST /ai/chat`)
- Modify: `.jeos/adr/ADR-130-unified-japa-ai.md` (amendment note)

- [ ] **Step 1: Write the section file**

Create `.jeos/sections/115-ai-quick-panel.md` covering: purpose (slide-in page-aware panel on all user-facing surfaces), the `ai_conversations.anchor` contract (format `^[a-z0-9_.:/-]{1,200}$`, UNIQUE partial index, soft-delete frees the anchor), `POST /ai/chat` `anchor` field + `GET /ai/conversations?anchor=` filter, the `@japaconnect/ai-panel` package boundary (deps injection — apps supply csrf/apiBase/surface/t/fullChatUrl), per-app route→context extractor tables, eligibility (`feature.japabot` + `ai.context.use` OR `agent.use`), and the `/ai?c={conversation_id}` deep link. Cite `docs/superpowers/specs/2026-10-18-ai-quick-panel-design.md` as the design source.

- [ ] **Step 2: Update the routes catalog + ADR-130**

In `.jeos/conventions/api-routes-catalog.md`, annotate `POST /ai/chat` request body with `anchor?: string` and `GET /ai/conversations` with `?anchor={key}` (returns the anchored thread only). Append a short "Amendment — AI Quick Panel (§115)" section to `ADR-130-unified-japa-ai.md`: the unified surface gains a cross-app panel variant; threads may be anchored to a page/entity; `/ai?c=` deep-links a thread.

- [ ] **Step 3: Commit**

```bash
git add .jeos/sections/115-ai-quick-panel.md .jeos/conventions/api-routes-catalog.md .jeos/adr/ADR-130-unified-japa-ai.md
git commit -m "docs(jeos): §115 AI quick panel — anchor contract, routes, ADR-130 amendment"
```

---

### Task 2: Migration + sqlc anchor queries

**Files:**
- Create: `migrations/000888_ai_conversation_anchor.up.sql`
- Create: `migrations/000888_ai_conversation_anchor.down.sql`
- Modify: `internal/db/queries/ai_conversations.sql`
- Regenerate: `internal/db/gen/` (via `make sqlc-gen`)

**Interfaces:**
- Produces: `gen.GetAIConversationByAnchor(ctx, gen.GetAIConversationByAnchorParams{UserID, Anchor}) → (gen.AiConversation, error)`; `gen.CreateAIConversationParams` gains `Anchor *string`; `gen.AiConversation` gains `Anchor *string`.

- [ ] **Step 1: Write the migration**

```sql
-- migrations/000888_ai_conversation_anchor.up.sql
-- §115 AI Quick Panel: per-page thread anchoring. One live conversation
-- per (user_id, anchor); soft-deleting a thread frees the anchor so the
-- panel starts fresh on next send.
ALTER TABLE ai_conversations ADD COLUMN anchor text;

CREATE UNIQUE INDEX idx_ai_conversations_anchor
    ON ai_conversations (user_id, anchor)
    WHERE anchor IS NOT NULL AND deleted_at IS NULL;
```

```sql
-- migrations/000888_ai_conversation_anchor.down.sql
DROP INDEX IF EXISTS idx_ai_conversations_anchor;
ALTER TABLE ai_conversations DROP COLUMN IF EXISTS anchor;
```

- [ ] **Step 2: Add the sqlc queries**

In `internal/db/queries/ai_conversations.sql`, replace `CreateAIConversation` and add the anchor lookup:

```sql
-- name: CreateAIConversation :one
INSERT INTO ai_conversations (user_id, title, project_id, anchor) VALUES (@user_id, @title, @project_id, sqlc.narg(anchor))
RETURNING *;
```

```sql
-- §115 AI Quick Panel: anchored (page-scoped) thread lookup for the
-- slide-in panel — one live conversation per (user_id, anchor).
-- name: GetAIConversationByAnchor :one
SELECT * FROM ai_conversations
WHERE user_id = @user_id AND anchor = @anchor AND deleted_at IS NULL;
```

- [ ] **Step 3: Apply migration + regenerate**

```bash
make migrate    # runs `go run ./cmd/migrate/`
make sqlc-gen
```

Expected: `internal/db/gen/ai_conversations.sql.go` regenerated; `AiConversation.Anchor *string`, `CreateAIConversationParams.Anchor *string`, `GetAIConversationByAnchor` present.

- [ ] **Step 4: Fix compile errors at CreateAIConversation call sites**

`CreateAIConversationParams` now has a required `Anchor *string` field — every caller must set it (nil where no anchor). Find them:

```bash
grep -rn "CreateAIConversation" internal/ --include="*.go" | grep -v internal/db/gen
```

Known call sites: `ConversationService.Resolve` (internal/ai/conversations.go — updated in Task 3), plus any test stubs implementing `ConversationStore` (internal/ai/conversations_test.go, agent_unified_test.go — the stub must add `GetAIConversationByAnchor` and the new param). Compile: `go build ./internal/ai/...` — fix until clean (temporary `Anchor: nil` where the caller isn't Task-3-updated yet).

- [ ] **Step 5: Commit**

```bash
git add migrations/000888_ai_conversation_anchor.*.sql internal/db/queries/ai_conversations.sql internal/db/gen/
git commit -m "feat(ai): ai_conversations.anchor column + by-anchor lookup (§115)"
```

---

### Task 3: Backend anchor plumbing (Resolve, request field, list filter)

**Files:**
- Modify: `internal/ai/conversations.go` (interface + Resolve + new GetByAnchor method)
- Modify: `internal/ai/agent_unified.go` (UnifiedChatRequest.Anchor + pass to Resolve)
- Modify: `internal/ai/unified_handlers.go` (input field, sanitizeAnchor, `?anchor=` filter)
- Test: `internal/ai/conversations_test.go` (anchor resolve tests)
- Test: `internal/ai/unified_handlers_test.go` (sanitizeAnchor + filter tests)

**Interfaces:**
- Consumes: `gen.GetAIConversationByAnchor`, `CreateAIConversationParams.Anchor` (Task 2)
- Produces:
  - `ConversationService.Resolve(ctx, userID, conversationID, projectID *pgtype.UUID, title, anchor string) (gen.AiConversation, error)`
  - `ConversationService.GetByAnchor(ctx, userID pgtype.UUID, anchor string) (gen.AiConversation, error)`
  - `sanitizeAnchor(raw string) string` (package `ai`)
  - `UnifiedChatRequest.Anchor string`; `unifiedChatInput.anchor` JSON field
  - `GET /ai/conversations?anchor={key}` → `{data:[convSummary]}` (0 or 1 items)

- [ ] **Step 1: Write the failing Resolve anchor tests**

In `internal/ai/conversations_test.go` — follow the existing stub pattern (the file has a stub `ConversationStore`; extend it with `GetAIConversationByAnchor` recording calls):

```go
func TestResolveByAnchor(t *testing.T) {
	t.Run("existing anchored thread is returned", func(t *testing.T) {
		// stub: GetAIConversationByAnchor returns a conversation → Resolve returns it,
		// CreateAIConversation is NOT called.
	})
	t.Run("miss creates with anchor stamped", func(t *testing.T) {
		// stub: GetAIConversationByAnchor → pgx.ErrNoRows → CreateAIConversation called
		// with Anchor == &"wikipage:abc".
	})
	t.Run("23505 on create retries the anchor lookup", func(t *testing.T) {
		// stub: first CreateAIConversation → &pgconn.PgError{Code:"23505"};
		// second GetAIConversationByAnchor → conversation → returned.
	})
	t.Run("conversation_id wins over anchor", func(t *testing.T) {
		// valid conversationID resolves via GetAIConversationForUser;
		// GetAIConversationByAnchor never called.
	})
	t.Run("empty anchor keeps existing behavior", func(t *testing.T) {
		// anchor "" → no by-anchor lookup; plain create.
	})
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/ai/ -run TestResolveByAnchor -v`
Expected: FAIL — `GetAIConversationByAnchor` undefined on the stub / Resolve signature mismatch.

- [ ] **Step 3: Extend the store interface and Resolve**

In `internal/ai/conversations.go`, add to `ConversationStore`:

```go
GetAIConversationByAnchor(ctx context.Context, arg gen.GetAIConversationByAnchorParams) (gen.AiConversation, error)
```

Replace `Resolve` (line ~86):

```go
// Resolve returns the conversation for a send: an explicit id wins, then
// the §115 page anchor, then a fresh create (stamped with the anchor so
// later sends on the same page resume the thread). A 23505 race on the
// anchor unique index re-reads by anchor once.
func (s *ConversationService) Resolve(ctx context.Context, userID pgtype.UUID, conversationID, projectID *pgtype.UUID, title, anchor string) (gen.AiConversation, error) {
	if conversationID != nil && conversationID.Valid {
		conv, err := s.store.GetAIConversationForUser(ctx, gen.GetAIConversationForUserParams{
			ID:     *conversationID,
			UserID: userID,
		})
		if err == nil {
			return conv, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return gen.AiConversation{}, fmt.Errorf("resolving conversation: %w", err)
		}
	}

	if anchor != "" {
		conv, err := s.store.GetAIConversationByAnchor(ctx, gen.GetAIConversationByAnchorParams{
			UserID: userID,
			Anchor: anchor,
		})
		if err == nil {
			return conv, nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return gen.AiConversation{}, fmt.Errorf("resolving conversation by anchor: %w", err)
		}
	}

	var pid pgtype.UUID
	if projectID != nil && projectID.Valid {
		pid = *projectID
	}
	var anchorArg *string
	if anchor != "" {
		anchorArg = &anchor
	}

	conv, err := s.store.CreateAIConversation(ctx, gen.CreateAIConversationParams{
		UserID:    userID,
		Title:     title,
		ProjectID: pid,
		Anchor:    anchorArg,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && anchor != "" {
			if existing, lerr := s.store.GetAIConversationByAnchor(ctx, gen.GetAIConversationByAnchorParams{
				UserID: userID,
				Anchor: anchor,
			}); lerr == nil {
				return existing, nil
			}
		}
		return gen.AiConversation{}, fmt.Errorf("creating conversation: %w", err)
	}
	return conv, nil
}

// GetByAnchor returns the live anchored thread for the panel's
// pre-send resume (GET /ai/conversations?anchor=).
func (s *ConversationService) GetByAnchor(ctx context.Context, userID pgtype.UUID, anchor string) (gen.AiConversation, error) {
	conv, err := s.store.GetAIConversationByAnchor(ctx, gen.GetAIConversationByAnchorParams{
		UserID: userID,
		Anchor: anchor,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return gen.AiConversation{}, ErrConversationNotFound
		}
		return gen.AiConversation{}, err
	}
	return conv, nil
}
```

(Ensure `pgconn` is imported; conversations.go already imports `pgx`, `errors`, `fmt`.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/ai/ -run TestResolveByAnchor -v`
Expected: PASS (update every `Resolve(` caller/stub for the new signature — `agent_unified.go:211` passes `req.Anchor`, added below).

- [ ] **Step 5: Write the failing sanitizeAnchor + filter tests**

In `internal/ai/unified_handlers_test.go` (same package `ai`):

```go
func TestSanitizeAnchor(t *testing.T) {
	cases := map[string]string{
		"wikipage:9f0c-2a":            "wikipage:9f0c-2a",
		"  Content:AbC_1  ":           "content:abc_1",
		"route:/(app)/blog/[slug]":    "route:/(app)/blog/[slug]",
		"bad anchor!!":                "",
		"nested{json}":                "",
		strings.Repeat("a", 201):      "",
		"":                            "",
	}
	for in, want := range cases {
		if got := sanitizeAnchor(in); got != want {
			t.Fatalf("sanitizeAnchor(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 6: Implement sanitizeAnchor + plumb the request field**

In `internal/ai/unified_handlers.go` (needs `regexp` import):

```go
// §115 AI Quick Panel — anchor keys are lowercase [a-z0-9_.:/-], ≤200
// chars. Invalid anchors are dropped (warn-logged), never a request error:
// a bad anchor must not block chat.
var anchorRe = regexp.MustCompile(`^[a-z0-9_.:/-]{1,200}$`)

func sanitizeAnchor(raw string) string {
	a := strings.ToLower(strings.TrimSpace(raw))
	if a == "" {
		return ""
	}
	if !anchorRe.MatchString(a) {
		slog.Warn("ai: dropping invalid anchor")
		return ""
	}
	return a
}
```

Add to `unifiedChatInput`: `Anchor string \`json:"anchor,omitempty"\``.

In `HandleUnifiedChat` after `input.Context = sanitizeClientContext(input.Context)`:

```go
anchor := sanitizeAnchor(input.Anchor)
```

and set `Anchor: anchor` on the `req := UnifiedChatRequest{...}` literal.

In `internal/ai/agent_unified.go`: add `Anchor string` field to `UnifiedChatRequest` (line ~44) and change the Resolve call (line ~211):

```go
c, err := h.convos.Resolve(ctx, userID, req.ConversationID, req.ProjectID, truncateRunes(req.Prompt, titleLen), req.Anchor)
```

- [ ] **Step 7: Add the `?anchor=` filter to HandleListConversationsV2**

In `internal/ai/unified_handlers.go`, inside `HandleListConversationsV2` insert the branch AFTER the `type convoSummary struct` block + `out`/`dbIDs` declarations and BEFORE the `if h.Convos != nil` drain loop (~line 256 — `convoSummary` must be declared first):

```go
// §115: ?anchor= returns only the page-anchored thread (0 or 1 items) —
// the quick panel's pre-send resume. Anchored threads are always unified
// rows, so the legacy union below is skipped on this path.
if anchor := sanitizeAnchor(r.URL.Query().Get("anchor")); anchor != "" {
	filtered := []convoSummary{}
	if h.Convos != nil {
		if conv, aerr := h.Convos.GetByAnchor(r.Context(), user.UserID, anchor); aerr == nil {
			sum := convoSummary{
				ConversationID: conv.ID.String(),
				Title:          conv.Title,
				LastEntryAt:    conv.UpdatedAt.Time,
			}
			if conv.ProjectID.Valid {
				sum.ProjectID = conv.ProjectID.String()
			}
			filtered = append(filtered, sum)
		}
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": filtered,
		"meta": map[string]any{"total": int64(len(filtered)), "page_size": pageSize, "page_offset": 0},
	})
	return
}
```

Note: `convoSummary`/`pageSize`/`pageOffset` are already declared above this point; `convoSummary` lacks `TurnCount`/`Preview` population here — anchored resume only needs `conversation_id`; the detail endpoint supplies the rest.

- [ ] **Step 8: Build + full ai package tests**

```bash
go build ./internal/ai/... && go test ./internal/ai/ -count=1
```

Expected: build clean; all tests pass including existing unified handler tests.

- [ ] **Step 9: Commit**

```bash
git add internal/ai/conversations.go internal/ai/agent_unified.go internal/ai/unified_handlers.go internal/ai/conversations_test.go internal/ai/unified_handlers_test.go
git commit -m "feat(ai): conversation anchors — Resolve upsert, request field, ?anchor= filter (§115)"
```

---

### Task 4: `@japaconnect/ai-panel` package — scaffold, types, stream factory

**Files:**
- Create: `packages/ai-panel/package.json`
- Create: `packages/ai-panel/src/index.ts`
- Create: `packages/ai-panel/src/types.ts`
- Create: `packages/ai-panel/src/deps.ts`
- Create: `packages/ai-panel/src/stream.ts`
- Create: `packages/ai-panel/src/context.svelte.ts`
- Modify: `package.json` (root — confirm `packages/*` workspace covers it; already does)

**Interfaces:**
- Produces:
  - `initAiPanelDeps(deps: AiPanelDeps): void` / `getAiPanelDeps(): AiPanelDeps`
  - `apiGet<T>(path): Promise<T | null>` / `apiPost<T>(path, body?): Promise<T | null>` (envelope-unwrap, csrf+surface headers)
  - `createStreamChat(): (opts: StreamChatOptions) => AsyncGenerator<AgentEvent>`
  - `setAiPageContext(ctx | null)`, `clearAiPageContext()`, `aiPageContext` (read: `aiPageContext.current`)
  - Types: `AgentEvent`, `AgentEventType`, `AgentRequestError`, `AgentCapabilities`, `AgentToolCategory`, `AgentGeoContext`, `ConversationMessage`, `ConversationToolCall`, `UnifiedConversationSummary`, `AiCitationSource`, `AiPageContext`, `AiPanelDeps`, `StreamChatOptions`, `ToolSelectionOverride`

- [ ] **Step 1: Scaffold the package**

`packages/ai-panel/package.json` — mirror `packages/ui/package.json` shape:

```json
{
	"name": "@japaconnect/ai-panel",
	"version": "0.0.1",
	"private": true,
	"type": "module",
	"svelte": "./src/index.ts",
	"types": "./src/index.ts",
	"exports": {
		".": { "types": "./src/index.ts", "svelte": "./src/index.ts" }
	},
	"peerDependencies": {
		"svelte": "^5.0.0",
		"@sveltejs/kit": "^2.0.0",
		"lucide-svelte": "^1.0.0",
		"tailwindcss": "^4.0.0",
		"bits-ui": "^2.0.0"
	},
	"dependencies": {
		"@japaconnect/api-url": "workspace:*"
	}
}
```

- [ ] **Step 2: `src/types.ts` — shared types**

Copy from `apps/main/src/lib/ai/agentTypes.ts` ONLY these (package must not import `chatTemplate` or app `$lib`): `AgentRequestError`, `AgentEventType`, `AgentEvent`, `AgentToolCapability`, `AgentGeoContext`, `AgentToolCategory`, `AgentCapabilities`, `UnifiedConversationSummary`, `ConversationToolCall`, `ConversationMessage`. Plus `AiCitationSource` (copy the 11-line interface verbatim from `apps/main/src/lib/ai/types.ts:57`). Plus:

```ts
export interface ToolSelectionOverride {
	enabled?: boolean;
	categories?: string[];
	tools?: string[];
}

export interface StreamChatOptions {
	message: string;
	conversationId?: string;
	projectId?: string;
	/** §115 page anchor — find-or-create the page's thread server-side. */
	anchor?: string;
	model?: string;
	voiceId?: string;
	context?: Record<string, unknown>;
	humanize?: boolean;
	humanizeLevel?: string;
	tools?: ToolSelectionOverride;
	signal?: AbortSignal;
}

/** What the host app injects — the package can't reach app $lib modules. */
export interface AiPanelDeps {
	apiBase: string;
	surface: string;
	getCsrfToken: () => string | null;
	refreshCsrfToken: () => Promise<unknown>;
	t?: (key: string, fallback?: string, params?: Record<string, string | number>) => string;
	fullChatUrl: (conversationId?: string) => string;
}

export interface AiPageContext {
	anchor: string;
	label: string;
	context: Record<string, unknown>;
}
```

- [ ] **Step 3: `src/deps.ts` — injected deps + envelope api client**

```ts
import { buildApiUrl } from '@japaconnect/api-url';
import type { AiPanelDeps } from './types';

let deps: AiPanelDeps | null = null;

export function initAiPanelDeps(d: AiPanelDeps): void {
	deps = d;
}

export function getAiPanelDeps(): AiPanelDeps {
	if (!deps) throw new Error('ai-panel: initAiPanelDeps() not called');
	return deps;
}

export function pt(key: string, fallback: string, params?: Record<string, string | number>): string {
	return deps?.t?.(key, fallback, params) ?? fallback;
}

function headers(extra?: Record<string, string>): Record<string, string> {
	const d = getAiPanelDeps();
	const h: Record<string, string> = { accept: 'application/json', 'x-jc-surface': d.surface, ...extra };
	const csrf = d.getCsrfToken();
	if (csrf) h['x-csrf-token'] = csrf;
	return h;
}

async function unwrap<T>(res: Response): Promise<T | null> {
	if (!res.ok) return null;
	const json = (await res.json().catch(() => null)) as { data?: T } | null;
	return (json?.data ?? null) as T | null;
}

export async function apiGet<T>(path: string): Promise<T | null> {
	const res = await fetch(buildApiUrl(path, getAiPanelDeps().apiBase), {
		credentials: 'include',
		headers: headers()
	});
	return unwrap<T>(res);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T | null> {
	const res = await fetch(buildApiUrl(path, getAiPanelDeps().apiBase), {
		method: 'POST',
		credentials: 'include',
		headers: headers({ 'content-type': 'application/json' }),
		body: JSON.stringify(body ?? {})
	});
	return unwrap<T>(res);
}
```

- [ ] **Step 4: `src/stream.ts` — the SSE factory**

Port `apps/main/src/lib/ai/unifiedStream.ts` verbatim EXCEPT:
- signature: `export function createStreamChat() { return async function* streamChat(opts: StreamChatOptions) {...} }` — reads `getAiPanelDeps()` for apiBase/csrf/surface instead of `$env/static/public` + `$lib/api/csrf.svelte`.
- body gains `anchor: opts.anchor || undefined`.
- imports: `buildApiUrl` from `@japaconnect/api-url`; `AgentRequestError`, `AgentEvent`, `StreamChatOptions` from `./types`; csrf/base/surface from `./deps`.
- keep the 403 `csrf_token_invalid` → `refreshCsrfToken()` → retry-once logic unchanged.

- [ ] **Step 5: `src/context.svelte.ts` — the page-context store**

```ts
import type { AiPageContext } from './types';

let current = $state<AiPageContext | null>(null);

export const aiPageContext = {
	get current() {
		return current;
	}
};

export function setAiPageContext(ctx: AiPageContext | null): void {
	current = ctx;
}
```

- [ ] **Step 6: `src/index.ts`**

```ts
export { default as AiPanel } from './AiPanel.svelte';
export { initAiPanelDeps, getAiPanelDeps } from './deps';
export { createStreamChat } from './stream';
export { aiPageContext, setAiPageContext } from './context.svelte';
export * from './types';
```

- [ ] **Step 7: Install + verify resolution**

```bash
bun install
cd apps/main && bun add @japaconnect/ai-panel@workspace:*   # also docs/academy/tools in their tasks
bunx tsc --noEmit -p packages/ai-panel 2>/dev/null || true   # sanity; full typecheck happens per-app
```

- [ ] **Step 8: Commit**

```bash
git add packages/ai-panel bun.lock apps/*/package.json
git commit -m "feat(ai-panel): shared package scaffold — deps, SSE factory, page-context store (§115)"
```

---

### Task 5: `AiPanel.svelte` — the panel component

**Files:**
- Create: `packages/ai-panel/src/AiPanel.svelte`
- Test: `packages/ai-panel/src/AiPanel.test.ts` (or vitest in apps/main — see Step 6)

**Interfaces:**
- Consumes: everything from Task 4 + `apiGet`/`apiPost`/`pt` from `./deps`.
- Produces: `<AiPanel open={boolean} user={SessionUserLike | null} onclose={() => void} />` where `SessionUserLike = { permissions?: string[] } | null`.

Component contract:

```svelte
<script lang="ts">
	interface Props {
		open: boolean;
		/** null → guest → sign-in CTA. */
		user: { permissions?: string[] } | null;
		onclose: () => void;
	}
</script>
```

- [ ] **Step 1: Write the failing component test**

Vitest + @testing-library/svelte (check which app has vitest configured — `apps/main` runs `chatTemplate.test.ts`; run package tests from main if simpler):

```ts
// packages/ai-panel/src/context.test.ts
import { describe, it, expect } from 'vitest';
import { aiPageContext, setAiPageContext } from './context.svelte';

describe('aiPageContext', () => {
	it('sets and clears page context', () => {
		expect(aiPageContext.current).toBeNull();
		setAiPageContext({ anchor: 'wikipage:1', label: 'Wiki · Housing', context: { content_id: '1' } });
		expect(aiPageContext.current?.anchor).toBe('wikipage:1');
		setAiPageContext(null);
		expect(aiPageContext.current).toBeNull();
	});
});
```

Run to verify it fails (module not found → then passes after Task 4 files exist; order steps accordingly).

- [ ] **Step 2: Build `AiPanel.svelte`**

Structure (follow `JapaAIChat.svelte` patterns — the event loop at ~line 1150 and `loadConversation` at ~974 are the references; this is a trimmed standalone):

Turn model (declare above the state block):

```ts
interface PanelToolCall { name: string; status: string }
interface PanelTurn {
	id: number;
	role: 'user' | 'assistant';
	content: string;
	in_progress?: boolean;
	progress?: string;
	tool_calls?: PanelToolCall[];
	sources?: AiCitationSource[];
	error?: string;
	session_id?: string | null;
}
```

State:

```ts
let turns = $state<PanelTurn[]>([]);
let conversationId = $state('');
let sending = $state(false);
let composer = $state('');
let loadedAnchor = $state('');                // anchor whose thread is loaded
let agentCapable = $state<boolean | null>(null); // null = not yet probed
let toolsOn = $state(true);                   // panel default: agent ops on
let loadingThread = $state(false);
let turnSeq = 0;
let activeAbort: AbortController | null = null;
let activeSessionId = $state<string | null>(null);
```

Derived context/anchor:

```ts
const ctxNow = $derived(aiPageContext.current);
const anchor = $derived(ctxNow?.anchor ?? 'panel:general');
```

Effects:

```ts
// On open / anchor change while open → load the anchored thread.
$effect(() => {
	if (open && anchor !== loadedAnchor && user) {
		void loadAnchoredThread(anchor);
	}
});
// First open → probe capabilities (403/flag → chat-only, toggle hidden).
$effect(() => {
	if (open && agentCapable === null && user) {
		void probeCapabilities();
	}
});
```

Key functions (adapted from JapaAIChat — same semantics, minus sidebar/projects/templates):

```ts
async function probeCapabilities() {
	const caps = await apiGet<{ categories?: unknown[] }>('/ai/agent/capabilities');
	agentCapable = Array.isArray(caps?.categories) && caps.categories.length > 0;
}

async function loadAnchoredThread(a: string) {
	loadingThread = true;
	loadedAnchor = a;
	const list = await apiGet<{ conversation_id: string }[]>(`/ai/conversations?anchor=${encodeURIComponent(a)}`);
	const conv = list?.[0];
	if (!conv) { conversationId = ''; turns = []; loadingThread = false; return; }
	const detail = await apiGet<{
		conversation_id: string;
		running_session_id?: string;
		messages?: ConversationMessage[];
	}>(`/ai/conversations/${conv.conversation_id}`);
	conversationId = detail?.conversation_id ?? conv.conversation_id;
	turns = (detail?.messages ?? []).map((m) => ({
		id: ++turnSeq,
		role: m.role,
		content: m.content,
		session_id: m.agent_session_id,
		tool_calls: m.tool_calls?.map((tc) => ({ name: tc.tool_name, status: tc.status }))
	}));
	loadingThread = false;
	if (detail?.running_session_id && !sending) startSessionPoll(detail.running_session_id);
}

async function send() {
	const message = composer.trim();
	if (!message || sending) return;
	composer = '';
	sending = true;
	turns = [...turns,
		{ id: ++turnSeq, role: 'user', content: message },
		{ id: ++turnSeq, role: 'assistant', content: '', in_progress: true, tool_calls: [] }];
	const live = turns[turns.length - 1];
	const abort = new AbortController();
	activeAbort = abort;
	activeSessionId = null;
	try {
		for await (const ev of createStreamChat()({
			message,
			conversationId: conversationId || undefined,
			anchor: conversationId ? undefined : anchor,
			context: ctxNow?.context,
			tools: agentCapable && toolsOn ? { enabled: true } : { enabled: false },
			signal: abort.signal
		})) {
			handleEvent(ev, live);
		}
		// stream ended without final_response → detach-poll fallback
		if (live.in_progress) {
			const sid = live.session_id ?? (conversationId ? await findRunningSessionId(conversationId) : '');
			if (sid) { live.progress = pt('ai.panel.bg_running', 'Working in the background…'); live.session_id = sid; startSessionPoll(sid); }
			else { live.in_progress = false; live.content ||= pt('ai.panel.stream_closed', 'Stream closed.'); }
		}
	} catch (e) {
		live.in_progress = false;
		live.error = e instanceof AgentRequestError ? e.message : pt('ai.panel.error', 'Something went wrong.');
	} finally {
		sending = false;
		activeAbort = null;
	}
}

function handleEvent(ev: AgentEvent, turn: PanelTurn) {
	switch (ev.type) {
		case 'session_start': turn.session_id = ev.session_id ?? null; break;
		case 'progress': turn.progress = ev.message ?? ''; break;
		case 'tool_call_start':
			turn.tool_calls = [...(turn.tool_calls ?? []), { name: ev.tool_name ?? '', status: 'running' }];
			break;
		case 'tool_call_result': {
			const tc = turn.tool_calls?.findLast((t) => t.name === ev.tool_name && t.status === 'running');
			if (tc) tc.status = ev.status ?? 'done';
			break;
		}
		case 'delta': turn.content += ev.content ?? ''; break;
		case 'sources': turn.sources = ev.sources ?? []; break;
		case 'conversation': if (ev.conversation_id) conversationId = ev.conversation_id; break;
		case 'final_response':
			turn.in_progress = false;
			if (ev.content) turn.content = ev.content;
			if (ev.conversation_id) conversationId = ev.conversation_id;
			break;
		case 'error':
			turn.in_progress = false;
			turn.error = ev.error ?? pt('ai.panel.error', 'Something went wrong.');
			break;
	}
	turns = [...turns];
}
```

Trimmed `findRunningSessionId`/`startSessionPoll`/`stopSessionPoll`/`cancelBackgroundSession` — port from JapaAIChat.svelte:485-575, replacing `api.get/post` with `apiGet`/`apiPost` and dropping the `convos` sidebar insert + `loadConversation` → `loadAnchoredThread(loadedAnchor)`.

Markup — fixed right drawer (400px, `sm:bottom-0`, full-screen `<640px`), slides via `translate-x-full → translate-x-0` transition; `role="dialog"` + `aria-label`; `Esc` key handler calls `onclose`; header row: Sparkles icon + `t('ai.panel.title','Japa AI')` + context chip (`ctxNow.label` when set) + expand CTA (`<a href={getAiPanelDeps().fullChatUrl(conversationId || undefined)}>` with `ExternalLink` icon, `aria-label` `ai.panel.open_full`, `target` same-tab on main / `_blank` on satellite apps — pass a `fullChatNewTab?: boolean` dep or just always same-tab since it's an absolute URL) + close `X` button calling `onclose`.

Guest body (`!user`): centered sign-in card — `t('ai.panel.sign_in', 'Sign in to chat')` linking to `fullChatUrl()` (main app `/ai` redirects to login).

Body states: `loadingThread` → skeleton rows; empty turns → welcome line `t('ai.panel.welcome', …)`; transcript scroll anchored to bottom (`tick()` + `el.scrollTop = scrollHeight` after turn pushes/deltas).

Composer: `<textarea>` auto-grow (max ~5 rows), `Enter` sends / `Shift+Enter` newline; tools toggle (only when `agentCapable === true`) — a `Wrench` icon button with `aria-pressed` bound to `toolsOn`; send `ArrowUp` button / `Square` stop button while `sending` (calls `activeAbort?.abort()`).

Sources: small numbered chips row under each assistant turn (reuse the AiSourceCitations pattern — title link + tier).

Styling: all CSS vars (`var(--color-surface)`, `--color-border`, `--color-text`, `--color-muted`, `--color-primary`, `--color-primary-contrast`, `--radius`) — no hardcoded colors; these vars exist in every app's theme.

- [ ] **Step 3: Typecheck the package from main app**

```bash
cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "ai-panel" | head
```

(Errors surface through main's svelte-check once imported in Task 6; a standalone package check needs a tsconfig — if the package lacks one, add a minimal `tsconfig.json` extending the repo pattern used by `packages/ui`.)

- [ ] **Step 4: Commit**

```bash
git add packages/ai-panel/src/AiPanel.svelte packages/ai-panel/src/*.test.ts
git commit -m "feat(ai-panel): AiPanel component — anchored thread resume, streaming, tool pills (§115)"
```

---

### Task 6: Main app — rewire to the package

**Files:**
- Modify: `apps/main/src/lib/ai/unifiedStream.ts` → re-export shim
- Modify: `apps/main/src/lib/ai/agentTypes.ts` → partial re-export shim
- Modify: `apps/main/package.json` (`@japaconnect/ai-panel` dep — done in Task 4 Step 7)

- [ ] **Step 1: Convert unifiedStream.ts to a shim**

```ts
// §115: implementation moved to @japaconnect/ai-panel; this shim keeps
// existing imports working. main's deps are initialized in the (app)
// layout before any chat surface mounts.
export { type ToolSelectionOverride, type StreamChatOptions } from '@japaconnect/ai-panel';
import { createStreamChat } from '@japaconnect/ai-panel';
export const streamChat = createStreamChat();
```

- [ ] **Step 2: Convert agentTypes.ts to partial re-export**

Keep every type the package does NOT own (`AIPrefs`, `AiProject`, `AgentSession`, `AgentToolCall`, `AgentSessionDetail`, `AgentListMeta`, `AgentToolStat`, `AgentCostReportRow`, `AIPrivacyStatus`, `AIPrivacyUpdateResult`) and re-export shared ones:

```ts
export { AgentRequestError } from '@japaconnect/ai-panel';
export type {
	AgentEventType, AgentEvent, AgentToolCapability, AgentGeoContext,
	AgentToolCategory, AgentCapabilities, UnifiedConversationSummary,
	ConversationToolCall, ConversationMessage
} from '@japaconnect/ai-panel';
// …unchanged remaining declarations…
```

Important: `AgentRequestError` must be re-exported by VALUE (it's a class used with `instanceof`) — type-only re-export would break `e instanceof AgentRequestError` at JapaAIChat.svelte:1195.

- [ ] **Step 3: Check every importer still resolves**

```bash
grep -rln "from '\$lib/ai/unifiedStream'\|from './unifiedStream'\|from '\$lib/ai/agentTypes'\|from './agentTypes'" apps/main/src | grep -v node_modules
```

Expected: all resolve through the shims — no import-site edits needed.

- [ ] **Step 4: Typecheck + unit tests**

```bash
cd apps/main && bun run check   # or bunx svelte-check
bun run test                    # vitest — chatTemplate.test.ts must still pass
```

Expected: clean. `/ai` behavior unchanged (deps are not yet initialized — `streamChat` factory call is lazy; `getAiPanelDeps()` only runs on send. Verify `createStreamChat()` defers `getAiPanelDeps()` into the request path — it does, since deps are read inside `streamChat`'s body per call. If it throws at import time, move the deps read inside the generator function.)

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/ai/unifiedStream.ts apps/main/src/lib/ai/agentTypes.ts
git commit -m "refactor(main): AI stream/types re-export from @japaconnect/ai-panel (§115)"
```

---

### Task 7: Main app — FAB toggle, panel mount, deps init

**Files:**
- Modify: `apps/main/src/routes/(app)/+layout.svelte`
- Create: `apps/main/src/lib/ai/panel.ts` (deps wiring helper)

- [ ] **Step 1: Create the deps wiring module**

```ts
// apps/main/src/lib/ai/panel.ts
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { initAiPanelDeps } from '@japaconnect/ai-panel';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { t } from '$lib/i18n';

let inited = false;

/** Wire the shared ai-panel package to main's api/csrf/i18n. Idempotent. */
export function ensureAiPanelDeps(): void {
	if (inited) return;
	inited = true;
	initAiPanelDeps({
		apiBase: PUBLIC_JC_API_URL,
		surface: 'main',
		getCsrfToken,
		refreshCsrfToken,
		t,
		fullChatUrl: (c) => (c ? `/ai?c=${encodeURIComponent(c)}` : '/ai')
	});
}
```

- [ ] **Step 2: Modify the (app) layout**

In `+layout.svelte` (the file already imports Sparkles + has `showChatFab` at ~line 153 and the FAB markup at ~291):

```ts
import { AiPanel } from '@japaconnect/ai-panel';
import { ensureAiPanelDeps } from '$lib/ai/panel';
import { resolveAiPageContext } from '$lib/ai/pageContext';   // Task 8
import { setAiPageContext } from '@japaconnect/ai-panel';
import { afterNavigate } from '$app/navigation';

// §115: deps must init on EVERY page, not just on FAB click — /ai hides
// the FAB but JapaAIChat streams through the same package factory. Inside
// the existing onMount (client-only, idempotent).
onMount(() => ensureAiPanelDeps());

let aiPanelOpen = $state(false);

// §115: panel FAB replaces the /ai link — opens the slide-in panel instead.
const showChatFab = $derived(
	data.features?.['feature.japabot'] === true &&
		!page.route.id?.startsWith('/(app)/ai')
);

// Page-context registry: recompute on every data/route change; cleared
// while navigating so a stale entity context can't leak across pages.
$effect(() => {
	setAiPageContext(resolveAiPageContext(page.url.pathname, page.data));
});
afterNavigate(() => setAiPageContext(null));
```

Replace the FAB `<a>` block (~line 294) with:

```svelte
{#if showChatFab}
	<button
		type="button"
		onclick={() => (aiPanelOpen = !aiPanelOpen)}
		aria-label={t('ai.panel.title', 'Japa AI')}
		title={t('ai.panel.title', 'Japa AI')}
		aria-expanded={aiPanelOpen}
		class="fixed bottom-20 right-4 z-40 grid h-12 w-12 place-items-center rounded-full bg-[var(--color-primary)] text-[var(--color-primary-contrast)] shadow-lg transition-transform hover:scale-105 sm:bottom-5"
	>
		<Sparkles size={22} />
	</button>
	<AiPanel open={aiPanelOpen} user={data.user ?? null} onclose={() => (aiPanelOpen = false)} />
{/if}
```

- [ ] **Step 3: Typecheck + smoke**

```bash
cd apps/main && bun run check && bun run dev
```

Expected: FAB renders as before; clicking toggles a (currently generic, `panel:general`-anchored) panel; guest users see the sign-in CTA.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/routes/\(app\)/+layout.svelte apps/main/src/lib/ai/panel.ts
git commit -m "feat(main): AI quick panel — FAB toggle + deps wiring (§115)"
```

---

### Task 8: Main app — page-context extractor + `/ai?c=` deep link

**Files:**
- Create: `apps/main/src/lib/ai/pageContext.ts`
- Test: `apps/main/src/lib/ai/pageContext.test.ts`
- Modify: `apps/main/src/routes/(app)/ai/+page.svelte`
- Modify: `apps/main/src/lib/ai/JapaAIChat.svelte` (accept `initialConversationId` prop)

**Interfaces:**
- Produces: `resolveAiPageContext(pathname: string, data: Record<string, unknown>): AiPageContext` — entity extractors keyed on `pathname` prefixes; `path:{pathname}` fallback
- Consumes: `AiPageContext` from `@japaconnect/ai-panel`; `page.data.page` (WikiPage — `id`, `space_id`, `slug`, `title`, `category`), `page.data.spaceDetail` (`id`, `slug`, `name`), `page.data.article` (ContentArticle — `id`, `slug`, `title`, `content_type`, `space_id`, `tags`).

- [ ] **Step 1: Write the failing extractor tests**

`apps/main/src/lib/ai/pageContext.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { resolveAiPageContext } from './pageContext';

describe('resolveAiPageContext', () => {
	it('wiki page → wikipage anchor with full context', () => {
		const ctx = resolveAiPageContext('/wiki/toronto/housing', {
			page: { id: 'p1', space_id: 's1', slug: 'housing', title: 'Housing', category: 'guides' },
			spaceDetail: { id: 's1', slug: 'toronto', name: 'Toronto' }
		});
		expect(ctx?.anchor).toBe('wikipage:p1');
		expect(ctx?.context).toMatchObject({ content_type: 'wiki_page', content_id: 'p1', space_id: 's1', slug: 'housing', title: 'Housing' });
	});
	it('wiki space index → space anchor', () => {
		const ctx = resolveAiPageContext('/wiki/toronto', {
			spaceDetail: { id: 's1', slug: 'toronto', name: 'Toronto' }
		});
		expect(ctx?.anchor).toBe('space:s1');
	});
	it('blog post → content anchor', () => {
		const ctx = resolveAiPageContext('/blog/moving-to-canada', {
			article: { id: 'c9', slug: 'moving-to-canada', title: 'Moving', content_type: 'post' }
		});
		expect(ctx?.anchor).toBe('content:c9');
		expect(ctx?.context).toMatchObject({ content_type: 'post', content_id: 'c9' });
	});
	it('unknown path → path anchor with title only', () => {
		const ctx = resolveAiPageContext('/forum', { title: 'Forum' });
		expect(ctx?.anchor).toBe('path:/forum');
	});
	it('entity path with missing data → path anchor', () => {
		const ctx = resolveAiPageContext('/blog/moving-to-canada', {});
		expect(ctx?.anchor).toBe('path:/blog/moving-to-canada');
	});
});
```

Run: `cd apps/main && bunx vitest run src/lib/ai/pageContext.test.ts` → FAIL (module missing).

- [ ] **Step 2: Implement the extractor**

```ts
// apps/main/src/lib/ai/pageContext.ts
// §115 — pathname+data → AiPageContext. Entity extractors read API-loaded
// page data only (Rule 16); the path:{pathname} fallback gives every page
// a scoped thread (pathname chars are always anchor-safe — route.id
// values contain ()[] and are NOT usable in anchors).
import type { AiPageContext } from '@japaconnect/ai-panel';

type Data = Record<string, unknown>;

function str(v: unknown): string {
	return typeof v === 'string' ? v : '';
}

function pathAnchor(pathname: string, data: Data): AiPageContext {
	const title = str(data.title);
	return {
		anchor: `path:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}

function articleContext(d: Data): AiPageContext | null {
	const a = d.article as { id?: string; slug?: string; title?: string; content_type?: string; space_id?: string | null; tags?: { slug?: string }[] } | null;
	if (!a?.id) return null;
	return {
		anchor: `content:${a.id}`,
		label: `Blog · ${a.title ?? a.slug}`,
		context: {
			content_type: a.content_type ?? 'post',
			content_id: a.id,
			...(a.slug ? { slug: a.slug } : {}),
			...(a.title ? { title: a.title } : {}),
			...(a.space_id ? { space_id: a.space_id } : {}),
			...(Array.isArray(a.tags) && a.tags.length
				? { tags: a.tags.map((t) => str(t?.slug)).filter(Boolean) }
				: {})
		}
	};
}

function spaceContext(d: Data): AiPageContext | null {
	const space = (d.spaceDetail ?? d.space) as { id?: string; slug?: string; name?: string } | null;
	if (!space?.id) return null;
	return {
		anchor: `space:${space.id}`,
		label: `Wiki · ${space.name ?? space.slug}`,
		context: { content_type: 'space', space_id: space.id, slug: space.slug ?? '', title: space.name ?? '' }
	};
}

function wikiPageContext(d: Data): AiPageContext | null {
	const p = d.page as { id?: string; space_id?: string; slug?: string; title?: string; category?: string } | null;
	if (!p?.id) return null;
	return {
		anchor: `wikipage:${p.id}`,
		label: `Wiki · ${p.title ?? p.slug}`,
		context: {
			content_type: 'wiki_page',
			content_id: p.id,
			...(p.space_id ? { space_id: p.space_id } : {}),
			...(p.slug ? { slug: p.slug } : {}),
			...(p.title ? { title: p.title } : {})
		}
	};
}

export function resolveAiPageContext(pathname: string, data: Data): AiPageContext {
	const segs = pathname.split('/').filter(Boolean);
	// /wiki/{space_slug}/{page_slug} → page; /wiki/{space_slug} → space.
	// (/wiki/{space}/{page}/history shares the page anchor — it's the same entity.)
	if (segs[0] === 'wiki' && segs.length >= 3) {
		const ctx = wikiPageContext(data);
		if (ctx) return ctx;
	}
	if (segs[0] === 'wiki' && segs.length === 2) {
		const ctx = spaceContext(data);
		if (ctx) return ctx;
	}
	// Generic entity fallback: article-bearing pages (blog, content,
	// guides, stories, comparisons) and space-bearing pages get their
	// entity anchor regardless of the route segment shape.
	const viaArticle = articleContext(data);
	if (viaArticle) return viaArticle;
	const viaSpace = spaceContext(data);
	if (viaSpace) return viaSpace;
	return pathAnchor(pathname, data);
}
```

(Extend as other entity surfaces are audited — forum threads → `thread:{id}`, docs pages → `doc:{id}` — same pattern.)

- [ ] **Step 3: Verify tests pass**

`bunx vitest run src/lib/ai/pageContext.test.ts` → PASS.

- [ ] **Step 4: `/ai?c=` deep link**

`JapaAIChat.svelte`: add prop `initialConversationId?: string` to `Props`; in `onMount` (it already has one — check ~line 470), after initial state settles:

```ts
if (initialConversationId) void loadConversation(initialConversationId);
```

`+page.svelte`: pass `initialConversationId={page.url.searchParams.get('c') ?? undefined}` — needs `import { page } from '$app/state';`. A bad/foreign id already fails soft: `loadConversation` sets `error` and the UI shows it — acceptable per spec.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/ai/pageContext.ts apps/main/src/lib/ai/pageContext.test.ts apps/main/src/routes/\(app\)/ai/+page.svelte apps/main/src/lib/ai/JapaAIChat.svelte
git commit -m "feat(main): page-context extractor + /ai?c= conversation deep link (§115)"
```

---

### Task 9: i18n keys (all four apps)

**Files:**
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json`
- Modify: `apps/tools/src/lib/i18n/en.json`, `apps/tools/src/lib/i18n/fr.json`
- Modify: `apps/academy/src/lib/i18n/en.json`, `apps/academy/src/lib/i18n/fr.json`
- Modify: `apps/docs/src/lib/i18n/en.json`, `apps/docs/src/lib/i18n/fr.json`

- [ ] **Step 1: Add `ai.panel.*` keys (en + fr) to each app's catalog**

English set (match existing JSON style, alphabetical placement per file conventions):

```json
"ai.panel.title": "Japa AI",
"ai.panel.welcome": "Ask me anything — I can see what page you're on.",
"ai.panel.open_full": "Open in Japa AI",
"ai.panel.tools": "Agent tools",
"ai.panel.sign_in": "Sign in to chat",
"ai.panel.sign_in_help": "Japa AI is available to signed-in members.",
"ai.panel.error": "Something went wrong. Try again.",
"ai.panel.bg_running": "Working in the background…",
"ai.panel.stream_closed": "Stream closed.",
"ai.panel.placeholder": "Ask about this page…",
"ai.panel.send": "Send",
"ai.panel.stop": "Stop",
"ai.panel.close": "Close"
```

French translations follow each file's existing tone (e.g. `"ai.panel.title": "Japa AI"`, `"ai.panel.sign_in": "Connectez-vous pour discuter"`). Check how each app handles `ai.*` keys already — main has 253 of them; satellites may have none (still fine — `t` falls back to the English `fallback` arg anyway, keys make it translatable).

- [ ] **Step 2: Validate JSON**

```bash
for f in apps/{main,tools,academy,docs}/src/lib/i18n/{en,fr}.json; do bun -e "JSON.parse(require('fs').readFileSync('$f','utf8'))" || echo "BAD: $f"; done
```

- [ ] **Step 3: Commit**

```bash
git add apps/*/src/lib/i18n/{en,fr}.json
git commit -m "feat(i18n): ai.panel.* keys across user-facing apps (§115)"
```

---

### Task 10: tools app integration

**Files:**
- Modify: `apps/tools/package.json` (`@japaconnect/ai-panel` dep)
- Create: `apps/tools/src/lib/ai/panel.ts`
- Create: `apps/tools/src/lib/ai/pageContext.ts`
- Modify: `apps/tools/src/routes/+layout.svelte`
- Modify: `apps/tools/src/routes/+layout.server.ts` (only if `features`/`user` aren't already exposed — they are, line ~44-51)

- [ ] **Step 1: Deps module**

```ts
// apps/tools/src/lib/ai/panel.ts — same shape as main's, surface 'tools',
// fullChatUrl → absolute main app URL via existing mainAppUrl() helper.
import { PUBLIC_JC_API_URL } from '$env/static/public';
import { initAiPanelDeps } from '@japaconnect/ai-panel';
import { getCsrfToken, refreshCsrfToken } from '$lib/api/csrf.svelte';
import { t } from '$lib/i18n';
import { mainAppUrl } from '$lib/seo';

let inited = false;
export function ensureAiPanelDeps(): void {
	if (inited) return;
	inited = true;
	initAiPanelDeps({
		apiBase: PUBLIC_JC_API_URL,
		surface: 'tools',
		getCsrfToken,
		refreshCsrfToken,
		t,
		fullChatUrl: (c) => mainAppUrl(c ? `/ai?c=${encodeURIComponent(c)}` : '/ai')
	});
}
```

Check `$lib/api/csrf.svelte` exports `refreshCsrfToken` returning `Promise<boolean>` — compatible with `Promise<unknown>`.

- [ ] **Step 2: Extractor** — derive the tool anchor from the app's own `TOOLS` registry (`apps/tools/src/lib/tools/registry.ts` — `ToolMeta.route` ↔ `ToolMeta.slug`), never a hardcoded route list:

```ts
import { TOOLS } from '$lib/tools/registry';
import type { AiPageContext } from '@japaconnect/ai-panel';

export function resolveAiPageContext(pathname: string, data: Record<string, unknown>): AiPageContext {
	const tool = TOOLS.find((t) => pathname === t.route || pathname.startsWith(t.route + '/'));
	if (tool) {
		return {
			anchor: `tool:${tool.slug}`,
			label: tool.name,
			context: { content_type: 'tool', slug: tool.slug, title: tool.name }
		};
	}
	const title = typeof data.title === 'string' ? data.title : '';
	return {
		anchor: `path:${pathname}`.toLowerCase(),
		label: title || 'Page',
		context: title ? { title } : {}
	};
}
```

- [ ] **Step 3: Layout wiring** — same FAB+panel block as main (Task 7 Step 2), gated on `data.features?.['feature.japabot'] === true`, `user={data.user ?? null}`. Deps init in `onMount(() => ensureAiPanelDeps())` (same every-page rule as main — the FAB can be suppressed on any route). Suppress the FAB on `/japabot` (redirects anyway — harmless either way).

- [ ] **Step 4: Typecheck + commit**

```bash
cd apps/tools && bun run check
git add apps/tools
git commit -m "feat(tools): AI quick panel integration (§115)"
```

---

### Task 11: academy app integration

**Files:**
- Modify: `apps/academy/package.json`
- Create: `apps/academy/src/lib/ai/panel.ts`
- Create: `apps/academy/src/lib/ai/pageContext.ts`
- Modify: `apps/academy/src/routes/+layout.svelte`

- [ ] **Step 1: Deps module** — identical shape, `surface: 'academy'`, `fullChatUrl` via `MAIN_ORIGIN` from `$lib/appOrigins` (`${MAIN_ORIGIN}/ai?c=…`). Check the exact `X-JC-Surface` value academy sends — grep `x-jc-surface` in `apps/academy/src` and mirror it.

- [ ] **Step 2: Extractor** — `/tracks/[slug]` → `track:{track.id}` with `context: {content_type:'academy_track', content_id, slug, title}` (track row comes from `page.data.track`); lesson player routes (`/tracks/[slug]/lessons/[id]` or whatever exists — check `find apps/academy/src/routes -name "+page.server.ts" | xargs grep -l lesson`) → `lesson:{id}` + `content_type:'academy_lesson'`; fallback `path:{pathname}`.

- [ ] **Step 3: Layout wiring** — same FAB+panel pattern, `data.features['feature.japabot']`, `user={data.user}`.

- [ ] **Step 4: Typecheck + commit**

```bash
cd apps/academy && bun run check
git add apps/academy
git commit -m "feat(academy): AI quick panel integration (§115)"
```

---

### Task 12: docs app integration

**Files:**
- Modify: `apps/docs/package.json`
- Create: `apps/docs/src/lib/api/csrf.ts` (docs has no csrf store — cookie-based adapter)
- Create: `apps/docs/src/lib/ai/panel.ts`
- Create: `apps/docs/src/lib/ai/pageContext.ts`
- Modify: `apps/docs/src/routes/+layout.svelte`

- [ ] **Step 1: CSRF adapter**

Docs reads the token from the `csrf_token` cookie (mirrored server-side in `hooks.server.ts:222-236`). `apps/docs/src/lib/api/csrf.ts`:

```ts
import { env } from '$env/dynamic/public';
import { buildApiUrl } from '@japaconnect/api-url';
import { getCookie } from '$lib/api/client';

export function getCsrfToken(): string | null {
	return getCookie('csrf_token') ?? null;
}

export async function refreshCsrfToken(): Promise<void> {
	const base = (env.PUBLIC_JC_API_URL ?? '/api/v1').replace(/\/+$/, '');
	await fetch(buildApiUrl('/csrf', base), { credentials: 'include' });
}
```

(`getCookie` already exists in `apps/docs/src/lib/api/client.ts:51`.)

- [ ] **Step 2: Deps module** — `surface: 'docs'` (matches `JC_SURFACE` in `apps/docs/src/lib/api/client.ts:19`), csrf from the new adapter, `fullChatUrl` → absolute main-app URL using the established docs pattern already used in 5 components (`GuestAuthButtons`, `UserDropdown`, `GuestHeader`, `GuestFooter`, `HelpfulWidget`): `(env.PUBLIC_MAIN_ORIGIN ?? 'http://localhost:3000').replace(/\/$/, '')`.

- [ ] **Step 3: Extractor** — `/docs/[audience]/[slug]` → `doc:{page.id}` with `context: {content_type:'doc', content_id, slug, title, doc_category: page.category, doc_subcategory: page.subcategory, audience}` (page row shape: `data.page` per `loadDocPage` — `id`, `title`, `slug`, `category`, `subcategory`, `audience`, `parent_id`); `/docs/api/[slug]` → same treatment from its data; index/category routes → `path:{pathname}`.

- [ ] **Step 4: Layout wiring** — FAB+panel block, `data.features?.['feature.japabot']`, `user={data.user}`. Hide FAB on `/docs/*/new` editor routes (panel competes with the authoring layout) — optional, harmless to show.

- [ ] **Step 5: Typecheck + commit**

```bash
cd apps/docs && bun run check
git add apps/docs
git commit -m "feat(docs): AI quick panel integration (§115)"
```

---

### Task 13: Verification sweep

- [ ] **Step 1: Backend**

```bash
go build ./... && go test ./internal/ai/ -count=1
```

- [ ] **Step 2: All app typechecks + unit tests**

```bash
for app in main tools academy docs; do (cd apps/$app && bun run check && bun run test 2>/dev/null | tail -3); done
```

- [ ] **Step 3: Manual matrix (dev servers)**

Run backend + each app; verify per spec §9: wiki page agent ops (`wiki_page.submit_revision`, `wiki_page.create`), blog `content.update`, docs `docs.update`, chat-only user (context grounding, no tools toggle), guest sign-in CTA, same-thread-across-devices via anchor, `/ai?c=` opens the same thread, FAB hidden on `/ai`.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat: AI quick panel — page-aware slide-in chat with anchored threads (§115)"
```
