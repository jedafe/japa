# Japa AI Body Assistant Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` (recommended) or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a permission-gated "japa AI" drafting assistant to every create/edit body field in `apps/main`, backed by a new `POST /api/v1/ai/assist/draft` endpoint.

**Architecture:** The backend adds a single handler reusing the existing `ai` package dispatcher and humanization pipeline. The frontend introduces reusable `AiBodyButton` and `AiDraftPanel` components; they are wired into the main composer (`CreateShell`/`PostEditor`) and into each structured content create/edit form.

**Tech Stack:** Go 1.24, chi, SvelteKit 2, Svelte 5 Runes, Bits UI, Tailwind CSS v4, TipTap, `pgtype.UUID`, `bun`.

## Global Constraints

- RBAC gating: `ai.editor.assist` is required for both the UI and the API.
- Honor the global `ai.humanize.*` tenant settings and allow per-request overrides.
- Svelte 5 Runes only — no `export let`, no `$:`, no `on:click`, no `createEventDispatcher`.
- Tailwind v4 + CSS custom properties (`var(--color-*)`) for styling; dark mode controlled by the `jc_theme` cookie (`[data-theme="dark"]`).
- No hardcoded data; use `data.user?.permissions` and feature flags from the backend.
- All UI text goes through `t()` and is added to `apps/main/src/lib/i18n/en.json`.
- Backend errors use the existing `errpkg.WriteError` shape.

---

## Task 1: Backend — `HandleDraft` handler skeleton and request parsing

**Files:**
- Modify: `internal/ai/handlers.go` (add `HandleDraft`)
- Create: `internal/ai/draft_test.go`

**Interfaces:**
- Consumes: `Handlers.dispatchAndRecordUsage(ctx, ID, userID, "assist_draft", "content_generation", req)`.
- Produces: `func (h *Handlers) HandleDraft(w http.ResponseWriter, r *http.Request)`.

- [ ] **Step 1: Add the `HandleDraft` method stub to `internal/ai/handlers.go`**

Add below `HandleGenerateOG` (around line 1077):

```go
// HandleDraft handles POST /api/v1/ai/assist/draft — generate or improve a body
// and optionally fill custom fields.
func (h *Handlers) HandleDraft(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteError(w, http.StatusBadRequest, "not_implemented", "Draft handler not implemented")
}
```

- [ ] **Step 2: Write the failing test in `internal/ai/draft_test.go`**

```go
package ai

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleDraft_MissingPermission(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader([]byte(`{"content_type":"blog_post","title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDraft_ValidBlogPost(t *testing.T) {
	t.Parallel()
	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", &mockProvider{})
	h := &Handlers{Dispatcher: disp}
	user, tenant := testUserAndTenant()
	body, _ := json.Marshal(map[string]any{
		"content_type": "blog_post",
		"title":        "My journey",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
```

- [ ] **Step 3: Run the tests to confirm they fail**

```bash
cd /home/jmobile/websites/japa/japaconnect
CGO_ENABLED=0 go test ./internal/ai -run TestHandleDraft -v
```

Expected: `TestHandleDraft_MissingPermission` passes (401). `TestHandleDraft_ValidBlogPost` fails with status 400 (not 200).

- [ ] **Step 4: Add `FieldSchema`, `FieldOption`, `DraftInput` types and request parsing in `HandleDraft`**

Add to `internal/ai/handlers.go`:

```go
type FieldSchema struct {
	Key     string        `json:"key"`
	Type    string        `json:"type"`
	Label   string        `json:"label"`
	Options []FieldOption `json:"options,omitempty"`
}

type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type DraftInput struct {
	ContentType   string         `json:"content_type"`
	Title         string         `json:"title"`
	ExistingBody  string         `json:"existing_body"`
	Instructions  string         `json:"instructions"`
	Tone          string         `json:"tone"`
	Humanize      *bool          `json:"humanize,omitempty"`
	HumanizeLevel string         `json:"humanize_level,omitempty"`
	FieldSchema   []FieldSchema  `json:"field_schema,omitempty"`
	Context       map[string]any `json:"context,omitempty"`
}
```

Then implement parsing:

```go
func (h *Handlers) HandleDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok { return }

	ID, ok := requireTenant(w, r)
	if !ok { return }

	var input DraftInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.ContentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "content_type is required")
		return
	}

	errpkg.WriteError(w, http.StatusBadRequest, "not_implemented", "Draft generation not yet implemented")
}
```

- [ ] **Step 5: Commit**

```bash
git add internal/ai/handlers.go internal/ai/draft_test.go
git commit -m "feat(ai): add HandleDraft skeleton and parsing tests

Adds POST /api/v1/ai/assist/draft handler stub, request parsing,
and missing-auth/permission tests."
```

---

## Task 2: Backend — prompt building, dispatch, and response mapping

**Files:**
- Modify: `internal/ai/handlers.go`
- Modify: `internal/ai/handlers.go` — add `DraftResult`, `buildDraftPrompt`, `parseDraftResponse`
- Create: `internal/ai/draft_test.go` (extend tests)

**Interfaces:**
- Consumes: `resolveHandlerHumanize`, `writeHumanizePayload`, `dispatchAndRecordUsage`.
- Produces: JSON response `{ data: { title, body_html, field_values, tokens_used } }`.

- [ ] **Step 1: Add the `DraftResult` type**

In `internal/ai/handlers.go`:

```go
type DraftResult struct {
	Title       string         `json:"title"`
	BodyHTML    string         `json:"body_html"`
	FieldValues map[string]any `json:"field_values"`
	TokensUsed  int            `json:"tokens_used"`
}
```

- [ ] **Step 2: Write the failing integration test**

Append to `internal/ai/draft_test.go`:

```go
func TestHandleDraft_ReturnsBodyAndTitle(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		name: "mock",
		sendFn: func(_ context.Context, _ Request) (Response, error) {
			return Response{
				ProviderRef: "mock-ref",
				Status:      "success",
				Raw: map[string]any{
					"model": "mock-model",
					"usage": map[string]any{"total_tokens": 42.0},
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": `{"title":"Final Title","body_html":"<p>Hello</p>","field_values":{}}`,
							},
						},
					},
				},
			}, nil
		},
	}
	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", provider)
	h := &Handlers{Dispatcher: disp}

	user, tenant := testUserAndTenant()
	body, _ := json.Marshal(map[string]any{
		"content_type": "blog_post",
		"title":        "Draft Title",
		"tone":         "friendly",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data DraftResult `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Data.Title != "Final Title" {
		t.Errorf("title = %q, want %q", resp.Data.Title, "Final Title")
	}
	if resp.Data.BodyHTML != "<p>Hello</p>" {
		t.Errorf("body_html = %q, want %q", resp.Data.BodyHTML, "<p>Hello</p>")
	}
	if resp.Data.TokensUsed != 42 {
		t.Errorf("tokens_used = %d, want 42", resp.Data.TokensUsed)
	}
}
```

- [ ] **Step 3: Run the test to confirm it fails**

```bash
CGO_ENABLED=0 go test ./internal/ai -run TestHandleDraft_ReturnsBodyAndTitle -v
```

Expected: FAIL (handler still returns 400 or empty response).

- [ ] **Step 4: Implement prompt, dispatch, and parse logic in `HandleDraft`**

```go
func (h *Handlers) HandleDraft(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok { return }
	ID, ok := requireTenant(w, r)
	if !ok { return }

	var input DraftInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.ContentType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "content_type is required")
		return
	}

	if input.Tone == "" { input.Tone = "professional" }

	prompt := buildDraftPrompt(input)

	payload := map[string]any{
		"prompt":       prompt,
		"user_id":      user.UserID.String(),
		"tenant_id":    ID.String(),
		"content_type": input.ContentType,
		"tone":         input.Tone,
		"task":         "draft",
	}

	hEnabled, hLevel := h.resolveHandlerHumanize(r.Context(), ID, input.Humanize, input.HumanizeLevel)
	writeHumanizePayload(payload, hEnabled, hLevel)

	resp, err := h.dispatchAndRecordUsage(r.Context(), ID, user.UserID, "assist_draft", "content_generation", Request{
		Category: "content_generation",
		Payload:  payload,
	})
	if err != nil {
		slog.Error("ai draft failed", "error", err)
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", "AI draft failed")
		return
	}

	result, err := parseDraftResponse(resp)
	if err != nil {
		slog.Error("ai draft parse failed", "error", err)
		errpkg.WriteError(w, http.StatusBadGateway, "ai_error", "Failed to parse AI draft")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}
```

Add helper functions:

```go
func buildDraftPrompt(input DraftInput) string {
	var b strings.Builder
	b.WriteString("You are a content editor for an immigration community platform.\n")
	b.WriteString("Content type: " + input.ContentType + "\n")
	if input.Title != "" {
		b.WriteString("Current title: " + input.Title + "\n")
	}
	if input.ExistingBody != "" {
		b.WriteString("Existing body: " + input.ExistingBody + "\n")
	}
	b.WriteString("Tone: " + input.Tone + "\n")
	if input.Instructions != "" {
		b.WriteString("Instructions: " + input.Instructions + "\n")
	}
	if len(input.FieldSchema) > 0 {
		b.WriteString("Also return values for these fields as JSON under field_values:\n")
		for _, f := range input.FieldSchema {
			b.WriteString("- " + f.Key + " (" + f.Type + "): " + f.Label + "\n")
		}
	}
	b.WriteString("Return a JSON object with keys: title (optional), body_html, field_values (optional). Do not include markdown code fences.\n")
	return b.String()
}

func parseDraftResponse(resp Response) (DraftResult, error) {
	result := DraftResult{Title: "", BodyHTML: "", FieldValues: map[string]any{}, TokensUsed: 0}

	raw := mapGenerateResult(resp)
	result.TokensUsed = raw["tokens_used"].(int)

	content := raw["generated_text"].(string)
	if content == "" { return result, nil }

	// Try to parse the generated text as a JSON object. If that fails,
	// treat the whole content as body_html.
	var parsed DraftResult
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		result.Title = parsed.Title
		result.BodyHTML = parsed.BodyHTML
		result.FieldValues = parsed.FieldValues
	} else {
		result.BodyHTML = content
	}

	if result.FieldValues == nil { result.FieldValues = map[string]any{} }
	return result, nil
}
```

- [ ] **Step 5: Run the tests**

```bash
CGO_ENABLED=0 go test ./internal/ai -run TestHandleDraft -v
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/ai/
git commit -m "feat(ai): implement /ai/assist/draft prompt and response parsing

Generates body HTML, optional title, and custom field values from
content context and tone/humanize overrides."
```

---

## Task 3: Backend — route wiring and handler tests

**Files:**
- Modify: `internal/server/server.go`
- Modify: `internal/ai/draft_test.go` (add missing-permission/validation tests)

- [ ] **Step 1: Register the route in `internal/server/server.go`**

Add after the `/ai/assist/generate-og` route (around line 4083):

```go
r.With(auth.RequireAuth(kr, vkClient.Redis()), rbac.RequirePermission(rbacResolver, "ai.editor.assist"), mw.RateLimit(vkClient.Redis(), "moderate")).
	Post("/ai/assist/draft", aiHandlers.HandleDraft)
```

- [ ] **Step 2: Add missing tests to `internal/ai/draft_test.go`**

```go
func TestHandleDraft_MissingContentType(t *testing.T) {
	t.Parallel()
	h := &Handlers{}
	user, tenant := testUserAndTenant()
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader([]byte(`{"title":"x"}`)))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDraft_HumanizeOverride(t *testing.T) {
	t.Parallel()
	var got map[string]any
	provider := &mockProvider{
		name: "mock",
		sendFn: func(_ context.Context, req Request) (Response, error) {
			got = req.Payload
			return Response{Raw: map[string]any{
				"choices": []any{map[string]any{"message": map[string]any{"content": `{"body_html":"<p>x</p>"}`}}},
			}}, nil
		},
	}
	disp := NewDispatcher()
	disp.RegisterProvider("content_generation", provider)
	h := &Handlers{Dispatcher: disp}
	user, tenant := testUserAndTenant()
	body, _ := json.Marshal(map[string]any{
		"content_type":   "blog_post",
		"title":          "t",
		"humanize":       true,
		"humanize_level": "heavy",
	})
	req := httptest.NewRequest(http.MethodPost, "/ai/assist/draft", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = withAuthContext(req, user, tenant)
	w := httptest.NewRecorder()

	h.HandleDraft(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if got["humanize"] != true || got["humanize_level"] != "heavy" {
		t.Errorf("humanize payload = %v", got)
	}
}
```

- [ ] **Step 3: Run the full backend AI test suite**

```bash
CGO_ENABLED=0 go test ./internal/ai/... -v -count=1
```

Expected: all existing + new tests pass.

- [ ] **Step 4: Build the backend**

```bash
cd /home/jmobile/websites/japa/japaconnect
go build ./...
```

Expected: clean build.

- [ ] **Step 5: Commit**

```bash
git add internal/server/server.go internal/ai/draft_test.go internal/ai/handlers.go
git commit -m "feat(ai): wire POST /api/v1/ai/assist/draft route and tests

Routes the new draft endpoint under ai.editor.assist with moderate
rate limiting and adds validation/humanize tests."
```

---

## Task 4: Frontend — types and API client for `/ai/assist/draft`

**Files:**
- Create: `apps/main/src/lib/ai/types.ts`
- Create: `apps/main/src/lib/ai/draftApi.ts`

**Interfaces:**
- Produces: `draftContent(request: DraftRequest): Promise<ClientResult<DraftResponse>>`.

- [ ] **Step 1: Add shared types in `apps/main/src/lib/ai/types.ts`**

```ts
export interface FieldSchema {
	key: string;
	type: string;
	label: string;
	options?: { value: string; label: string }[];
}

export interface DraftRequest {
	content_type: string;
	title?: string;
	existing_body?: string;
	instructions?: string;
	tone?: string;
	humanize?: boolean;
	humanize_level?: string;
	field_schema?: FieldSchema[];
	context?: Record<string, unknown>;
}

export interface DraftResponse {
	title?: string;
	body_html: string;
	field_values?: Record<string, unknown>;
	tokens_used?: number;
}
```

- [ ] **Step 2: Add the API client in `apps/main/src/lib/ai/draftApi.ts`**

```ts
import { api } from '$lib/api/client';
import type { ClientResult } from '$lib/api/client';
import type { DraftRequest, DraftResponse } from './types';

export async function draftContent(request: DraftRequest): Promise<ClientResult<DraftResponse>> {
	return api.post<DraftResponse>('/ai/assist/draft', request);
}
```

- [ ] **Step 3: Verify the module compiles**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun x svelte-check --tsconfig ./tsconfig.json --output human-verbose
```

Expected: no errors from the new files (ignore pre-existing).

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/ai/types.ts apps/main/src/lib/ai/draftApi.ts
git commit -m "feat(main): add /ai/assist/draft frontend types and API client

Adds DraftRequest/DraftResponse types and a thin post helper."
```

---

## Task 5: Frontend — `AiBodyButton` component

**Files:**
- Create: `apps/main/src/lib/ai/AiBodyButton.svelte`

**Interfaces:**
- Consumes: `data.user?.permissions` (assumes `PageData.user` from root layout), `inline_ai_assistant` feature flag.
- Produces: `onclick` event to open the drafting panel.

- [ ] **Step 1: Create `AiBodyButton.svelte`**

```svelte
<script lang="ts">
	import { Sparkles } from 'lucide-svelte';
	import { t } from '$lib/i18n';

	interface Props {
		disabled?: boolean;
		onclick?: () => void;
		class?: string;
	}

	let { disabled = false, onclick, class: klass = '' }: Props = $props();
</script>

<button
	type="button"
	{disabled}
	onclick={() => onclick?.()}
	class="inline-flex items-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1 text-xs font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50 {klass}"
>
	<Sparkles size={12} class="text-[var(--color-primary)]" />
	{t('ai.body.button_label', 'japa AI')}
</button>
```

- [ ] **Step 2: Add gating helper in `apps/main/src/lib/ai/canUseAI.ts`**

```ts
import type { User } from '$lib/types';

export function canUseAI(user: User | null | undefined, features: Record<string, boolean> = {}): boolean {
	if (features.inline_ai_assistant === false) return false;
	return user?.permissions?.includes('ai.editor.assist') ?? false;
}
```

- [ ] **Step 3: Add i18n keys to `apps/main/src/lib/i18n/en.json`**

Add under the existing `ai.agent` keys:

```json
	"ai.body.button_label": "japa AI",
	"ai.body.panel_title": "Japa AI Assistant",
	"ai.body.placeholder": "Tell the AI what to do...",
	"ai.body.tone_label": "Tone",
	"ai.body.humanize_label": "Humanize",
	"ai.body.apply": "Apply",
	"ai.body.cancel": "Cancel",
	"ai.body.regenerate": "Regenerate",
	"ai.body.generating": "Drafting...",
	"ai.body.error": "Failed to generate draft. Please try again.",
	"ai.body.unavailable": "AI assistant is not available.",
```

- [ ] **Step 4: Verify build**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run check
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/ai/AiBodyButton.svelte apps/main/src/lib/ai/canUseAI.ts apps/main/src/lib/i18n/en.json
git commit -m "feat(main): add AiBodyButton and AI permission helper

Adds the japa AI trigger button, permission gating, and i18n keys."
```

---

## Task 6: Frontend — `AiDraftPanel` component

**Files:**
- Create: `apps/main/src/lib/ai/AiDraftPanel.svelte`

**Interfaces:**
- Consumes: `draftContent` from `draftApi.ts`, `FieldSchema` / `DraftRequest` / `DraftResponse` from `types.ts`, `t` from `$lib/i18n`.
- Produces: `onApply: (payload: { body: string; title?: string; fieldValues?: Record<string, unknown> }) => void`.

- [ ] **Step 1: Create `AiDraftPanel.svelte`**

```svelte
<script lang="ts">
	import { Sparkles, X, Loader2, RefreshCw, Check } from 'lucide-svelte';
	import { t } from '$lib/i18n';
	import { draftContent } from './draftApi';
	import type { DraftRequest, DraftResponse, FieldSchema } from './types';

	interface Props {
		open: boolean;
		contentType: string;
		title: string;
		body: string;
		context?: Record<string, unknown>;
		fieldSchema?: FieldSchema[];
		onApply: (payload: { body: string; title?: string; fieldValues?: Record<string, unknown> }) => void;
		onClose: () => void;
	}

	let { open, contentType, title, body, context, fieldSchema, onApply, onClose }: Props = $props();

	let prompt = $state('');
	let tone = $state('professional');
	let humanize = $state(false);
	let humanizeLevel = $state('medium');
	let draft = $state<DraftResponse | null>(null);
	let loading = $state(false);
	let error = $state('');

	const TONES = [
		{ value: 'professional', label: t('ai.body.tone_professional', 'Professional') },
		{ value: 'friendly', label: t('ai.body.tone_friendly', 'Friendly') },
		{ value: 'concise', label: t('ai.body.tone_concise', 'Concise') },
		{ value: 'persuasive', label: t('ai.body.tone_persuasive', 'Persuasive') },
		{ value: 'academic', label: t('ai.body.tone_academic', 'Academic') },
	];

	const HUMANIZE_LEVELS = [
		{ value: 'off', label: t('ai.body.humanize_off', 'Off') },
		{ value: 'light', label: t('ai.body.humanize_light', 'Light') },
		{ value: 'medium', label: t('ai.body.humanize_medium', 'Medium') },
		{ value: 'heavy', label: t('ai.body.humanize_heavy', 'Heavy') },
	];

	async function generate() {
		loading = true;
		error = '';
		try {
			const req: DraftRequest = {
				content_type: contentType,
				title,
				existing_body: body,
				instructions: prompt,
				tone,
				humanize,
				humanize_level: humanizeLevel,
				field_schema: fieldSchema,
				context,
			};
			const res = await draftContent(req);
			if (res.error) {
				error = res.error.message;
			} else if (res.data) {
				draft = res.data;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : t('ai.body.error');
		} finally {
			loading = false;
		}
	}

	function apply() {
		if (!draft) return;
		onApply({
			body: draft.body_html,
			title: draft.title,
			fieldValues: draft.field_values,
		});
		onClose();
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape') {
			onClose();
		} else if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			generate();
		}
	}
</script>

{#if open}
	<div
		class="fixed inset-0 z-40 bg-black/20"
		onclick={onClose}
		role="presentation"
	></div>
	<div
		class="fixed right-0 top-0 z-50 h-full w-full max-w-md border-l border-[var(--color-border)] bg-[var(--color-surface)] p-4 shadow-xl"
		role="dialog"
		aria-label={t('ai.body.panel_title')}
		onkeydown={onKeydown}
	>
		<div class="flex h-full flex-col gap-3">
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-semibold text-[var(--color-text)]">{t('ai.body.panel_title')}</h2>
				<button type="button" onclick={onClose} aria-label={t('common.close', 'Close')}>
					<X size={18} />
				</button>
			</div>

			<textarea
				bind:value={prompt}
				rows={3}
				placeholder={t('ai.body.placeholder')}
				class="w-full resize-none rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-2 text-sm text-[var(--color-text)] outline-none focus:border-[var(--color-primary)]"
			></textarea>

			<div class="grid grid-cols-2 gap-2">
				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]">{t('ai.body.tone_label')}</label>
					<select bind:value={tone} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20">
						{#each TONES as t (t.value)}
							<option value={t.value}>{t.label}</option>
						{/each}
					</select>
				</div>
				<div class="flex flex-col gap-1">
					<label class="text-xs text-[var(--color-muted)]">{t('ai.body.humanize_label')}</label>
					<select bind:value={humanizeLevel} class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20">
						{#each HUMANIZE_LEVELS as l (l.value)}
							<option value={l.value}>{l.label}</option>
						{/each}
					</select>
				</div>
			</div>

			<button
				type="button"
				disabled={loading}
				onclick={generate}
				class="inline-flex w-full items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50"
			>
				{#if loading}<Loader2 size={14} class="animate-spin" />{/if}
				{t('ai.body.regenerate', 'Generate')}
			</button>

			{#if error}
				<p class="text-xs text-red-500">{error}</p>
			{/if}

			{#if draft}
				<div class="flex-1 overflow-y-auto rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-background)] p-3 text-sm">
					{@html draft.body_html}
				</div>

				<div class="flex gap-2">
					<button type="button" onclick={apply} class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-2 text-sm font-medium text-[var(--color-primary-contrast)] transition-colors hover:bg-[var(--color-primary-hover)] disabled:opacity-50">
						<Check size={14} /> {t('ai.body.apply')}
					</button>
					<button type="button" onclick={generate} class="inline-flex flex-1 items-center justify-center gap-1.5 rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-sm font-medium text-[var(--color-text)] transition-colors hover:bg-[var(--color-surface-subtle)] disabled:opacity-50">
						<RefreshCw size={14} /> {t('ai.body.regenerate')}
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
```

- [ ] **Step 2: Add additional i18n keys to `en.json`**

```json
	"ai.body.tone_professional": "Professional",
	"ai.body.tone_friendly": "Friendly",
	"ai.body.tone_concise": "Concise",
	"ai.body.tone_persuasive": "Persuasive",
	"ai.body.tone_academic": "Academic",
	"ai.body.humanize_off": "Off",
	"ai.body.humanize_light": "Light",
	"ai.body.humanize_medium": "Medium",
	"ai.body.humanize_heavy": "Heavy",
```

- [ ] **Step 3: Run Svelte check and typecheck**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run check
bun x svelte-check --tsconfig ./tsconfig.json
```

Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/ai/AiDraftPanel.svelte apps/main/src/lib/i18n/en.json
git commit -m "feat(main): add AiDraftPanel side drafting panel

Implements the panel with prompt, tone, humanization, preview,
apply/cancel/regenerate, and keyboard shortcuts."
```

---

## Task 7: Frontend — main composer integration

**Files:**
- Modify: `apps/main/src/lib/components/create/editors/PostEditor.svelte`
- Modify: `apps/main/src/lib/components/create/CreateShell.svelte`

**Interfaces:**
- Consumes: `AiBodyButton`, `AiDraftPanel`, `canUseAI`.
- Produces: `bodyHtml`, `title`, `fieldValues` mutations.

- [ ] **Step 1: Rework `PostEditor` to expose an AI trigger slot**

Add an `aiTrigger` snippet prop to `PostEditor`:

```ts
interface Props {
	html: string;
	onedit: () => void;
	onhtml?: (html: string) => void;
	onjson?: (json: Record<string, unknown>) => void;
	onfocus?: () => void;
	defaultMode?: EditorMode;
	aiTrigger?: import('svelte').Snippet;
}
```

Render inside the toolbar:

```svelte
{#if aiTrigger}
	<span class="tb-sep"></span>
	{@render aiTrigger()}
{/if}
```

- [ ] **Step 2: Wire the panel in `CreateShell.svelte`**

Import:

```ts
import AiBodyButton from '$lib/ai/AiBodyButton.svelte';
import AiDraftPanel from '$lib/ai/AiDraftPanel.svelte';
import { canUseAI } from '$lib/ai/canUseAI';
```

Add state:

```ts
let aiPanelOpen = $state(false);
```

Pass the AI trigger to `PostEditor`:

```svelte
<PostEditor
	html={bodyHtml}
	onedit={markDirty}
	onhtml={(v) => { bodyHtml = v; }}
	onfocus={() => (focusedField = 'body')}
	defaultMode={defaultEditor}
	aiTrigger={aiButtonSnippet}
/>
```

Define the snippet:

```svelte
{#snippet aiButtonSnippet()}
	{#if canUseAI(data.user)}
		<AiBodyButton onclick={() => (aiPanelOpen = true)} />
	{/if}
{/snippet}
```

Add the panel:

```svelte
<AiDraftPanel
	open={aiPanelOpen}
	contentType={selectedType.value}
	{title}
	body={bodyHtml}
	fieldSchema={allFieldGroups.flatMap((g) => g.fields).map((f) => ({
		key: f.key,
		type: f.field_type,
		label: f.label,
		options: Array.isArray(f.options)
			? f.options.map((o) => (typeof o === 'string' ? { value: o, label: o } : o))
			: undefined,
	}))}
	onApply={(payload) => {
		if (payload.title) {
			title = payload.title;
			handleTitle(payload.title);
		}
		bodyHtml = payload.body;
		if (payload.fieldValues) {
			fieldValues = { ...fieldValues, ...payload.fieldValues };
		}
		markDirty();
	}}
	onClose={() => (aiPanelOpen = false)}
/>
```

- [ ] **Step 3: Verify the main composer still builds**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run check
```

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/create/editors/PostEditor.svelte apps/main/src/lib/components/create/CreateShell.svelte
git commit -m "feat(main): wire japa AI into main composer

Adds the AI trigger to PostEditor and the drafting panel to
CreateShell, applying generated body/title/field values."
```

---

## Task 8: Frontend — structured content forms integration

**Files:**
- Modify each structured new/edit page:
  - `apps/main/src/routes/(app)/stories/new/+page.svelte`
  - `apps/main/src/routes/(app)/stories/[slug]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/scam-alerts/new/+page.svelte`
  - `apps/main/src/routes/(app)/scam-alerts/[id]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/achievements/new/+page.svelte`
  - `apps/main/src/routes/(app)/achievements/[slug]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/planning-groups/new/+page.svelte`
  - `apps/main/src/routes/(app)/planning-groups/[id]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/calendar/new/+page.svelte`
  - `apps/main/src/routes/(app)/calendar/[id]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/jobs/create/+page.svelte`
  - `apps/main/src/routes/(app)/jobs/[slug]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/mutual-aid/new/+page.svelte`
  - `apps/main/src/routes/(app)/mutual-aid/[id]/edit/+page.svelte`
  - `apps/main/src/routes/(app)/ama/new/+page.svelte`
  - `apps/main/src/routes/(app)/services/new/+page.svelte`
  - `apps/main/src/routes/(app)/services/[slug]/edit/+page.svelte`

**Interfaces:**
- Consumes: `AiBodyButton`, `AiDraftPanel`, `canUseAI`.
- Produces: local `title`, `body` updates.

- [ ] **Step 1: Create a reusable `AiBodyField.svelte` wrapper for plain forms**

Create `apps/main/src/lib/ai/AiBodyField.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import AiBodyButton from './AiBodyButton.svelte';
	import AiDraftPanel from './AiDraftPanel.svelte';
	import { canUseAI } from './canUseAI';
	import type { FieldSchema } from './types';

	interface Props {
		id: string;
		label: string;
		contentType: string;
		title: string;
		body: string;
		onBody: (v: string) => void;
		onTitle?: (v: string) => void;
		placeholder?: string;
		rows?: number;
		context?: Record<string, unknown>;
		fieldSchema?: FieldSchema[];
		disabled?: boolean;
		user?: { permissions?: string[] } | null;
	}

	let { id, label, contentType, title, body, onBody, onTitle, placeholder, rows = 8, context, fieldSchema, disabled = false, user }: Props = $props();
	let open = $state(false);

	const inputClass = 'w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm text-[var(--color-text)] outline-none transition focus:border-[var(--color-primary)] focus:ring-2 focus:ring-[var(--color-primary)]/20';
</script>

<div class="flex flex-col gap-1.5">
	<div class="flex items-center justify-between">
		<label for={id} class="text-sm font-medium text-[var(--color-text)]">{label}</label>
		{#if canUseAI(user)}
			<AiBodyButton onclick={() => (open = true)} {disabled} />
		{/if}
	</div>
	<textarea {id} {rows} value={body} oninput={(e) => onBody(e.currentTarget.value)} {placeholder} class={inputClass} {disabled}></textarea>
</div>

<AiDraftPanel
	{open}
	{contentType}
	{title}
	{body}
	{context}
	{fieldSchema}
	onApply={(payload) => {
		onBody(payload.body);
		if (payload.title && onTitle) onTitle(payload.title);
	}}
	onClose={() => (open = false)}
/>
```

- [ ] **Step 2: Add the wrapper to `/stories/new/+page.svelte` as the template**

Replace the Body section:

```svelte
<AiBodyField
	id="f-body"
	label={t('stories.field_body', 'Your Story')}
	contentType="story"
	{title}
	body={body}
	onBody={(v) => (body = v)}
	user={data.user}
	placeholder={t('stories.field_body_ph', 'Tell us about your relocation experience…')}
	rows={8}
/>
```

Add import:

```ts
import AiBodyField from '$lib/ai/AiBodyField.svelte';
```

- [ ] **Step 3: Repeat for each remaining structured new/edit page**

Use the same pattern. For each page:
1. Import `AiBodyField`.
2. Locate the body `<textarea>` and its `<label>`.
3. Replace the field group with `<AiBodyField ... />` using the local `title` and `body` variables.
4. Use the appropriate `contentType` (`story`, `scam_alert`, `achievement`, `planning_group`, `calendar`, `job`, `mutual_aid`, `ama`, `service`).
5. For edit pages, pass the existing `title` and `body`.

- [ ] **Step 4: Run the full Svelte check**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run check
```

Expected: no new errors.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/ai/AiBodyField.svelte apps/main/src/routes/(app)/
git commit -m "feat(main): add japa AI to structured content forms

Wraps body textareas with AiBodyField across stories, scam-alerts,
achievements, planning-groups, calendar, jobs, mutual-aid, ama,
and services create/edit pages."
```

---

## Task 9: Frontend — tool forms integration

**Files:**
- Modify relevant tool pages:
  - `apps/main/src/routes/(app)/tools/embassy-alerter/+page.svelte`
  - `apps/main/src/routes/(app)/tools/processing-tracker/+page.svelte`
  - `apps/main/src/routes/(app)/tools/credential-checker/+page.svelte`
  - `apps/main/src/routes/(app)/tools/cv-formatter/+page.svelte`
  - `apps/main/src/routes/(app)/tools/community-locator/+page.svelte`
  - `apps/main/src/routes/(app)/tools/attestation-guide/+page.svelte`
  - `apps/main/src/routes/(app)/tools/visa-routes/+page.svelte`
  - `apps/main/src/routes/(app)/tools/housing-guide/+page.svelte`
  - `apps/main/src/routes/(app)/tools/tax-calculator/+page.svelte`
  - `apps/main/src/routes/(app)/tools/language-test/+page.svelte`
  - `apps/main/src/routes/(app)/tools/cost-of-living/+page.svelte`

- [ ] **Step 1: For each tool form, wrap the body/description field**

Use `AiBodyField` only for pages that are editor-style forms (not calculators or read-only result pages). For example, if a tool has a "Description" or "Notes" textarea, add `AiBodyField` with `contentType` set to the tool name, e.g. `embassy_alert`, `processing_tracker`, etc.

- [ ] **Step 2: Run Svelte check**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run check
```

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/routes/(app)/tools/
git commit -m "feat(main): add japa AI to tool description fields

Wraps editor-style tool description textareas with AiBodyField."
```

---

## Task 10: Feature flag, build, and manual QA

**Files:**
- Modify: `apps/main/src/lib/ai/canUseAI.ts` (if not done)
- Modify: `apps/main/src/lib/i18n/en.json` (any missing keys)

- [ ] **Step 1: Add the `inline_ai_assistant` feature flag check**

The flag is read from `data.features` in `PageData`. In `canUseAI`, use `features.inline_ai_assistant !== false`.

- [ ] **Step 2: Build the main app**

```bash
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run build
```

Expected: build succeeds.

- [ ] **Step 3: Run full test suite**

```bash
# Backend
cd /home/jmobile/websites/japa/japaconnect
CGO_ENABLED=0 go test ./internal/ai/... -count=1

# Frontend
cd /home/jmobile/websites/japa/japaconnect/apps/main
bun run test
```

- [ ] **Step 4: Manual QA checklist**

- `/create` blog: generate body from title, switch tone, apply, regenerate.
- `/stories/new`: generate body, verify form submit still works.
- `/stories/[slug]/edit`: improve existing body.
- Main composer with custom fields: generate and fill custom fields.
- User without `ai.editor.assist`: button hidden.
- Mobile (≤768px): panel covers the viewport and closes with the X button.

- [ ] **Step 5: Commit and final summary**

```bash
git add -A
git commit -m "feat(main): enable japa AI inline assistant across create/edit forms

Adds backend POST /api/v1/ai/assist/draft, reusable AiBodyButton,
AiDraftPanel, AiBodyField, and integrates them into the main
composer and structured content/tool forms."
```

---

## Self-review

- **Spec coverage:**
  - Backend endpoint — Tasks 1–3.
  - Tone/humanization controls — Tasks 1, 6.
  - `AiBodyButton` / `AiDraftPanel` — Tasks 5–6.
  - Main composer integration — Task 7.
  - Structured content integration — Task 8.
  - Tool forms — Task 9.
  - Permission gating — Tasks 2, 5.
  - Feature flag — Task 10.
  - i18n — Tasks 5–6, 10.
  - Tests — Tasks 1–3, 10.
- **Placeholder scan:** no TBD/TODO placeholders in steps.
- **Type consistency:** `DraftRequest`, `DraftResponse`, `FieldSchema` are consistent across the backend and `apps/main/src/lib/ai/types.ts`. `onApply` payload shape is consistent in `AiDraftPanel` and consumers.
