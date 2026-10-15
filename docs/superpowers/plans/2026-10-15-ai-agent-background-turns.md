# AI Agent Background Turns Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tool-enabled `/ai` turns complete server-side after client disconnect; the conversation surfaces a running session + auto-polls to land the result; templates gain a `preview`/`create` intent that steers tool choice.

**Architecture:** `runLoop` gets a `detached` flag — detached turns run on `context.WithoutCancel` with non-blocking event delivery; a new sqlc query exposes the in-flight session id on conversation detail so the client can poll `GET /ai/agent/sessions/{id}`; `ChatTemplateValues.intent` rides the opaque prefs blob (no backend schema change).

**Spec:** `docs/superpowers/specs/2026-10-15-ai-agent-background-turns-design.md`

**Tech Stack:** Go + chi + sqlc + pgx + Valkey; SvelteKit + Svelte 5 runes; Bun/Vitest.

## Global Constraints

- JEOS is the source of truth and is updated BEFORE code (Task 1).
- Never hand-edit `internal/db/` generated files — edit `internal/db/queries/*.sql`, run `make sqlc-gen`.
- No `npm`/`npx`/`pnpm`/`yarn` — use `bun`/`bunx` (Rule 18).
- Commit messages and code must carry NO AI attribution (Rule 17).
- All commits go to `main`.
- New API errors use `{ error: { code: "snake_case", ... } }` (Rule 5).
- `agent.skip_hitl` bypass is already live — do not re-implement it.

---

### Task 1: JEOS spec updates

**Files:**
- Modify: `.jeos/sections/103-ai-agent-executor.md` (append amendment block at the end, before "See Also")
- Modify: `.jeos/conventions/api-routes-catalog.md:906` (`GET /ai/conversations/{id}` line) and `:916` (`PUT /me/ai-preferences` line)
- Modify: `.jeos/sections/11-job-queue-asynq.md` (add catalog note near `agent:create_draft_request`, line ~71)

- [ ] **Step 1: Append the §103 amendment**

Append before `## See Also` in `sections/103-ai-agent-executor.md`:

```markdown
### Amendment — detached tool-enabled turns (background runs)

- **Detach semantics.** Unified turns whose resolved tool catalog is
  non-empty (tools on + `agent.use`) run detached: `runLoop` executes on
  `context.WithoutCancel(r.Context())` bounded by `agent.timeout_seconds`.
  SSE is a view of the run, not its lifecycle — a client disconnect drops
  live events but the turn completes, the session row reaches a terminal
  status, and the assistant message persists (all session/transcript
  writes already use `WithoutCancel`). Pure-chat turns (empty catalog)
  stay request-bound: a disconnected conversational reply is discarded.
  The legacy `/ai/agent` path always runs detached — it is agent-only by
  definition.
- **Event delivery under detach.** `sendEvent` becomes a non-blocking
  try-send on detached turns; `final_response`/`error` get a ~2s blocking
  grace so a connected-but-slow consumer still receives the end marker.
  Audit lives in `ai_agent_sessions`/`ai_agent_tool_calls`, never in the
  stream.
- **Failure transcript notice.** Detached terminal paths (error,
  max-iterations, cancellation) append a best-effort assistant message so
  the thread is never silently unanswered; attached turns keep SSE-only
  errors.
- **`running_session_id` contract.** `GET /ai/conversations/{id}` returns
  `running_session_id` when a non-stale `running` session exists for the
  thread (same cutoff semantics as `HasRunningAgentTurnForConversation`).
  Clients poll `GET /ai/agent/sessions/{id}` to land the result. The
  existing `POST /ai/agent/sessions/{id}/cancel` + Valkey
  `jc:agent:session:{id}` cancel check work unchanged on detached turns.
- **Future phase (spec'd, not built):** an `ai:agent_run` Asynq task that
  survives API restarts, with the SSE stream served from a Valkey event
  buffer on `jc:agent:session:{id}` (replay + resume for re-attached
  clients).
```

- [ ] **Step 2: Update the routes catalog**

Line 906 becomes:

```
GET    /ai/conversations/{id}            detail; includes running_session_id when a non-stale agent turn is in-flight (detached runs, §103)
```

Extend the `PUT /me/ai-preferences` line (916) — after "folders are derived, not stored entities" append:

```
; values.intent selects insert behavior — 'preview' (draft text in chat) or 'create' (persist via create tools)
```

- [ ] **Step 3: Add the §11 catalog note**

After the `agent:create_draft_request` catalog entry (~line 71), add:

```markdown
agent:agent_run                  — deferred/background agent session run (§103 detached-turn follow-up phase; spec'd, not yet implemented).
```

- [ ] **Step 4: Commit**

```bash
git add .jeos/sections/103-ai-agent-executor.md .jeos/conventions/api-routes-catalog.md .jeos/sections/11-job-queue-asynq.md docs/superpowers/specs/2026-10-15-ai-agent-background-turns-design.md
git commit -m "jeos: detached agent turns, running_session_id, template intent (§103)"
```

(The spec file may already be committed — `git status` first.)

---

### Task 2: Detached execution + failure notice (backend)

**Files:**
- Modify: `internal/ai/agent.go` — `loopParams` (~line 543), `runLoop` head (~568-584), terminal paths (~897-934), legacy call site (~420)
- Modify: `internal/ai/agent_unified.go` — unified call site (~308)
- Test: `internal/ai/agent_unified_test.go` (harness: `newUnifiedHandler(t, provider, perms, opts)`)

**Interfaces:**
- Produces: `loopParams.detached bool`; `persistTerminalNotice(ctx, p, session, notice string)`.
- Consumes: existing `sendEvent` semantics — detached mode must ALWAYS return true (early-abort paths keyed on its return must not fire).

- [ ] **Step 1: Write the failing tests**

`recordingProvider` returns instantly and ignores ctx, so a plain
cancel-race can't distinguish attached from detached (the loop may finish
before cancel lands — and the terminal `sendEvent` return value isn't
checked, so a late-cancelled attached run would still "complete"). Use a
**gated provider** that blocks until released and honors `ctx.Done()`
like a real LLM client. Add to `agent_unified_test.go`:

```go
// gatedProvider blocks ChatWithTools until release is closed; a cancelled
// ctx aborts the call like a real LLM client would. Non-optional for the
// detach tests — the instant recordingProvider makes cancel races
// nondeterministic.
type gatedProvider struct {
	recordingProvider
	release chan struct{}
}

func (p *gatedProvider) ChatWithTools(ctx context.Context, req ChatWithToolsRequest) (ChatWithToolsResponse, error) {
	p.reqs = append(p.reqs, req)
	select {
	case <-p.release:
	case <-ctx.Done():
		return ChatWithToolsResponse{}, ctx.Err()
	}
	return ChatWithToolsResponse{Content: p.reply, FinishReason: "stop", Usage: TokenUsage{TotalTokens: 42}}, nil
}

// A tools-on turn (agent.use → non-empty catalog → detached) survives
// client disconnect: the loop completes and the terminal event still
// lands for a connected reader.
func TestUnifiedChat_DetachedTurnCompletesAfterCancel(t *testing.T) {
	provider := &gatedProvider{recordingProvider: recordingProvider{name: "mock", reply: "done"}, release: make(chan struct{})}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"agent.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := h.HandleUnifiedChat(ctx, uid, UnifiedChatRequest{Prompt: "run a task"})
	if err != nil {
		t.Fatal(err)
	}

	// session_start proves the loop is parked on the gated LLM call.
	if first := <-events; first.Type != EventSessionStart {
		t.Fatalf("expected session_start first, got %v", first.Type)
	}
	cancel()                  // client disconnects
	close(provider.release)   // the (detached) LLM call can now return

	got := []AgentEvent{}
	done := make(chan struct{})
	go func() {
		for ev := range events {
			got = append(got, ev)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("detached loop stalled")
	}
	if findEvent(got, EventFinalResponse) == nil {
		t.Fatalf("expected final_response after disconnect, got %+v", got)
	}
}

// Control: a pure-chat turn (ai.context.use only → empty catalog →
// attached) stays request-bound — cancelling mid-loop fails the run
// instead of finishing it.
func TestUnifiedChat_ChatTurnStillAbortsOnCancel(t *testing.T) {
	provider := &gatedProvider{recordingProvider: recordingProvider{name: "mock", reply: "done"}, release: make(chan struct{})}
	h, _, _ := newUnifiedHandler(t, provider, &stubPerms{granted: map[string]bool{"ai.context.use": true}})
	uid := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}

	ctx, cancel := context.WithCancel(context.Background())
	events, err := h.HandleUnifiedChat(ctx, uid, UnifiedChatRequest{Prompt: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	<-events // session_start — loop is parked on the gated call
	cancel()
	close(provider.release) // provider sees ctx.Done and fails the call

	got := []AgentEvent{}
	for ev := range events {
		got = append(got, ev)
	}
	if findEvent(got, EventFinalResponse) != nil {
		t.Fatal("attached chat turn must not complete after disconnect")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/jmobile/websites/japa/japaconnect && go test ./internal/ai/ -run 'DetachedTurn|ChatTurnStillAborts' -v`
Expected: `DetachedTurnCompletesAfterCancel` FAILS (loop aborts after cancel — no `final_response`); the control test may already pass today (everything is request-bound).

- [ ] **Step 3: `loopParams` + ctx detach**

In `agent.go` `loopParams` add:

```go
	// detached marks tool-enabled turns that complete server-side even
	// when the SSE consumer disconnects — the request context is dropped
	// for the working ctx and sendEvent becomes non-blocking (§103
	// amendment: detached turns).
	detached bool
```

In `runLoop`, immediately after `defer close(events)` and before `sendEvent` is defined:

```go
	// Detached (tool-enabled) turns complete server-side even when the SSE
	// consumer disconnects — the stream is a view of the run, not its
	// lifecycle. Persistence already uses WithoutCancel; this detaches the
	// working context so in-flight LLM/tool calls aren't cancelled.
	if p.detached {
		ctx = context.WithoutCancel(ctx)
	}
```

- [ ] **Step 4: Non-blocking `sendEvent` on detached turns**

Replace the `sendEvent` closure body:

```go
	sendEvent := func(ev AgentEvent) bool {
		if !p.detached {
			select {
			case events <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		}
		// Detached: nobody may be reading — never block the loop on a full
		// channel. Terminal events get a brief grace so a still-connected
		// consumer receives the end marker.
		select {
		case events <- ev:
			return true
		default:
		}
		if ev.Type != EventFinalResponse && ev.Type != EventError {
			return true
		}
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case events <- ev:
		case <-timer.C:
		}
		return true
	}
```

- [ ] **Step 5: `persistTerminalNotice` + wire the terminal paths**

Add the helper near `runLoop` (mirrors the success-path persist call at ~line 947):

```go
// persistTerminalNotice writes a best-effort assistant message recording a
// detached turn's terminal failure/cancellation — a background task that
// dies must not leave the transcript silently unanswered. Attached turns
// keep SSE-only errors (the client renders the live error event).
func (h *AgentHandler) persistTerminalNotice(ctx context.Context, p loopParams, session gen.AiAgentSession, notice string) {
	if !p.detached || p.unified == nil || !p.unified.hasConv || h.convos == nil {
		return
	}
	persistCtx := context.WithoutCancel(ctx)
	msg, err := h.convos.AppendAssistantMessage(persistCtx, p.unified.conv.ID, notice, p.model, p.unified.req.VoiceID, 0, 0, &session.ID)
	if err != nil {
		slog.Warn("ai: failed to persist terminal notice", "session_id", session.ID, "error", err)
		return
	}
	if err := h.convos.LinkSession(persistCtx, session.ID, p.unified.conv.ID, msg.ID); err != nil {
		slog.Warn("ai: failed to link terminal notice session", "session_id", session.ID, "error", err)
	}
	if err := h.convos.Touch(persistCtx, p.unified.conv.ID); err != nil {
		slog.Warn("ai: failed to touch conversation", "conversation_id", p.unified.conv.ID, "error", err)
	}
}
```

In the three terminal blocks, after `trackCost()`:

```go
	// cancelled path:
	h.persistTerminalNotice(ctx, p, session, "The agent task was cancelled.")

	// loopErr path:
	h.persistTerminalNotice(ctx, p, session, fmt.Sprintf("The agent task failed: %s", loopErr.Error()))

	// maxedOut / empty-response path (same, after loopErr is set):
	h.persistTerminalNotice(ctx, p, session, fmt.Sprintf("The agent task failed: %s", loopErr.Error()))
```

- [ ] **Step 6: Set `detached` at both call sites**

Legacy `HandleAgent` call site (~line 420) — always an agent run:

```go
	go h.runLoop(ctx, session, userID, loopParams{
		prompt:        prompt,
		geoSpaceID:    geoSpaceID,
		geoSummary:    geoSummary,
		preSearch:     preSearchResult,
		draftCtx:      draftCtx,
		agentCounters: true,
		detached:      true, // the legacy path is agent-only by definition
	}, events)
```

Unified call site (`agent_unified.go` ~line 308):

```go
		detached:      len(catalog) > 0, // tools on → run to completion in background
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/ai/ -run 'TestUnifiedChat|TestAgent' -v`
Expected: new test passes; existing tests unchanged.

- [ ] **Step 8: vet + commit**

```bash
go vet ./internal/ai/ && gofmt -l internal/ai/agent.go internal/ai/agent_unified.go
git add internal/ai/agent.go internal/ai/agent_unified.go internal/ai/agent_unified_test.go
git commit -m "ai: detach tool-enabled turns from request lifecycle + failure transcript notice"
```

---

### Task 3: `running_session_id` on conversation detail (backend)

**Files:**
- Modify: `internal/db/queries/ai_conversations.sql` (after `HasRunningAgentTurnForConversation`, ~line 133)
- Regenerate: `make sqlc-gen` (writes `internal/db/`)
- Modify: `internal/ai/conversations.go` — `ConversationStore` interface (~line 45) + new `RunningSessionID` method (near `HasRunningTurn`, ~line 247)
- Modify: `internal/ai/unified_handlers.go` `serveUnifiedDetail` (~line 486)
- Test: `internal/ai/conversations_test.go` — stub method + `RunningSessionID` test

**Interfaces:**
- Produces: `ConversationService.RunningSessionID(ctx, convID, userID pgtype.UUID, cutoff time.Time) (pgtype.UUID, error)`; detail JSON field `running_session_id` (absent when none).
- Consumes: sqlc `GetRunningAgentSessionForConversation`.

- [ ] **Step 1: Add the query**

In `ai_conversations.sql`, after `HasRunningAgentTurnForConversation` (~line 133):

```sql
-- ADR-130 detached turns: the in-flight agent session id for a
-- conversation so the client can poll it after a disconnect or on
-- return. Same staleness cutoff semantics as
-- HasRunningAgentTurnForConversation.
-- name: GetRunningAgentSessionForConversation :one
SELECT id FROM ai_agent_sessions
WHERE conversation_id = @conversation_id
  AND user_id = @user_id
  AND status = 'running'
  AND started_at > @running_since
ORDER BY started_at DESC
LIMIT 1;
```

Run: `make sqlc-gen`

- [ ] **Step 2: Store interface + service method**

In `conversations.go` `ConversationStore`, after `HasRunningAgentTurnForConversation`:

```go
	GetRunningAgentSessionForConversation(ctx context.Context, arg gen.GetRunningAgentSessionForConversationParams) (pgtype.UUID, error)
```

New method near `HasRunningTurn`:

```go
// RunningSessionID returns the conversation's in-flight agent session id
// (invalid UUID when none) — same staleness semantics as HasRunningTurn.
func (s *ConversationService) RunningSessionID(ctx context.Context, convID, userID pgtype.UUID, cutoff time.Time) (pgtype.UUID, error) {
	id, err := s.store.GetRunningAgentSessionForConversation(ctx, gen.GetRunningAgentSessionForConversationParams{
		ConversationID: convID,
		UserID:         userID,
		RunningSince:   pgtype.Timestamptz{Time: cutoff, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	return id, err
}
```

Add the stub method to `stubConvoStore` in `conversations_test.go` (returns `pgtype.UUID{}, pgx.ErrNoRows` by default; add a `running map[pgtype.UUID]pgtype.UUID` field keyed by conversation when a test needs a running row).

- [ ] **Step 3: Emit `running_session_id` in `serveUnifiedDetail`**

After the `project_id` block (~line 486):

```go
	// Detached turns keep running after a client disconnect — surface the
	// in-flight session so the client can poll it to completion (§103
	// amendment). Absent when no non-stale running session exists.
	if h.Settings != nil {
		timeout := h.Settings.GetIntOr(r.Context(), "agent.timeout_seconds", defaultTimeoutSeconds)
		if sid, serr := h.Convos.RunningSessionID(r.Context(), convID, user.UserID, time.Now().Add(-time.Duration(timeout)*time.Second)); serr == nil && sid.Valid {
			detail["running_session_id"] = sid.String()
		}
	}
```

- [ ] **Step 4: Tests**

In `conversations_test.go`, stub-backed test:

```go
func TestConversationService_RunningSessionID(t *testing.T) {
	store := newStubConvoStore()
	svc := NewConversationService(store, nil)
	// no running row → invalid uuid, no error
	id, err := svc.RunningSessionID(context.Background(), convID, userID, time.Now().Add(-time.Minute))
	require.NoError(t, err)
	require.False(t, id.Valid)
	// seed running row → returns it (extend the stub's running map)
}
```

- [ ] **Step 5: Run tests, vet, commit**

```bash
go test ./internal/ai/ -run 'TestConversationService' -v
go vet ./internal/ai/
git add internal/db/queries/ai_conversations.sql internal/db/ internal/ai/conversations.go internal/ai/conversations_test.go internal/ai/unified_handlers.go
git commit -m "ai: running_session_id on conversation detail for detached-turn polling"
```

---

### Task 4: Template `intent` field + spec-text steering (frontend lib)

**Files:**
- Modify: `apps/main/src/lib/ai/chatTemplate.ts` — `ChatTemplateValues` (~18), `DEFAULT_CHAT_TEMPLATE_VALUES`, `normalizeTemplateValues` (~145), `buildTemplateBlock` (~274), `buildTemplateInsertText` (~314)
- Test: `apps/main/src/lib/ai/chatTemplate.test.ts`

**Interfaces:**
- Produces: `ChatTemplateValues.intent: 'preview' | 'create'`; `buildTemplateBlock(name, values)` and `buildTemplateInsertText(values, labels)` — signatures unchanged, both read `values.intent`.

- [ ] **Step 1: Write the failing tests**

```ts
describe('intent', () => {
	it('normalizes intent to preview|create with preview default', () => {
		expect(normalizeTemplateValues({}).intent).toBe('preview');
		expect(normalizeTemplateValues({ intent: 'create' }).intent).toBe('create');
		expect(normalizeTemplateValues({ intent: 'bogus' }).intent).toBe('preview');
	});

	it('preview intent says the drafts stay in chat', () => {
		const text = buildTemplateInsertText(base());
		expect(text).toContain('Present the draft(s) in chat');
		expect(text).not.toContain('content.create_draft');
	});

	it('create intent instructs persistence per topic', () => {
		const text = buildTemplateInsertText({ ...base(), intent: 'create' });
		expect(text).toContain('content.create_draft');
		expect(text).toContain('content.bulk_create');
	});

	it('attach block keeps generate_draft for preview, create tools for create', () => {
		expect(buildTemplateBlock('T', base())).toContain('content.generate_draft');
		const b = buildTemplateBlock('T', { ...base(), intent: 'create' });
		expect(b).toContain('content.create_draft');
		expect(b).not.toContain('generate_draft');
	});

	it('intent does not leak into the generate_draft spec JSON', () => {
		expect(buildTemplateSpec({ ...base(), intent: 'create' })).not.toHaveProperty('intent');
	});
});
```

- [ ] **Step 2: Run to verify fail**

Run: `cd apps/main && bunx vitest run src/lib/ai/chatTemplate.test.ts`
Expected: FAIL — `intent` not on `ChatTemplateValues`.

- [ ] **Step 3: Implement**

In `ChatTemplateValues` add (near the top of the interface):

```ts
	/** Send intent — 'preview' renders drafts in chat (generate_draft),
	 * 'create' steers the agent to persist items (create_draft/bulk_create). */
	intent: 'preview' | 'create';
```

In `DEFAULT_CHAT_TEMPLATE_VALUES` add `intent: 'preview'`.

In `normalizeTemplateValues` return object add:

```ts
		intent: v.intent === 'create' ? 'create' : 'preview',
```

In `buildTemplateBlock`, branch the instruction:

```ts
	const persist = values.intent === 'create';
	return (
		`\n\n---\n` +
		`[Chat template: "${label}"] Apply this generation spec to my request. ` +
		(persist
			? `Persist the result as real content — call content.create_draft once per topic ` +
				`(or content.bulk_create when my request lists several topics), filling ` +
				`title/body from my request and these constraints:\n\n`
			: `If the content.generate_draft tool is available, call it with these arguments, ` +
				`filling "title" and "instructions" from my request above — when my request ` +
				`lists multiple topics, make one call per topic; otherwise draft the ` +
				`content directly, honoring the same constraints.\n\n`) +
		'```json\n' +
		JSON.stringify(spec, null, 2) +
		'\n```'
	);
```

In `buildTemplateInsertText`, replace the header line:

```ts
	const lines: string[] = [
		values.intent === 'create'
			? `Create one ${values.content_type || 'guide'} per topic in the Topics list — persist each as a real content item (content.create_draft for a single topic, content.bulk_create for several), with these requirements:`
			: `Write one ${values.content_type || 'guide'} draft per topic in the Topics list, with these requirements:`
	];
```

And extend the preview trailing guidance — right before the Topics marker is pushed, for `preview` intent add a line so the outcome is explicit:

```ts
	if (values.intent !== 'create') {
		lines.push('', 'Present the draft(s) in chat — do not create content items.');
	}
```

Check `SPEC_HEADER_RE` still matches both headers — update it to cover `Create one`:

```ts
const SPEC_HEADER_RE = /^(?:Write (?:a|one)|Create one)\b[^\n]*(?:draft|requirements)[^\n]*:/;
```

- [ ] **Step 4: Run tests**

Run: `bunx vitest run src/lib/ai/chatTemplate.test.ts`
Expected: PASS (all existing + new).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/ai/chatTemplate.ts apps/main/src/lib/ai/chatTemplate.test.ts
git commit -m "ai: template intent field steering preview vs persisted create"
```

---

### Task 5: Editor intent toggle + i18n (frontend UI)

**Files:**
- Modify: `apps/main/src/lib/ai/ChatTemplatePanel.svelte` — save footer next to the Folder field (~line 1248)
- Modify: `apps/main/src/lib/i18n/en.json`, `apps/main/src/lib/i18n/fr.json` — `ai.templates.intent*` keys

**Interfaces:**
- Consumes: `values.intent` (bindable `values` prop already carries it — mutating `values.intent` propagates to the parent's `tplValues`).

- [ ] **Step 1: i18n keys**

`en.json` (in the `ai.templates` group, near `folder`):

```json
		"intent": "Intent",
		"intent_preview": "Preview in chat",
		"intent_create": "Create content",
		"intent_hint": "Preview drafts text in the conversation; Create persists real items via the agent.",
```

`fr.json` mirrors:

```json
		"intent": "Intention",
		"intent_preview": "Aperçu dans le chat",
		"intent_create": "Créer le contenu",
		"intent_hint": "Aperçu rédige le texte dans la conversation ; Créer persiste de vrais éléments via l'agent.",
```

- [ ] **Step 2: Segmented control in the save footer**

In `ChatTemplatePanel.svelte`, inside the save footer directly above the Folder `tpl-field` div (~line 1248):

```svelte
		<div class="tpl-field">
			<span class="tpl-label">{t('ai.templates.intent', 'Intent')}</span>
			<div
				class="flex overflow-hidden rounded-[var(--radius)] border border-[var(--color-border)]"
				role="radiogroup"
				aria-label={t('ai.templates.intent', 'Intent')}
			>
				{#each [
					['preview', t('ai.templates.intent_preview', 'Preview in chat')],
					['create', t('ai.templates.intent_create', 'Create content')]
				] as [val, label] (val)}
					<button
						type="button"
						role="radio"
						aria-checked={values.intent === val}
						onclick={() => (values.intent = val as 'preview' | 'create')}
						class="flex-1 px-2 py-1.5 text-xs {values.intent === val
							? 'bg-[var(--color-primary)] text-white font-medium'
							: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
					>
						{label}
					</button>
				{/each}
			</div>
		</div>
```

(`values` is the bound `ChatTemplateValues` prop — mutating `values.intent` flows to the parent's `tplValues` like the other editor fields.)

- [ ] **Step 3: Check + commit**

```bash
cd apps/main && bunx svelte-check --threshold warning
git add apps/main/src/lib/ai/ChatTemplatePanel.svelte apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "ai: template intent toggle (preview in chat vs create content)"
```

---

### Task 6: Return-UX polling + background indicator (frontend)

**Files:**
- Modify: `apps/main/src/lib/ai/JapaAIChat.svelte` — `loadConversation` (~825), `send()` stream tail (~999-1035), new poll state near `activeSessionId` (~466), indicator in the transcript area

**Interfaces:**
- Consumes: `GET /ai/agent/sessions/{id}` → `{ data: { session: { status } } }` (terminal ≠ `'running'`); `GET /ai/conversations/{id}` → `running_session_id` (Task 3).
- Produces: `bgSessionId` state + `startSessionPoll(id)` / `stopSessionPoll()`.

- [ ] **Step 1: Poll state + functions**

Near `activeSessionId` (~line 466):

```ts
	// Detached turns keep running server-side after a disconnect — when a
	// conversation carries a running session (or a live stream drops), poll
	// the session endpoint until terminal, then refetch so the persisted
	// assistant message (or failure notice) lands.
	let bgSessionId: string | null = $state(null);
	let bgPollTimer: ReturnType<typeof setInterval> | null = null;
	const BG_POLL_MS = 3000;
	const BG_POLL_CAP_MS = 10 * 60 * 1000;

	function stopSessionPoll() {
		if (bgPollTimer) clearInterval(bgPollTimer);
		bgPollTimer = null;
		bgSessionId = null;
	}

	function startSessionPoll(sessionId: string) {
		if (!sessionId || bgSessionId === sessionId) return;
		stopSessionPoll();
		bgSessionId = sessionId;
		const startedAt = Date.now();
		let failures = 0;
		bgPollTimer = setInterval(async () => {
			if (!bgSessionId) return;
			if (Date.now() - startedAt > BG_POLL_CAP_MS) {
				stopSessionPoll();
				return;
			}
			const res = await api.get<{ session: { status: string } }>(
				`/ai/agent/sessions/${bgSessionId}`
			);
			const status = res.data?.session?.status;
			if (res.error) {
				// api error shape is { code, message, status } — a 404 means
				// the session row is gone; anything else gets one retry.
				if (++failures >= 2 || res.error.code === 'session_not_found') stopSessionPoll();
				return;
			}
			failures = 0;
			if (status && status !== 'running') {
				const convId = conversationId;
				stopSessionPoll();
				if (convId) await loadConversation(convId);
			}
		}, BG_POLL_MS);
	}
```

- [ ] **Step 2: Conversation load wiring**

In `loadConversation`, extend the response type with `running_session_id?: string` and, at the end of the success path (after `sidebarOpen = false`):

```ts
		// A detached turn may still be running server-side for this thread —
		// poll it and land the persisted result when it finishes.
		stopSessionPoll();
		if (res.data.running_session_id && !sending) {
			startSessionPoll(res.data.running_session_id);
		}
```

- [ ] **Step 3: Mid-stream drop → poll**

In `send()`, capture the tools state at send time (the composer toggle could flip while the stream is live — decide detach-vs-discard by what was SENT, not what's set now): right after `const message = input.trim()` add

```ts
		const sentWithTools = toolsOn;
```

Then the post-loop `if (liveTurn.in_progress)` block (~line 999): when the stream ends with no terminal event but a session exists and the turn was sent with tools, switch to background polling instead of "Stream closed.":

```ts
		if (liveTurn.in_progress) {
			const sid = liveTurn.session_id;
			if (sid && sentWithTools) {
				// Detached turn — the run continues server-side; poll the
				// session and land the persisted message on completion.
				liveTurn.progress = t('ai.agent.bg_running', 'Working in the background…');
				startSessionPoll(sid);
			} else {
				liveTurn.in_progress = false;
				if (!liveTurn.content && !liveTurn.error) {
					liveTurn.content = t('ai.agent.stream_closed', 'Stream closed.');
				}
			}
			turns = [...turns];
		}
```

Keep `in_progress: true` while polling so the turn's spinner/progress area shows the background state; `loadConversation` replaces `turns` on completion.

Also: on `AbortError` (user pressed Stop → the cancel endpoint was already POSTed) do NOT poll — the catch branch already handles it; ensure `startSessionPoll` isn't called there. Verify the stop/cancel handler (~line 1175) calls `stopSessionPoll()` — add it inside.

Cleanup: `import { onDestroy }` — the file's svelte import is currently `{ onMount, tick, untrack }` (~line 18) — add `onDestroy` and register `onDestroy(stopSessionPoll)`.

Also call `stopSessionPoll()` at the top of `startNewChat()` and as the first line of `loadConversation` (stopping early avoids a stale poll firing mid-load; the success-path call re-arms it when the new conversation reports a running session).

- [ ] **Step 4: Indicator in the transcript**

Where the turn spinner renders (`in_progress`/`progress` — the loading indicator block), extend it so `bgSessionId` shows the same treatment, OR simplest: under the transcript list add

```svelte
	{#if bgSessionId}
		<div class="flex items-center gap-2 text-xs text-[var(--color-muted)]">
			<Loader2 size={12} class="animate-spin" />
			{t('ai.agent.bg_running', 'Agent is working in the background — the result will appear here.')}
		</div>
	{/if}
```

(The live-turn case already shows progress on the turn; this covers the return-to-conversation case where no liveTurn exists.)

- [ ] **Step 5: i18n**

`en.json`: `"bg_running": "Agent is working in the background — the result will appear here."`
`fr.json`: `"bg_running": "L'agent travaille en arrière-plan — le résultat apparaîtra ici."`

(both under the existing `ai.agent` group.)

- [ ] **Step 6: Check + commit**

```bash
cd apps/main && bunx svelte-check --threshold warning
git add apps/main/src/lib/ai/JapaAIChat.svelte apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "ai: poll running agent sessions and surface background turns in the transcript"
```

---

### Task 7: End-to-end verification

**Files:** none (verification only)

- [ ] **Step 1: Full check suite**

```bash
cd /home/jmobile/websites/japa/japaconnect
go test ./internal/ai/ && go vet ./internal/ai/
cd apps/main && bunx vitest run src/lib/ai/ && bunx svelte-check --threshold warning
```

- [ ] **Step 2: Rebuild + restart the API**

```bash
cd /home/jmobile/websites/japa/japaconnect && go build -o bin/japa-api ./cmd/... # same target as the running binary
# restart the running ./bin/japa-api process
```

- [ ] **Step 3: Browser verify (agent-browser)**

- `/ai` tools-on send → the turn streams as before.
- Simulate disconnect mid-run (close the EventSource / navigate) → reload → conversation shows the background indicator → result lands without manual refresh.
- Template editor shows the intent toggle; `create`-intent insert text names `content.create_draft`/`bulk_create`.
- Console stays clean (Vite HMR noise aside).

- [ ] **Step 4: Final commit if any fixes**

---

## Self-review notes

- **Spec coverage:** detach (T2), failure notice (T2), `running_session_id` + query (T3), poll UX (T6), template intent (T4/T5), JEOS-first (T1), Asynq deferred phase (spec'd only — JEOS note in T1, no code).
- **Type consistency:** `intent: 'preview'|'create'` everywhere; `RunningSessionID` returns `pgtype.UUID`; `running_session_id` emitted only when valid; `startSessionPoll(sessionId: string)`.
- **Known follow-ups (not bugs):** Asynq `ai:agent_run` (restart-survival) and Valkey-buffered SSE resume are documented as the next phase in §103/§11.
