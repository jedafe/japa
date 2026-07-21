# `/create` Composer — Design Spec

- **Date:** 2026-07-21
- **App:** Main App (`apps/main`, SvelteKit 2 · Svelte 5 Runes · SSR · port 3000)
- **Surface:** The `/create` page — **Exception #2** of the ADR-058 shell (dev.to `/new` reading/writing layout)
- **Status:** Approved design; ready for implementation planning
- **Build strategy:** Big-bang (one spec → one plan → one build), delivered as tightly-isolated components so the code reviews in pieces even though it ships in one pass.

---

## 1. Summary & Locked Scope

`/create` is the platform's full authoring surface. It replaces the current 17-line placeholder. Every scope fork was decided toward the complete Exception #2 spec:

| Decision | Choice |
|---|---|
| Content types | **Blog + custom post types (CPTs) + comparisons** |
| Body editor | **Full TipTap with all four triggers**: `/` slash, `@` mention, `#` hashtag, `[[` interlink |
| Draft model | **Debounced autosave (~2s) + manual Save draft**; publish is a separate state transition |
| Settings/SEO panel | **Full**: feature image, tags, Space, visibility, schedule, canonical, **series**, custom fields, SEO (meta title/desc, OG, index toggle) |
| Guide panel | **Dynamic** focus-reactive help (default right-panel tab) |

### Non-goals (explicitly out of this build)
- Discussion-type composing (those compose inline in `ComposeModal`, not `/create`).
- The `/forum/[category]/new` full-page thread composer (separate route).
- Revision history UI (`GET /content/{id}/revisions`), analytics, and moderation flows.
- Rich collaborative/multiplayer editing.

---

## 2. API Contract (verified 2026-07-21)

All endpoints confirmed live against the running backend (`:8080`) and/or the JEOS route catalog + Go handlers. **The Gate passes — no missing routes.**

### Create paths (the registry's two branches)
- **Blog + all CPTs → `POST /content`** (the superset endpoint). Verified input struct:
  ```
  { content_type, title, slug, body, body_html, field_values (json),
    canonical_url, status, visibility, publish_at, space_id, geo_space_id }
  ```
  Backend defaults `status→"draft"`, `visibility→"public"`, returns the created record incl. `id`.
  > **Not `POST /blog`.** `POST /blog` is a thin wrapper accepting only `{title, slug, body, body_html, status, visibility}` and internally calls `CreateContent(ContentType:"blog_post")`. It cannot carry `field_values` / `canonical_url` / `space_id` / `publish_at`, so it is unsuitable for the full composer. Blog uses `POST /content` with `content_type:"blog_post"`.
- **Comparisons → `POST /comparisons`** then sub-resources:
  `POST /comparisons/{id}/items`, `POST /comparisons/{id}/items/reorder`, `PATCH /comparisons/{id}/items/{itemId}`, `POST /comparisons/{id}/dimensions`, `PATCH /comparisons/dimensions/{id}`, `PUT /comparisons/dimensions/{id}/values`, `PATCH /comparisons/{id}`.

### Lifecycle transitions
- Autosave update: `PATCH /content/{id}` (standard) · `PATCH /comparisons/{id}` (comparison top-level).
- Publish: `POST /content/{id}/publish`.
- Schedule: `POST /content/{id}/schedule` (`publish_at`).
- Draft hydration: `GET /me/drafts`, `GET /content/{slug}` (via `?draft=<id>`).

### Supporting reads/writes
- Type selector: `GET /content-types`.
- Space selector (required): `GET /me/spaces` (+ `GET /spaces/search`).
- Tags (post-level, **deferred linkage**): `GET /tags`, `GET /suggestions/tags`, `POST /content/{id}/tags`, `DELETE /content/{id}/tags/{tagId}`.
- Series (**deferred linkage**, standard form only): `GET /blog/series`, `POST /blog/series` `{title, slug, description, cover_image_id}`, `POST /blog/series/{id}/posts` `{content_id, position}`, `DELETE /blog/series/{id}/posts/{postId}`.
- Media (cover + inline images, 2-step): `POST /media/upload` → `POST /media/confirm/{id}`.
- Custom fields (Section 06): `GET /custom-field-groups`, `GET /custom-field-groups/{id}/fields`, `GET /custom-field-entity-types`, `GET /custom-field-values/{entity_type}/{entity_id}`, `PUT /custom-field-values`. In the standard create, values also ride inside `field_values`.
- Trigger search backends: `GET /search?type=users&q=` (auth; `@`), `GET /search?type=content&q=` / `GET /search/autocomplete` (`[[`), `GET /tags?q=` / `GET /suggestions/tags` (`#`). `/` is local.

---

## 3. Architecture

### Routing & layout
- Route: `src/routes/(app)/create/+page.svelte` (replaces the stub) + `+page.server.ts` loader.
- **Exception #2 layout:** the `(app)` layout must, for this route, force the left nav to the collapsed 72px rail and suppress the standard `RightSidebar`. **Mechanism (chosen):** `(app)/+layout.svelte` derives an `isExceptionRoute` flag from `page.route.id` (matching `/(app)/create` and, later, the blog-single route), evaluated during SSR layout render so there is no client flash. This keeps the signal in the layout (which owns the grid) rather than threading state up from the child page.
- Left column = the **main working area** (form). Right column = tabbed `[ Guide | Settings/SEO ]` panel. Sticky `SaveBar` at the bottom of the left column.

### The editor-registry seam (the core abstraction)
`contentTypeRegistry.ts` (pure, unit-tested) maps a `content_type` to its editor contract:

```
contentType → {
  formKind: 'standard' | 'comparison',
  bodyEditor: Component,          // PostEditor | ComparisonBuilder
  create:  (draft) => Promise<{id, slug}>,   // POST /content | POST /comparisons
  update:  (id, patch) => Promise<void>,     // PATCH /content/{id} | PATCH /comparisons/{id}
  publish: (id) => Promise<void>,            // POST /content/{id}/publish (+ schedule)
  supportsSeries: boolean,        // standard/blog-family only
  customFieldEntityType: string,  // which custom-field groups to load
}
```

- **Blog + every CPT** resolve to `formKind:'standard'` → `PostEditor` → `POST /content`. Only `content_type` + which custom-field groups load differ between them.
- **Comparisons** resolve to `formKind:'comparison'` → `ComparisonBuilder` → `POST /comparisons` + sub-resources.
- Adding a future type = one registry entry, no shell surgery.

### File structure
```
routes/(app)/create/
  +page.server.ts     load: content-types, me/spaces, custom-field-groups, series, ?draft hydration, csrf
  +page.ts            layout flags (collapse nav, hide right sidebar)
  +page.svelte        hosts CreateShell
lib/components/create/
  CreateShell.svelte          layout host; owns composer state; nav-collapse; hides RightSidebar
  contentTypeRegistry.ts      pure type→editor/endpoint/rules map
  useComposerState.svelte.ts  autosave state machine ($state class) + deferred-linkage reconciler
  editors/
    PostEditor.svelte         TipTap host (blog/CPT body)
    ComparisonBuilder.svelte  items × dimensions grid editor
  fields/
    CoverUploader.svelte  TitleField.svelte  TagInput.svelte  SpaceSelector.svelte
  panel/
    CreateRightPanel.svelte   [Guide|Settings] tab host
    CreateGuidePanel.svelte   focus-reactive help
    CreateSettingsPanel.svelte visibility, schedule, canonical, series, custom fields, SEO
    SeriesSelector.svelte     existing (GET) or create-new (POST) inline
  SaveBar.svelte              sticky Save draft / Publish + status pill
lib/tiptap/
  slashMenu.ts  mention.ts  hashtag.ts  interlink.ts   (suggestion extensions + query-shaping utils)
lib/api/create.ts            typed client wrappers for create/patch/publish/upload/link/search calls
```

### Deep-module boundaries
- `CreateShell` knows layout + hosts state; knows nothing about how a field talks to the API.
- `useComposerState` knows the draft lifecycle + dirty tracking + the deferred-linkage queue; knows nothing about layout.
- `contentTypeRegistry` is pure data.
- Each `fields/*` and `editors/*` owns one input, emits a typed value via an `onchange` callback prop, never reaches into siblings.

---

## 4. Component Tree

```
+page.svelte
└─ CreateShell
   ├─ LEFT (working area)
   │  ├─ CoverUploader ...... 2-step upload → media id + url
   │  ├─ TitleField ......... autosizing; drives slug suggestion
   │  ├─ TagInput ........... chips; GET /tags + /suggestions/tags autocomplete
   │  ├─ SpaceSelector ...... GET /me/spaces; REQUIRED ≥1 → gates Publish
   │  ├─ <body via registry>
   │  │   ├─ PostEditor ..... TipTap: slash / mention / hashtag / interlink
   │  │   └─ ComparisonBuilder items × dimensions grid
   │  └─ SaveBar ............ status pill + Save draft + Publish
   └─ RIGHT (tabbed panel)
      └─ CreateRightPanel [ Guide | Settings ]
         ├─ CreateGuidePanel ... focus-reactive help (Space rule / Tags / Series / body triggers)
         └─ CreateSettingsPanel
              ├─ VisibilitySelect (public / unlisted / private)
              ├─ ScheduleField → publish_at
              ├─ CanonicalField → canonical_url
              ├─ SeriesSelector (standard form only)
              ├─ CustomFieldGroup[] per content_type → field_values
              └─ SeoFields (meta title/desc, OG image, index toggle)
```

---

## 5. Data Flow & State Machine

### Load (`+page.server.ts`, SSR, each degrading gracefully)
Parallel: `GET /content-types` · `GET /me/spaces` · `GET /custom-field-groups` (+ entity-types) · `GET /blog/series` · `?draft=<id>` hydration · CSRF/session from layout data.

### Composer state machine (`useComposerState.svelte.ts`)
```
pristine ──first meaningful edit──► creating ──POST /content → id──► saved
                                                                       │
      ┌──────────────── edit ◄──────────────────────────────────────┘
      ▼
    dirty ──2s debounce──► saving ──PATCH /content/{id}──► saved

saved ──Publish (valid)──► POST /content/{id}/publish  ─► published
      └─ publish_at set ──► POST /content/{id}/schedule ─► scheduled
```
- **Single-flight + revisioning:** each save carries a local monotonic `rev`; a stale response (lower `rev`) is dropped. Edits during an in-flight save re-mark `dirty`; on resolve, if still dirty, reschedule. Debounce cancels on publish/unmount.
- **Status pill:** *Draft · Saved just now* / *Saving…* / *Scheduled for…* / *Published* / *Couldn't save · Retry*.

### Post-create linkage reconciler
Two things need a `content_id` that may not exist yet — **tags** (`POST /content/{id}/tags`) and **series** (`POST /blog/series/{id}/posts`). One queue serves both:
- chosen while `pristine` → held pending → flushed immediately after `creating` resolves;
- changed while `saved` → reconciled at once (diff add/remove).

### Comparisons
Save granularly: `POST /comparisons` mints the shell; title/desc autosave via `PATCH /comparisons/{id}`; items/dimensions/values persist through their own mutations as the grid is edited (no single-blob PATCH).

### Four triggers (TipTap suggestion utils; ~200ms debounce; min-length gate; keyboard-nav popup)
| Trigger | Backend | Inserts |
|---|---|---|
| `/`  | local | block (heading / list / quote / code / image) |
| `@`  | `GET /search?type=users&q=` | mention node (user id + handle) |
| `#`  | `GET /tags?q=` · `/suggestions/tags` | hashtag node |
| `[[` | `GET /search?type=content&q=` · `/search/autocomplete` | interlink node (content id + slug) |

---

## 6. Error Handling & Validation

### Publish gate (button disabled with inline reason until all hold)
title non-empty · **≥1 primary Space selected** (non-negotiable) · body non-empty (standard) or ≥2 items + ≥1 dimension (comparison) · all **required** custom fields filled. Field-level errors at the control; a summary by the Publish button. API envelope field codes (`missing_title`, `missing_slug`, …) map back to the same field messages on server rejection.

### Autosave failure classes
- **5xx / network:** pill → *Couldn't save · Retry*; keep dirty buffer; exponential backoff (a few attempts); auto-resume on reconnect. Work is never lost.
- **4xx validation:** stop retrying; surface message on the offending field.
- **409 conflict:** non-destructive banner *"This draft changed elsewhere — reload?"* (defensive even if backend doesn't emit it yet).

### Other
- **Upload:** validate type/size client-side before `POST /media/upload`; failure shows retry on that control only; never blocks the form.
- **Trigger search:** popup shows inline *"Couldn't load suggestions"*; author finishes the token as plain text; editing never blocked.
- **Load degradation:** content-types fails → fall back to `blog_post` only + notice; **spaces fails → SpaceSelector errors and Publish stays gated** (must not "publish anyway" — that violates the primary-space invariant); series/custom-fields fail → those sections show inline errors, rest works. Uses shared `EmptyState`, distinct from `LoadingSkeleton`.
- **Navigation:** `beforeNavigate` flushes a pending debounce; only warns if a save is actively failing.

---

## 7. Testing Strategy

Tooling: Vitest 4 + `@testing-library/svelte` 5 + jsdom + svelte-check. Co-located `.test.ts`, matching the existing `cardStyle.test.ts` style.

### Unit
- `contentTypeRegistry.test.ts` — resolution + unknown-type fallback to `blog_post`.
- **`useComposerState.svelte.test.ts` (crown jewel)** — mocked API client; cover every transition; single-flight revisioning (stale drop); `canPublish` gate; publish-vs-schedule branch; deferred-linkage queue (held while pristine → flushed after create → diff-reconciled after saved); 4xx-vs-5xx classification + backoff.
- Pure helpers: slug suggestion, `field_values` assembly, trigger query-parsing/request-shaping for `@ # [[`.

### Component (`@testing-library/svelte` + jsdom)
`SpaceSelector` gating, `TagInput` chip add/remove, `SaveBar` status pill per state, `CreateGuidePanel` focus-reactivity.

### TipTap
Test the suggestion **data-shaping utilities** as pure functions; mock ProseMirror for node insertion. No full editor E2E in unit tests.

### Contract verification (required manual acceptance — NOT a unit test)
Mock-shaped tests structurally cannot catch envelope/shape drift (cf. the card-style feature's two double-encoding bugs that passed all unit tests and were caught only by a live round-trip). **Mandatory acceptance:** against the running backend, create a draft → autosave → attach tag + series → publish, and confirm each persists.

### Baseline discipline
All five frontend suites are green as of the last baseline; any failure introduced is a new regression. `bun run test` + `bun run check` must pass before done.

---

## 8. Cross-cutting Requirements
- **Svelte 5 strict Runes** (`$state`, `$derived`, `$props`, `$effect`); no `export let`, `$:`, `on:click`, `createEventDispatcher`, or localStorage for state.
- **Bits UI** dot-notation for all interactive primitives (`Select.*`, `Dialog.*`, `Tabs.*`, `Popover.*`).
- **Tailwind v4** utilities; theme via CSS custom properties; dark mode via `[data-theme]` (never Tailwind `dark:`); never hardcode hex.
- **i18n:** every user-facing string via `$t('key')`; add keys to `en.json` AND `fr.json`.
- **Rule 16:** all data from `/api/v1`; `load()` never returns static data; mutations use CSRF from `$page.data.csrfToken`.
- Every async section gets a `LoadingSkeleton`; empty/error states use `EmptyState`.

---

## 9. Acceptance Criteria
1. `/create` renders the Exception #2 layout (72px nav, form-left, tabbed panel-right), SSR, no client flash.
2. Selecting a content type swaps the body editor and settings via the registry; comparisons show the grid builder.
3. Typing autosaves a draft (debounced) with an honest status pill; refresh via `?draft=<id>` restores it.
4. `/ @ # [[` all work, each backed by its live search endpoint (or local for `/`).
5. Full Settings panel functional: cover upload, tags, space, visibility, schedule, canonical, series, custom fields, SEO.
6. Publish is gated on title + ≥1 Space (+ required custom fields) and transitions state; schedule uses `publish_at`.
7. Tags and series attach correctly whether chosen before or after first save.
8. Errors classified (4xx vs 5xx vs 409); no data loss on transient failure.
9. `bun run test` + `bun run check` green; live round-trip acceptance passes.

---

## 10. Risks & Open Questions
- **Comparison editor is a mini-app.** Its granular sub-resource saving is the largest single-component risk; if it threatens the big-bang timeline, it is the natural candidate to land last within the same push.
- **Exact `field_values` shape** for custom fields (`GET /custom-field-groups/{id}/fields` field schema → how values serialize into `field_values` vs `PUT /custom-field-values`) to be pinned precisely during planning by reading the content service + custom-field handlers.
- **`content_type` enum values** (`blog_post`, CPT slugs from `GET /content-types`) to be confirmed from the live `content-types` response during planning.
- **Feature-flag gating:** `/blog` and `/comparisons` groups are feature-flag-gated (`feature.blog`, comparisons flag). `POST /content` with `content_type:"blog_post"` should be verified not to bypass an intended gate; if the blog flag is off, the blog type should be hidden from the selector.
