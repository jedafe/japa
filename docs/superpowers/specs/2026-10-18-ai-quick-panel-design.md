# Design — Japa AI Quick Panel (page-aware slide-in chat)

**Date:** 2026-10-18
**Status:** Approved (design review in session)
**Surfaces:** apps/main, apps/docs, apps/academy, apps/tools (all user-facing), `internal/ai`
**JEOS refs:** §103 (ai-agent-executor), §20 (ai-core), ADR-130 (unified-japa-ai). New section file required before code (see §10 below).

## Problem

Japa AI today lives only at `/ai` — a full-page surface. Acting on the page
you're looking at (update a wiki page, draft a child page, edit a blog post,
revise a doc) means leaving the page, navigating to `/ai`, and re-describing
the target. There is no ambient, page-aware AI surface.

## Goals

- A floating AI button (existing Sparkles FAB pattern) on every user-facing
  app that opens a **slide-in right panel** instead of navigating to `/ai`.
- The panel is **page/URL-aware**: it derives entity context from the current
  route (wiki page, blog post, doc, space, tool, lesson) and sends it with
  every turn so the agent can act on that entity and its subpages.
- **Page-scoped threads**: each anchored page gets one persistent
  conversation — reopening the panel on the same page resumes the same
  thread, durable and cross-device.
- The panel serves **normal chat** for `ai.context.use` users and **agent
  turns** (tool execution) for `agent.use` users — same OR admission as
  `POST /ai/chat`.
- A "Open in Japa AI" CTA deep-links the same conversation into the full
  `/ai` surface.

## Non-goals (v1)

- No chat templates, projects, model/voice pickers, or conversation sidebar
  in the panel — it is deliberately lightweight. Full features remain at `/ai`.
- No per-tool/category pickers — the tools toggle is all-or-nothing, bounded
  by the user's AI prefs (`disabled_categories`/`disabled_tools` apply
  server-side as today).
- No mobile-app (Flutter) panel.
- No changes to the tool catalog itself — existing `wiki_page.*`,
  `content.*`, `docs.*`, `academy.*`, `tools.*` tools already cover the
  described operations. (`wiki_page.create` takes space + category; "a page
  under this one" maps to same-space/same-category creation.)
- Guests cannot chat (chat requires auth); the panel shows a sign-in CTA.

## 1. Backend — conversation anchors

### Schema (new migration)

```sql
ALTER TABLE ai_conversations ADD COLUMN anchor text;

CREATE UNIQUE INDEX idx_ai_conversations_anchor
    ON ai_conversations (user_id, anchor)
    WHERE anchor IS NOT NULL AND deleted_at IS NULL;
```

`anchor` is an opaque client-supplied key identifying the page/entity a
thread belongs to, e.g. `wikipage:{page_id}`, `space:{space_id}`,
`content:{content_id}`, `doc:{doc_id}`, `tool:{slug}`, `lesson:{id}`,
`route:{route_id}` (section-level fallback). Soft-deleting a conversation
frees the anchor — the next send creates a fresh thread (desired semantics:
"clear this page's chat" = delete the thread).

### sqlc queries (`db/queries/` — never edit `internal/db`, Rule 2)

- `GetAIConversationByAnchor(user_id, anchor)` → live conversation or no rows.
- Extend `CreateAIConversation` with the `anchor` param.

### `ConversationService.Resolve` (internal/ai/conversations.go)

New signature: `Resolve(ctx, userID, conversationID, projectID *pgtype.UUID, title, anchor string)`.
Resolution order:

1. Valid `conversationID` → return it (existing behavior).
2. `anchor` non-empty → `GetAIConversationByAnchor`; hit → return it.
3. Miss → `CreateAIConversation` with `anchor` stamped. On `23505`
   (concurrent first-send on the same page) → re-run step 2 once.

Call sites: `agent_unified.go` (unified chat path) is the only caller that
passes anchor; other callers pass `""`.

### `UnifiedChatRequest` (internal/ai/unified_handlers.go)

- New field `anchor string`.
- `sanitizeAnchor`: trim, lowercase, reject unless it matches
  `^[a-z0-9_.:/-]{1,200}$`. Invalid anchors are dropped with a warn log —
  never a request error (a bad anchor must not block chat).
- `anchor` is a request field, NOT a context key — it never enters the
  prompt and needs no `allowedContextKeys` entry.

### `GET /ai/conversations` — `?anchor=` filter

Add an `anchor` query param to the list handler so the panel can discover
an existing thread for the current page before the first send
(cross-device resume). Returns the filtered list (0 or 1 items); all other
list behavior unchanged. Same gate as today.

No new endpoints otherwise. `POST /ai/chat` stays the single send path —
`feature.japabot` + `ai.context.use` OR `agent.use` + moderate rate limit.

## 2. Shared package — `@japaconnect/ai-panel`

New workspace package `packages/ai-panel` (alongside `packages/ui`;
workspaces already cover `packages/*`). Peer deps mirror `@japaconnect/ui`
(svelte 5, lucide-svelte, tailwind 4, bits-ui). Each app adds
`"@japaconnect/ai-panel": "workspace:*"`.

The package cannot import app `$lib` modules (i18n, api client, csrf,
`$env`), so every app dependency is **injected**:

```ts
export interface AiPanelDeps {
  apiBase: string;                                  // PUBLIC_JC_API_URL
  getCsrfToken: () => string | null;
  refreshCsrfToken: () => Promise<void>;
  surface: string;                                  // X-JC-Surface value
  t: (key: string, fallback?: string) => string;    // app i18n fn
  fullChatUrl: (conversationId?: string) => string; // "Open in Japa AI" CTA
}
```

Exports:

- **`streamChat`** — `apps/main/src/lib/ai/unifiedStream.ts` moved here as a
  factory `createStreamChat(deps) → (opts) => AsyncGenerator<AgentEvent>`.
  `StreamChatOptions` gains `anchor?: string`. `JapaAIChat.svelte` re-imports
  from the package (no duplicated SSE logic).
- **`agentTypes.ts`** — moved verbatim (types + `AgentRequestError`); main
  re-imports.
- **`aiPageContext`** — rune store (`pageContext.svelte.ts`):
  ```ts
  interface AiPageContext {
    anchor: string;                       // REQUIRED for thread scoping
    label: string;                        // context chip text
    context: Record<string, unknown>;     // allowlisted keys only
  }
  setAiPageContext(ctx | null); getAiPageContext();
  ```
- **`AiPanel.svelte`** — the panel component (below).

## 3. `AiPanel.svelte` — lightweight quick-chat

Fixed right drawer: `~400px` desktop, full-screen sheet under `640px`,
slides in/out on `panelOpen`, `z-index` above shell chrome, focus-trapped,
`Esc` closes, stays mounted while open (transcript survives open/close).

**Header:** "Japa AI" + context chip (`label` from `aiPageContext`, hidden
when no context) + "Open in Japa AI" icon-button CTA (disabled until a
conversation exists) + close button.

**Transcript:** trimmed versions of `JapaAIChat`'s renderers — user/
assistant bubbles, `delta` streaming text, `tool_call_start`/
`tool_call_result` pills (name + status icon), `progress` line, `sources`
citations (`AiSourceCitations`-equivalent moved to package), error turns.
No sidebar, no projects, no templates.

**Composer:** textarea + send + stop (AbortController). For `agent.use`
users (detected via `GET /ai/agent/capabilities` on first open; 403/flag-off
→ hide), a tools toggle — **default ON** in the panel (its purpose is agent
ops), sending `tools: { enabled: true }`. Non-agent users send no `tools`
field → pure chat.

**Guest state:** FAB opens panel showing "Sign in to chat" CTA →
`fullChatUrl()` → login redirect flow (mirrors tools `/japabot` page).

**Per-anchor thread flow:**

1. Panel opens → read `aiPageContext` → `GET /ai/conversations?anchor=…`
   → load messages via `GET /ai/conversations/{id}` (existing). No context →
   anchor is `route:{routeId}` (one thread per route pattern); routeId
   unavailable → `panel:general`.
2. First send → `streamChat({message, anchor, context, tools?})` — backend
   resolves-or-creates the anchored thread and returns `conversation_id` on
   the `conversation`/`final_response` events → panel stores it.
3. Subsequent sends → `conversation_id` (anchor no longer needed).
4. Route change while open → context chip updates; if the anchor changed,
   the transcript swaps to the new anchor's thread (fetch as in step 1).
   In-flight streams keep running server-side; the existing detach-and-poll
   pattern (`findRunningSessionId` + session poll) is reused in trimmed form.

**Context payload:** sent on EVERY turn (chat and agent) when
`aiPageContext` is set — chat turns gain read-level grounding ("summarize
this page") with zero capability leak since non-agent users receive no
tools. Agent turns additionally pass `context` into `USER CONTEXT (JSON)`
so the LLM picks the right tool arguments (content_id, space_id, slug…).

## 4. Page-context registry — per-app extractors

Each app ships `src/lib/ai/pageContext.ts`: a table mapping `route.id` →
`(data) => AiPageContext | null`. Layouts call it in an `$effect` on
`page.route.id`/`page.data` changes (set or clear). Extractors read
already-loaded page data — no extra fetches (Rule 16 compliant: all ids
come from API-loaded page data).

**apps/main:**

| Route | anchor | context |
|---|---|---|
| `/(app)/wiki/[space_slug]/[page_slug]` | `wikipage:{page_id}` | `content_type:'wiki_page'`, `content_id`, `space_id`, `slug`, `title` |
| `/(app)/wiki/[space_slug]` (incl. `/create`, `/history`) | `space:{space_id}` | `content_type:'space'`, `space_id`, `slug`, `title` |
| `/(app)/blog/[slug]` | `content:{content_id}` | `content_type:'post'` (or actual), `content_id`, `slug`, `title`, `tags` |
| `/docs/[...rest]` | `doc:{doc_id-or-path}` | `content_type:'doc'`, `slug`, `title`, `doc_category` |
| `/(app)/content/[slug]`, guides, stories | `content:{id}` | same as blog |
| `/(app)/spaces/[slug]` (+ subpages) | `space:{id}` | `space_id`, `slug`, `title` |
| anything else | `route:{route_id}` | `title` (page title) only |

**apps/docs:** doc pages → `doc:{id/path}`; section index → `route:{route_id}`.
**apps/academy:** track/lesson pages → `track:{id}` / `lesson:{id}` +
`content_type:'academy_track'|'academy_lesson'`; else `route:{route_id}`.
**apps/tools:** tool pages → `tool:{slug}`; else `route:{route_id}`.

## 5. Per-app integration

**apps/main** — `src/routes/(app)/+layout.svelte`:
- Existing Sparkles FAB (currently `<a href="/ai">`) becomes a `<button>`
  toggling the panel. Same visibility rules: `feature.japabot`, hidden on
  `/(app)/ai*` routes, visible to guests (panel shows sign-in CTA).
- Mount `<AiPanel>` next to the FAB; wire `pageContext` extractor; deps:
  main's `csrf.svelte` helpers, `PUBLIC_JC_API_URL`, `surface:'main'`,
  `t` from `$lib/i18n`, `fullChatUrl: (c) => c ? '/ai?c='+c : '/ai'`.

**apps/docs / academy / tools** — same three additions in each root
layout: FAB, `<AiPanel>` with app-local csrf/api/i18n deps and `surface`
matching that app's existing `X-JC-Surface` value, and the pageContext
extractor. `fullChatUrl` → absolute main-app URL (`tools` has
`mainAppUrl()`; `academy` has `appOrigins.ts`; docs gets an equivalent).
FAB suppressed on each app's AI/chat routes if any.

**`/ai` deep link** — `apps/main/src/routes/(app)/ai/+page.svelte` +
`JapaAIChat`: on mount, `?c={id}` → `loadConversation(id)` (function
exists at JapaAIChat.svelte:974). Unknown/foreign id → toast-less silent
fallback to new chat (same as a bad sidebar click today).

## 6. Data flow (agent turn on a wiki page)

1. User on `/wiki/toronto/housing` → extractor sets
   `{anchor:'wikipage:…', label:'Wiki · Housing',
     context:{content_type:'wiki_page', content_id, space_id, slug:'housing', title:'Housing'}}`.
2. FAB → panel opens → `GET /ai/conversations?anchor=wikipage:…` → resume
   or empty transcript.
3. "Add a section about rental prices" → `streamChat` POSTs
   `{message, anchor, context, tools:{enabled:true}}` → SSE events render
   inline (`wiki_page.read` → `wiki_page.submit_revision` pills, text
   deltas, final response + sources).
4. Backend: `sanitizeAnchor` → `Resolve` finds/creates anchored thread →
   context lands in `USER CONTEXT (JSON)` → tool loop runs → HITL-gated
   ops queue for review as usual.
5. "Open in Japa AI" → `/ai?c={conversation_id}` → full surface resumes
   the same thread (tools, templates, history all available there).

## 7. Error handling

- **Pre-check failures** (401/403/429/402/`ai_*` codes): `AgentRequestError`
  → inline error turn, same i18n surface as `/ai`.
- **Anchor race**: `23505` → server re-looks-up by anchor (one retry, same
  pattern as `errConversationBusy` sweep in `agent_unified.go`).
- **Stale anchor / deleted thread**: `?anchor=` lookup misses naturally →
  new thread; a `conversation_id` 404 on a send → retry once with anchor.
- **Stream drop mid-turn**: detach-and-poll pattern (trimmed from
  JapaAIChat).
- **Flag flips / consent errors**: `ai_consent_required`,
  `ai_assist_disabled`, `feature_japabot` 404s render as error turns;
  capabilities fetch failure → tools toggle hidden, chat unaffected.
- **Context staleness**: `aiPageContext` cleared on every navigation before
  the next page's extractor runs — a wiki context can never leak onto a
  blog page.
- **Anchor hygiene**: only `[a-z0-9_.:/-]` ≤200 chars reaches the DB;
  invalid → dropped server-side with warn log, chat proceeds unanchored.

## 8. Security & privacy

- `anchor` is per-user scoped (`UNIQUE(user_id, anchor)`), never shared —
  no enumeration vector beyond the user's own threads.
- `context` stays inside the existing `allowedContextKeys` allowlist +
  scalar/flat-array validation (ADR-010 anti-injection). No new keys needed.
- All existing gates preserved: `feature.japabot`/`feature.ai_agent` flags,
  `ai.context.use`/`agent.use` OR admission, per-tool RBAC + privacy checks
  in the executor, HITL queue, `ai_processing` consent, daily budget caps,
  moderate rate limit.
- `X-JC-Surface` continues to scope feature-flag resolution per app.

## 9. Testing

- **Go:** `Resolve` anchor tests (hit / create / race / post-delete
  re-anchor); `sanitizeAnchor` table test; unified handler anchor plumb
  test (alongside `unified_handlers_test.go` context tests);
  `?anchor=` list filter test.
- **Frontend (vitest):** pageContext extractor table tests (route+data →
  context); store set/clear-on-navigate; AiPanel composer payload
  (context+anchor+tools fields); capabilities-403 → toggle hidden.
- **Manual matrix:** wiki page "update this page" →
  `wiki_page.submit_revision`; "create a child page" → `wiki_page.create`
  same space/category; blog post → `content.update`; docs page →
  `docs.update`; chat-only user → no tools toggle, context-grounded
  answers; guest → sign-in CTA; same page across two devices → same thread.

## 10. JEOS bookkeeping (before code — AGENTS.md)

- New section file `.jeos/sections/115-ai-quick-panel.md` (next free
  number) capturing this spec: schema change, anchor contract, package
  layout, per-app integration, route catalog additions
  (`?anchor=` param on `GET /ai/conversations` in
  `conventions/api-routes-catalog.md`).
- ADR-130 gets a short amendment note (unified surface gains the
  page-aware panel + anchored threads); no new ADR.
- i18n keys under `ai.panel.*` added to each app's catalog.
