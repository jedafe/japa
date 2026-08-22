# Japa AI Body Assistant Design

**Status:** Design — pending implementation plan
**Refs:** §06 (Custom Fields), §14 (Content Type System), §20 (AI Chat & Content), §26 (Content Studio), §79 (Community Story Wall), §87 (Japa Stage Profiles), ADR-058, ADR-076, ADR-088

---

## 1. Problem

Users create many kinds of content on JapaConnect — blog posts, forum threads, stories, scam alerts, achievements, planning groups, calendar events, jobs, mutual aid requests, AMA questions, and structured tool forms. Each of these forms has a body (or description) field. Writing that body from scratch is friction, especially for structured content where users also have to fill custom fields. There is no inline, one-click AI assistant that can draft or improve the body and, where relevant, suggest values for the surrounding fields.

## 2. Goal

Add a reusable **"japa AI"** button to every create/edit form body field across `apps/main`. The button opens a side drafting panel that lets eligible users generate, improve, or rewrite body content. For the main composer it can also fill custom fields. The feature is gated by the existing `ai.editor.assist` RBAC permission and supports tone and humanization controls.

## 3. User flow

1. The user opens any create or edit form with a body field.
2. If they hold the `ai.editor.assist` permission, a "japa AI" button appears next to the body label or inside the editor chrome.
3. Clicking the button opens a right-side (or bottom on mobile) drafting panel.
4. The panel shows:
   - a prompt input for free-form instructions (e.g., "make this more formal", "expand to three paragraphs"),
   - a tone selector,
   - a humanization-level selector,
   - a preview of the generated draft,
   - Apply / Cancel / Regenerate actions.
5. The user clicks **Apply**; the draft is inserted into the form body. If custom fields are provided, the panel also applies any returned `field_values`.

## 4. Backend API design

### 4.1 New endpoint

Add a handler under the existing `ai.editor.assist` permission:

```
POST /api/v1/ai/assist/draft
```

### 4.2 Request body

```json
{
  "content_type": "blog_post",
  "title": "My journey from Lagos to London",
  "existing_body": "<p>...</p>",
  "instructions": "Expand into a friendly, three-paragraph story",
  "tone": "friendly",
  "humanize": true,
  "humanize_level": "medium",
  "field_schema": [
    { "key": "best_advice", "type": "textarea", "label": "Best Advice" }
  ],
  "context": {
    "destination_country": "United Kingdom",
    "origin_country": "Nigeria"
  }
}
```

| Field | Required | Description |
|---|---|---|
| `content_type` | yes | The type of content being drafted, e.g. `blog_post`, `story`, `guide`, `comparison`. |
| `title` | no | Current title. Empty or partial titles allow the AI to suggest a better one. |
| `existing_body` | no | Current body HTML/text. Used for improve/rewrite flows. |
| `instructions` | no | Free-form user instruction. |
| `tone` | no | One of `professional`, `friendly`, `concise`, `persuasive`, `academic`. Defaults to `professional`. |
| `humanize` | no | Override flag for humanization. |
| `humanize_level` | no | One of `off`, `light`, `medium`, `heavy`. |
| `field_schema` | no | Array of custom-field descriptors for structured content fill. |
| `context` | no | Extra context (origin, destination, visa route, etc.). |

### 4.3 Response body

```json
{
  "data": {
    "title": "My Lagos to London Journey",
    "body_html": "<p>...</p>",
    "field_values": {
      "best_advice": "Start your visa paperwork early."
    },
    "tokens_used": 123
  }
}
```

### 4.4 Handler behavior

- Requires auth and the `ai.editor.assist` RBAC node.
- Uses the existing `moderate` rate-limit bucket (same as other `/ai/assist/*` routes).
- Merges `humanize` / `humanize_level` with the tenant `ai.humanize.*` settings via `resolveHandlerHumanize`.
- Calls `dispatchAndRecordUsage` with `category="content_generation"` and `task="assist_draft"`.
- Builds a prompt that asks the model to return a JSON object containing `body_html`, `title`, and (when `field_schema` is provided) `field_values`.
- Sanitizes the returned `body_html` before returning it.
- Returns a JSON-shaped provider response and parses `body_html`, `title`, and `field_values` from it. If the model does not return valid JSON, the entire response is treated as `body_html` and `title`/`field_values` are omitted.
- Only returns field values whose keys match the provided `field_schema` and whose types can be safely coerced.
- This first version is a single JSON request/response; an SSE streaming variant can be added later by reusing the same panel UI and a separate `/ai/assist/draft/stream` route.

## 5. Frontend components

### 5.0 Shared types

```ts
interface FieldSchema {
  key: string;
  type: 'text' | 'textarea' | 'number' | 'select' | 'date' | 'url';
  label: string;
  options?: { value: string; label: string }[];
}
```

### 5.1 `AiBodyButton.svelte`

- Location: `apps/main/src/lib/ai/AiBodyButton.svelte`
- Props:
  - `contentType: string`
  - `title: string` (reactive)
  - `body: string` (reactive)
  - `context?: Record<string, unknown>`
  - `fieldSchema?: FieldSchema[]`
  - `disabled?: boolean`
  - `onOpen: () => void` (or bindable `open`)
- Only renders when `data.user?.permissions?.includes('ai.editor.assist')` and the feature flag `inline_ai_assistant` is enabled.
- Uses a sparkles icon and the label "japa AI".

### 5.2 `AiDraftPanel.svelte`

- Location: `apps/main/src/lib/ai/AiDraftPanel.svelte`
- Props:
  - `open: boolean` (bindable)
  - `contentType: string`
  - `title: string`
  - `body: string`
  - `context?: Record<string, unknown>`
  - `fieldSchema?: FieldSchema[]`
  - `onApply: (payload: { body: string; title?: string; fieldValues?: Record<string, unknown> }) => void`
- Local state: `prompt`, `tone`, `humanize`, `humanizeLevel`, `draft`, `loading`, `error`.
- Calls `POST /api/v1/ai/assist/draft`.
- Renders the preview in a scrollable area.
- Apply replaces the parent body and, if present, merges `field_values`.
- Cancel discards the draft and closes the panel.
- Regenerate re-runs the request with the same settings.

### 5.3 `AiBodyField.svelte` (optional wrapper)

- Convenience component for plain `<textarea>` body fields.
- Composes a label, the textarea, and `AiBodyButton` with consistent spacing.

## 6. Integration points

### 6.1 Main composer (`/create` and content edit)

- `CreateShell.svelte` receives `user` and `bodyHtml`.
- `PostEditor.svelte` adds `AiBodyButton` to the editor chrome.
- Clicking opens `AiDraftPanel` as a right slide-over.
- On apply: `bodyHtml = payload.body`; `title = payload.title ?? title`; merge `field_values` into `fieldValues`; call `markDirty()`.

### 6.2 Structured content create/edit pages

For each page with a body field, add `AiBodyButton` next to the body label and wire `AiDraftPanel`:

- `/stories/new`, `/stories/[slug]/edit`
- `/scam-alerts/new`, `/scam-alerts/[id]/edit`
- `/achievements/new`, `/achievements/[slug]/edit`
- `/planning-groups/new`, `/planning-groups/[id]/edit`
- `/calendar/new`, `/calendar/[id]/edit`
- `/jobs/create`, `/jobs/[slug]/edit`
- `/mutual-aid/new`, `/mutual-aid/[id]/edit`
- `/ama/new`
- `/services/new`, `/services/[slug]/edit`

For each, pass the relevant `contentType` (e.g. `story`, `scam_alert`, `achievement`), current `title`, and current `body` to the panel.

### 6.3 Tool forms

Several functional tools (`/tools/embassy-alerter`, `/tools/processing-tracker`, etc.) have body/description fields. Where the page is an editor form rather than a calculator, add `AiBodyButton` to the description textarea.

## 7. Permission and eligibility

- The button is hidden unless the user has the `ai.editor.assist` permission.
- The permission is checked client-side from `data.user?.permissions` and the backend re-checks it on every API call.
- A feature flag `inline_ai_assistant` (default `true`) lets operators disable the feature globally.

## 8. Error handling and UX

- **Loading**: show a typing/skeleton indicator in the panel; the form remains editable.
- **Errors**: display inline in the panel with a retry button. Do not mutate the parent form on error.
- **Permission errors**: if the backend returns `403`, show "AI assistant not available" and close the panel.
- **Rate limit**: on `429`, show a countdown and retry when allowed.
- **Preview-before-apply**: always show a read-only preview. Apply is explicit.
- **Undo**: after applying, show a 5-second toast to revert.
- **Accessibility**: `aria-expanded`, focus trap, `Esc` to close, `Ctrl/Cmd + Enter` to submit.

## 9. Testing

### 9.1 Backend

- `TestHandleDraft_MissingPermission` — 403.
- `TestHandleDraft_MissingTitleAndBody` — 400.
- `TestHandleDraft_ValidBlogPost` — returns `body_html` and `title`.
- `TestHandleDraft_WithFieldSchema` — returns matching `field_values`.
- `TestHandleDraft_HumanizeOverride` — merges request and global settings.

### 9.2 Frontend

- `AiBodyButton` renders only with `ai.editor.assist` permission.
- `AiDraftPanel` opens, calls the API, and emits `onApply`.
- `CreateShell` applies the draft to `bodyHtml` and `fieldValues`.

### 9.3 Manual QA

- `/create` — generate from title, switch tone, apply.
- `/stories/[slug]/edit` — improve existing body.
- Main composer with custom fields — fill body and fields.
- Permission absent — button hidden.
- Mobile — panel opens as bottom sheet.

## 10. Implementation phases

### Phase 1 — Backend endpoint
- Add `HandleDraft` and `POST /api/v1/ai/assist/draft` route.
- Wire `ai.editor.assist` permission and rate limit.
- Add handler tests.

### Phase 2 — Frontend components
- Build `AiBodyButton` and `AiDraftPanel`.
- Add feature-flag gating.
- Add i18n keys.

### Phase 3 — Main composer integration
- Add the button to `PostEditor` and the panel to `CreateShell`.
- Wire title/body/custom-field updates.

### Phase 4 — Structured content integration
- Add `AiBodyButton` to each structured create/edit form.
- Add `fieldSchema` where the page has extra fields worth filling.

### Phase 5 — Tool forms and QA
- Apply to relevant tool forms.
- Run manual QA and component tests.

## 11. Risks and mitigations

| Risk | Mitigation |
|---|---|
| New endpoint returns malformed HTML | Sanitize on the backend before returning; render preview as-is but trust the backend. |
| AI overwrites existing work without warning | Require explicit Apply; show preview and undo toast. |
| Permission leaks the button to ineligible users | Check on client and re-check on every API call. |
| Custom field fill returns wrong types | Coerce per `field_schema` type; ignore unknown keys. |
| Mobile side panel is cramped | Open as bottom sheet / full-screen drawer on small viewports. |

## 12. Summary

Add a permission-gated, reusable "japa AI" assistant next to every create/edit body field. A new backend endpoint `POST /api/v1/ai/assist/draft` generates body HTML, optionally improves the title, and fills custom fields when given a schema. The frontend provides `AiBodyButton` and `AiDraftPanel` components and integrates them into the main composer and every structured content form in `apps/main`.
