# Section 45 — Post Create Page: Implementation Complete

**Date:** 2026-06-25
**Phase:** 3
**Section:** 45
**Status:** Complete (frontend shell; backend save/publish wiring deferred)

## Overview

Section 45 implements the Post Create Page — the primary interface where authors
compose and publish content on JapaConnect. The implementation follows a
frontend-first strategy: the full UI shell (content type selector, editor layout,
header, sidebar, and SvelteKit routes) is built and wired together, while backend
save/publish API connections and TipTap editor integration are deferred to Phase 4.

The section also adds the `editor_block_permissions` migration and sqlc queries
that control which block types each user group can insert into the editor, enabling
granular access control for rich content blocks (embeds, AI tools, etc.) without
requiring separate RBAC permission nodes for every block type.

The content creation flow starts at `/create`, where the user selects a content
type from a card grid (ContentTypeSelector). This navigates to
`/create/{content_type}`, where the PostCreateLayout renders the full editor
experience: a sticky header with save/publish/preview actions, a title input, a
TipTap editor placeholder, and a collapsible sidebar with tags, featured image,
space selection, scheduling, visibility, SEO fields, and custom fields.

## Components Implemented

### Migration 000011 — Editor Block Permissions

1 new table in `migrations/` (now 11 migration files, 000001–000011):

1. **editor_block_permissions** — Controls which block types each user group can
   insert into the content editor. Composite PK on `(block_type, user_group_id)`.
   `enabled` boolean flag allows disabling a block type without deleting the row.
   FK to `user_groups` with `ON DELETE CASCADE`.

Seed data:

- **super_admin** group: all 12 block types enabled except `ai_generate` (disabled)
- **members** group: 10 block types enabled; `embed` and `ai_generate` disabled
- **guest** group: 5 basic blocks enabled (paragraph, heading, image, divider,
  code_block); all others disabled

Block types seeded: `paragraph`, `heading`, `image`, `divider`, `code_block`,
`quote`, `list`, `table`, `video_embed`, `comparison_embed`, `embed`,
`ai_generate`.

Conventions: deterministic UUIDs for system groups (matching Section 03 seeds),
`is_system`-style implicit protection via the seed pattern, composite PK for
natural uniqueness.

### sqlc Queries

3 queries in `internal/db/queries/editor_block_permissions.sql` (18 lines):

- `GetEnabledBlocksForGroups` (`:many`) — returns distinct block types enabled
  for a set of user group IDs. Uses `ANY(@group_ids::uuid[])` for batch lookup.
- `UpsertEditorBlockPermission` (`:exec`) — insert or update (on conflict)
  a block permission row. Used for admin CRUD.
- `DeleteEditorBlockPermission` (`:exec`) — delete a specific block permission
  row by composite key.

Generated code:

- `internal/db/gen/editor_block_permissions.sql.go` (75 lines) — 3 generated
  methods: `GetEnabledBlocksForGroups`, `UpsertEditorBlockPermission`,
  `DeleteEditorBlockPermission`. Parameter structs use `pgtype.UUID` for the
  group ID (standard sqlc convention for this project).

### Frontend: Content Type Selector

**ContentTypeSelector.svelte** (79 lines) — `apps/main/src/lib/create/`

- Card grid component that renders a list of content types as clickable cards
- Each card shows an icon (from Lucide via an `ICON_MAP` lookup), label,
  description, and a "Create" link with arrow
- Responsive grid: 1 column mobile, 2 columns tablet, 3 columns desktop
- Props: `types: ContentType[]` where ContentType has `slug`, `icon`, `label`,
  `description`
- Icon map supports 15 Lucide icons: FileText, MessageSquare, BookOpen, Calendar,
  HelpCircle, Users, Compass, ClipboardList, GraduationCap, Briefcase, Heart,
  Scale, Home, CreditCard, Star

### Frontend: Editor Header

**EditorHeader.svelte** (114 lines) — `apps/main/src/lib/create/`

- Sticky header bar with three sections: left (back + content type badge),
  center (status indicator), right (action buttons)
- Left: ArrowLeft back button + content type label badge (blue pill)
- Center: shows "Saving..." with spinner when `isSaving`, "Publishing..." with
  spinner when `isPublishing`, green dot "Published" when status is published,
  gray dot "Draft" otherwise
- Right: Save Draft (Save icon), Preview (Eye icon), Publish (Send + ChevronDown)
  buttons. Publish is primary blue; others are outlined.
- Uses Svelte 5 `$props()` for all inputs; no internal reactive state
- Uses Lucide Svelte icons throughout (ArrowLeft, Save, Eye, Send, ChevronDown,
  Loader2)

### Frontend: Editor Sidebar

**EditorSidebar.svelte** (450 lines) — `apps/main/src/lib/create/`

- Full-featured sidebar with 7 collapsible `<details>` sections, each with a
  Lucide icon in the summary
- **Tags** (open by default): text input with comma-separated tag entry, tag
  chips with remove buttons, Add button
- **Featured Image**: URL input with preview thumbnail, empty state with upload
  icon placeholder, remove button overlay on preview
- **Space**: dropdown select with 8 hardcoded JapaConnect spaces (General,
  Visa & Immigration, Housing, Jobs, Culture, Healthcare, Education, Finance)
- **Schedule**: datetime-local input with helper text ("Will be published at the
  scheduled time" / "Leave empty to publish immediately")
- **Visibility** (open by default): three radio options (Public/Members
  Only/Private) with icons (Globe/Users/Lock) and descriptions
- **SEO**: title input (60 char counter) and description textarea (160 char
  counter)
- **Custom Fields** (conditional): renders field types — text, textarea, select,
  number, date, checkbox — from a `CustomField[]` prop. Only shown when
  `customFields.length > 0`.
- Props interface defines callbacks: `onTagsChange`, `onImageChange`,
  `onSpaceChange`, `onScheduleChange`, `onVisibilityChange`, `onSeoChange`,
  `onCustomFieldChange`
- Uses internal `$state` only for `tagInput` (the text input value)

### Frontend: Post Create Layout

**PostCreateLayout.svelte** (220 lines) — `apps/main/src/lib/create/`

- Main layout wrapper composing EditorHeader, EditorSidebar, and the editor area
- Props: `contentType`, `initialTitle`, `initialContent`, `onSave`, `onPublish`
- Internal reactive state (12 `$state` variables): title, content, tags,
  featuredImage, spaceId, scheduledAt, visibility, seoTitle, seoDescription,
  isSaving, isPublishing, status, sidebarOpen
- `buildMetadata()` collects all sidebar fields into a single metadata object
  passed to `onSave`/`onPublish` callbacks
- `handleSave()`: sets `isSaving = true`, calls `onSave` callback, resets after
  1s timeout (placeholder for real API call)
- `handlePublish()`: sets `isPublishing = true`, calls `onPublish` callback,
  transitions to `status = 'published'` after 1.5s timeout (placeholder)
- **Autosave**: `$effect` sets a 30-second `setInterval` that triggers
  `handleSave()` whenever there is title or content. Cleanup via returned
  `clearInterval`.
- Mobile sidebar: floating toggle button (bottom-right, visible < `lg` breakpoint)
  opens a slide-over panel with a backdrop overlay. Desktop sidebar is inline
  and collapsible.
- Editor area: title input (large, borderless, placeholder "Untitled") and a
  `min-h-[500px]` content area that renders `{@html content}` when content
  exists, or a "Start writing..." placeholder. TipTap integration is deferred
  to Phase 4.
- Sidebar rendering is duplicated for desktop (inline) and mobile (fixed
  slide-over) to handle the different layout contexts cleanly.

### SvelteKit Routes

**`apps/main/src/routes/create/+page.svelte`** (29 lines)

- Content type selection page at `/create`
- Renders `ContentTypeSelector` with 7 hardcoded content types: blog_post,
  discussion, guide, event, question, listing, comparison
- Sets `<svelte:head>` title and meta description
- Full-page layout with heading "Create New Post" and subtitle

**`apps/main/src/routes/create/[content_type]/+page.svelte`** (49 lines)

- Dynamic route at `/create/{content_type}`
- Uses `$page.params.content_type` to look up the content type from a hardcoded
  map (same 7 types as the selection page)
- Uses `$derived` for `contentType` and `pageTitle` based on route param
- Renders `PostCreateLayout` with `onSave` and `onPublish` callbacks (currently
  `console.log` — real API wiring deferred)
- Shows a "Content type not found" error page with link back to `/create` for
  unknown types
- Sets `<svelte:head>` title dynamically

**`apps/main/src/routes/create/[content_type]/+page.server.ts`** (14 lines)

- Server-side load function validating `params.content_type` against a readonly
  tuple of valid types
- Returns `{ contentType: params.content_type }` on success
- Calls `error(404, ...)` for invalid types — prevents client-side rendering for
  non-existent content types

## Test Summary

No new backend tests in this section — the `editor_block_permissions` queries are
standard sqlc-generated CRUD that will be exercised through integration tests when
the admin API routes are wired in Phase 4.

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 9 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 22 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 7 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 37 | PASS |
| internal/spaces | 13 | PASS |
| internal/content | 10 | PASS |
| internal/renderer | 2 | PASS |
| internal/mentions | 5 | PASS |
| internal/linkpreview | 6 | PASS |
| **Total** | **131** | **ALL PASS** |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all 14 packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |
| Frontend TypeScript check | clean |
| Frontend build (SSR) | passing |
| Frontend build (client) | passing |

## API Routes Added

No new API routes in this section. The existing `/page-builder/block-types`
route (wired in server.go line 258) returns the list of available block types
for the current user's permissions — this was already present before Section 45
and is used by the page builder, not the post create flow.

## Permission Keys Registered

3 new permission keys added to `bootstrapPermissions` in `server.go` (lines
327–329), bringing the total to 36:

| Key | Description |
|-----|-------------|
| `content.page_builder.create` | Create pages with page builder |
| `content.page_builder.edit` | Edit pages with page builder |
| `content.page_builder.delete` | Delete page builder pages |

All 36 permissions are assigned to the `super_admin_role`, granted to the
`super_admin` system group. Every operation is idempotent.

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `migrations/000011_editor_block_permissions.sql` | editor_block_permissions table + 3 system group seeds (62 lines) |
| 2 | `internal/db/queries/editor_block_permissions.sql` | 3 sqlc queries for block permission CRUD (18 lines) |
| 3 | `internal/db/gen/editor_block_permissions.sql.go` | Generated query methods (75 lines) |
| 4 | `apps/main/src/lib/create/ContentTypeSelector.svelte` | Card grid for content type selection (79 lines) |
| 5 | `apps/main/src/lib/create/EditorHeader.svelte` | Sticky header with back, status, save/publish/preview (114 lines) |
| 6 | `apps/main/src/lib/create/EditorSidebar.svelte` | Sidebar with tags, image, space, schedule, visibility, SEO, custom fields (450 lines) |
| 7 | `apps/main/src/lib/create/PostCreateLayout.svelte` | Main layout composing header, editor area, sidebar with autosave (220 lines) |
| 8 | `apps/main/src/routes/create/+page.svelte` | Content type selection route (29 lines) |
| 9 | `apps/main/src/routes/create/[content_type]/+page.svelte` | Dynamic editor route (49 lines) |
| 10 | `apps/main/src/routes/create/[content_type]/+page.server.ts` | Server-side content type validation (14 lines) |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added 3 page_builder permission keys to bootstrapPermissions (lines 327–329); total now 392 lines |
| 2 | `internal/db/gen/models.go` | Regenerated by `sqlc generate` to include `EditorBlockPermission` struct |
| 3 | `internal/db/gen/querier.go` | Regenerated by `sqlc generate` to include 3 new query interfaces |
| 4 | `internal/db/gen/db.go` | Regenerated by `sqlc generate` to register new query methods |

## Key Decisions

- **Frontend-first implementation for Section 45** — the full UI shell is built
  and verified (`bun run check` passes) before backend save/publish API routes
  exist. This allows the content creation UX to be reviewed and iterated on
  independently. Backend wiring is a Phase 4 concern.
- **Content types hardcoded in frontend** — both the selector page and the
  dynamic route use hardcoded content type maps (7 types: blog_post, discussion,
  guide, event, question, listing, comparison). Real content types will be fetched
  from the API (via the content type system in Section 14) in Phase 4. The
  server-side validation in `+page.server.ts` uses the same hardcoded list for
  404 gating.
- **Editor uses TipTap placeholder** — the content area renders `{@html content}`
  as a temporary placeholder. Full TipTap editor integration (using the Editor
  component from Section 15) is deferred to Phase 4. The placeholder allows the
  layout, header, sidebar, and autosave logic to be tested end-to-end without
  TipTap.
- **Autosave every 30 seconds with $effect** — PostCreateLayout uses a Svelte 5
  `$effect` to set up a 30-second `setInterval` that triggers `handleSave()`.
  The effect cleans up the interval when the component unmounts or when there is
  no content. The current `handleSave` is a stub (1s timeout, no API call).
- **Collapsible sidebar on mobile with slide-over panel** — the sidebar is
  rendered twice: once as an inline panel on desktop (`lg:` breakpoint) and once
  as a fixed slide-over with backdrop on mobile. A floating toggle button
  (bottom-right) controls visibility. This pattern avoids complex CSS transitions
  between layouts.
- **`editor_block_permissions` uses row presence + enabled flag** — rather than
  a simple "has permission / does not have permission" boolean, the design uses
  composite PK rows with an `enabled` flag. This allows disabling a block type
  without deleting the permission row, making it easy to re-enable later. The
  `GetEnabledBlocksForGroups` query uses `DISTINCT block_type` for efficient
  group-level lookups.
- **Backend route already wired** — the `/page-builder/block-types` endpoint
  (server.go line 258) was wired before Section 45 and returns block types
  filtered by user group permissions. This was not created in this section but
  is referenced by the page builder and will be used by the editor in Phase 4.

## Known Issues and Deferrals

- **Editor block permissions API routes (admin CRUD)** — the migration and sqlc
  queries exist, but no HTTP handlers or admin routes for managing block
  permissions are wired yet. The `UpsertEditorBlockPermission` and
  `DeleteEditorBlockPermission` methods are available in the generated `gen`
  package for when the admin API is built.
- **TipTap editor integration (Phase 4)** — the content area in PostCreateLayout
  is a plain `{@html content}` placeholder. The real TipTap Editor component
  (from `src/lib/components/editor/Editor.svelte`, Section 15) needs to be
  mounted here with proper state binding.
- **API connection for save/publish (Phase 4)** — `handleSave()` and
  `handlePublish()` are stubs that call `console.log` (in the route component)
  and use `setTimeout` to simulate async behavior. Real API calls to
  `POST /api/v1/contents` and `PATCH /api/v1/contents/{id}` (with status change)
  are Phase 4 work.
- **Real content type fetching from API** — both the selector and route pages use
  hardcoded content type maps. In Phase 4, these should be fetched from the
  content type registry (Section 14) via an API endpoint, with the server load
  function hydrating the data.
- **Custom fields not wired to editor** — EditorSidebar accepts a
  `customFields` prop and renders them, but PostCreateLayout does not yet fetch
  or pass custom field definitions from the content type configuration. This
  depends on the custom fields API (Section 06) being connected to the frontend.
- **Space list hardcoded** — the sidebar's space dropdown uses 8 hardcoded spaces
  instead of fetching from the spaces API (Section 05). This is a Phase 4
  concern.
- **Preview functionality placeholder** — `handlePreview()` in PostCreateLayout
  is an empty function. Preview should open the content in a new tab using the
  SSR renderer (Section 15) when wired.
- **`<svelte:head>` placement** — the `+page.svelte` at `/create` places
  `<svelte:head>` at the top level correctly. The error-state `+page.svelte` at
  `/create/[content_type]` also places `<svelte:head>` at the top level. This
  follows the SvelteKit convention (error from prior sessions: `<svelte:head>`
  cannot be inside elements or blocks).

## Next Steps

Phase 4 — Backend Integration. This phase connects the Post Create Page to the
real backend:

- Wire TipTap Editor component into PostCreateLayout content area
- Implement save/publish API calls (POST/PATCH to `/api/v1/contents`)
- Fetch content types from API instead of hardcoded maps
- Fetch spaces from API for the sidebar dropdown
- Wire custom field definitions from content type configuration
- Implement editor block permission admin CRUD routes
- Connect autosave to real API endpoint (debounced, not interval-based)
- Implement preview via SSR renderer (Section 15)
