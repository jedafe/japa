# Docs Wiki-Style Publishing — Feature Image + Revision History

Date: 2026-10-31
Status: Approved design
Surfaces: `internal/docs` (Go backend), `apps/docs` (SvelteKit docs frontend)
Ref: japaconnect-engineering/sections/44-documentation-system.md (§44 + amendments 1–4)

## Goal

Bring the docs publishing experience to wiki parity for editors: per-page
feature image, full revision history with restore and clearing, and a
wiki-like edit flow (edit summary captured per save). All editing continues
to use the shared `@japaconnect/ui` `RichTextEditor` (the same TipTap editor
as the main app's composer — already wired via `DocEditor`).

Explicitly **out of scope**: community "suggest an edit" — docs stays
`docs.manage`-only for all writes.

## Decisions (from requirements)

| Question | Decision |
|---|---|
| Scope | Feature image + revision history + edit-flow polish |
| History clearing | Both: delete a single revision AND clear-all per page |
| Restore | Yes — "restore this version" re-points the page (as a new revision) |
| Feature image surfaces | Hero on the doc page, thumbnails on listing cards, `og:image` |

## Existing state (verified)

Already shipped by §44 in-app authoring amendments — **not rebuilt**:

- Inline edit on `/docs/{audience}/{slug}?edit=1` (`DocEditForm` + `DocEditor`)
- Create on `/docs/{audience}/new` + `/docs/api/new` (`NewPageForm`)
- TOC right sidebar + mobile TOC; categories/subcategories + managed catalog
- Review workflow: `draft → in_review → approved → published` with
  approve/publish endpoints owning reviewer metadata
- `MediaPickerModal` already used inside `DocEditor` for inline images, with
  `listMyMedia` / `uploadMediaImage` / `stockApi` helpers in `src/lib/api/media.ts`

## Backend design

### Migration

`NNNNNN_doc_page_revisions_feature_image.up/down.sql`:

```sql
ALTER TABLE doc_pages
    ADD COLUMN feature_image_id uuid REFERENCES media(id);

CREATE TABLE doc_page_revisions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    doc_page_id   uuid NOT NULL REFERENCES doc_pages(id) ON DELETE CASCADE,
    version       int  NOT NULL,
    title         text NOT NULL,
    content       jsonb NOT NULL,
    feature_image_id uuid REFERENCES media(id),
    summary       text,
    created_by    uuid REFERENCES users(id),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_doc_page_revisions_page
    ON doc_page_revisions (doc_page_id, version DESC);
```

`version` is a per-page sequence: `COALESCE(MAX(version),0)+1` inside the
same transaction as the page write.

### Write-path semantics

- `CreateDocPage` writes revision v1 (title, content, feature_image_id,
  summary, created_by).
- `UpdateDocPage` writes a new revision **only when `title`, `content`, or
  `feature_image_id` changed**. Content comparison reuses the semantic
  TipTap comparison added by amendment 3. Organizational-only changes
  (slug, category, subcategory, parent, position, audience) never version.
- `update` accepts a new optional `summary` field — stored on the revision,
  ignored when no revision is written. Create accepts `summary` too.
- Restore is a write: copies the revision's `title` + `content` +
  `feature_image_id` onto the page and inserts a new revision row with
  summary `"Restored from v{N}"` (or the caller's summary). Amendment 3
  rules apply — content-changing restore on an `approved` page demotes it
  to `in_review`.
- The **latest revision may not be deleted** (`409 conflict`); it anchors
  history to current state. Older revisions delete freely.
- `DELETE /admin/docs/pages/{id}/revisions` clears all revisions for the
  page and returns `204`. Afterwards the next content write restarts at
  v1 (MAX over empty set).

### Endpoints (all `docs.manage`-gated)

| Route | Purpose |
|---|---|
| `GET /api/v1/admin/docs/pages/{id}/revisions?page=&limit=` | Paginated list: `{id, version, title, summary, feature_image_id, created_by, created_at, excerpt}` |
| `GET /api/v1/admin/docs/revisions/{revId}` | Single revision with full `content` (view/diff before restore) |
| `POST /api/v1/admin/docs/pages/{id}/revisions/{revId}/restore` | Restore title+content+feature_image; writes new revision |
| `DELETE /api/v1/admin/docs/revisions/{revId}` | Delete one revision (not latest → 409) |
| `DELETE /api/v1/admin/docs/pages/{id}/revisions` | Clear all revisions for the page |
| `POST/PATCH /admin/docs/pages[...]` | + `feature_image_id` (uuid or `""`→NULL) and `summary` fields |
| `GET` DTOs | `DocPage` + `DocPageSummary` gain `feature_image_id` and computed `feature_image_url` (same mediaURL helper pattern as blog `cover_image_url`; empty string when unset) |

Audience-gate parity: revision endpoints check `checkPageAudienceGate` —
`docs.manage` holders without a gated audience's permission cannot read or
mutate that audience's history.

Pagination uses the standard `parsePageParams` envelope (`meta.page/limit/
total`), same shape as wiki entity revisions.

### JEOS + catalogs (before code — AGENTS.md)

- `sections/44-documentation-system.md`: new amendment covering the schema,
  write-path semantics, endpoints, and frontend surfaces.
- `conventions/api-routes-catalog.md`: register the 5 new routes.
- No new RBAC node (reuses `docs.manage`), no new Valkey keys, no new Asynq
  task types.

## Frontend design (`apps/docs`)

### Doc page (`/docs/{audience}/{slug}`)

- Hero feature image rendered above `<h1>` when `feature_image_url` present
  (rounded, `aspect-ratio` reserved to avoid CLS).
- `og:image` + `twitter:card = summary_large_image` when present.
- "History" link next to the existing "Edit this page" link (editors only)
  → `/docs/{audience}/{slug}/history`.
- `+page.server.ts` load passes `feature_image_url` through `DocPage`.

### Edit & create forms (`DocEditForm`, `NewPageForm`)

- **Feature image field**: preview thumb + "Choose image"/"Replace"/
  "Remove" buttons opening the shared `MediaPickerModal` with the app's
  existing `listMyMedia`/`uploadMediaImage`/`stockApi` helpers. Submitted
  as `feature_image_id` in the form action payload.
- **Edit summary field**: optional text input; posted as `summary` on save
  and create. Rendered later in the history list.
- Dirty tracking covers both new fields.

### History page (`/docs/{audience}/{slug}/history`)

New route inside `AppShell`, `docs.manage`-gated server-side:

- Paginated list (10/20/50/100 per page like wiki history): version, author,
  date, summary, content excerpt; status column not needed (no moderation).
- Row actions: **View** (expand/dialog rendering the revision's content via
  `renderDocContent`), **Restore** (confirm → POST restore → redirect back
  to the page), **Delete** (confirm → DELETE; hidden/disabled on latest).
- **Clear history** button in the header — typed confirm (`<Modal>` with
  the page title) → DELETE all → empty state.
- Empty state: "No revisions yet".

### Listing cards

`/docs/{audience}` index and `/docs/category/[name]` cards show a
`feature_image_url` thumbnail when present (left-thumb or top-banner
matching existing card anatomy); text-only fallback unchanged.

### Types

`DocPage`, `DocPageSummary`, `DocPageCreateRequest`, `DocPageUpdateRequest`
gain `feature_image_id`/`feature_image_url`/`summary` fields in
`apps/docs/src/lib/types/index.ts`. New `DocRevision` type.

## Error handling

- Revision list/get/delete on a nonexistent page or revision → 404.
- Deleting the latest revision → `409 revision_is_latest` (UI hides the
  action anyway; the code guards racing tabs).
- `feature_image_id` that doesn't resolve to a `media` row →
  `400 invalid_feature_image_id` (same contract as blog cover image).
- Restore on a missing revision → 404; restore failures surface through the
  standard error envelope and the history page shows the error inline.
- Optimistic lock unchanged: save still honors `expected_updated_at`.

## Testing

- Go: store/service tests — revision written on create, on content change,
  not on metadata-only change; version sequencing; latest-delete 409;
  clear-all; restore copies fields + demotes approved; feature_image_id
  validation; audience-gate on revision endpoints.
- Frontend: `bun run check`; existing serialize/catalog tests unaffected.
- Manual pass: create with image → edit with summary → history list →
  view → restore → delete single → clear all.

## Out of scope

- Community suggested edits / moderation queue (`docs.manage` only).
- Diff view between revisions (view single revision only).
- Retention auto-pruning (manual delete + clear-all chosen instead).
- Admin panel (`apps/admin`) changes — its page table can gain the new
  fields later; nothing breaks without them.
