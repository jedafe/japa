# Japa Comparison Engine (JCE) — Sub-spec 1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `subagent-driven-development` (recommended) or `executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Spec:** `docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md`

**Goal:** Implement the Japa Comparison Engine (JCE) Sub-spec 1 foundation: a standalone `apps/jce` SvelteKit surface, quick-vs-detailed comparison split, `/compare` picker, `/comparisons` directory, quick-to-detailed promotion, polymorphic `item_ref` resolver registry, and AI `depth` support — while migrating the `apps/main` comparison composer to the canonical `/api/v1/jce/*` API.

**Architecture:** Keep the existing `internal/comparison` package as the generic matrix engine and introduce `jce_service.go`, `jce_handlers.go`, `jce_store.go`, and a `resolver/` sub-package for JCE-specific orchestration. Add `comparison_content_metadata` for the quick/detailed split and `item_ref`/`config` JSONB columns for cross-domain items and type-specific settings. The new `apps/jce` app runs on port 3006 and is routed behind Nginx for `/compare*` and `/comparisons*`.

**Tech Stack:** Go 1.24+ (chi, pgx, sqlc), SvelteKit 2 + Svelte 5 Runes + Bits UI + Tailwind CSS v4, Bun, Valkey, PostgreSQL 18.

## Global Constraints

- Follow `AGENTS.md`, JEOS §30, and JEOS §103 conventions.
- All new Valkey keys must be added to `.jeos/conventions/valkey-keys-catalog.md` before use.
- All new/updated routes must be recorded in `.jeos/conventions/api-routes-catalog.md`.
- No hardcoded data in SvelteKit `load()` or components (Rule 16); all data comes from `/api/v1`.
- All UI text goes through `t()` and is added to `apps/jce/src/lib/i18n/en.json`.
- Use `bun` for all JS/TS tooling (Rule 18); do not use `npm`, `pnpm`, or `yarn`.
- JCE routes are gated by `feature.comparison` **AND** `feature.jce`.
- Quick comparisons are always `visibility=private`, `status=draft`, `seo_indexable=false`, `depth=quick`.
- Detailed comparisons created via promotion are separate `contents` rows with `depth=detailed`; the original quick comparison is not modified.

## Implementation Status

| Phase | Area | Status | Notes |
|-------|------|--------|-------|
| 0 | Preparation | `DONE` | `apps/jce` scaffolded, feature flags registered, and Docker Compose/Nginx wired. |
| 1 | Schema | `DONE` | `000540_comparison_content_metadata` and `000541_update_ai_comparison_create_schema` are already applied. Do **not** re-add `000540`; the next new migration is `000542`. |
| 2 | Backend JCE API | `DONE` | `/api/v1/jce/*` routes live in `internal/server/server.go`. Legacy `/api/v1/comparisons` route group removed. |
| 3 | Promotion | `DONE` | `POST /jce/comparisons/{id}/promote` implemented in `internal/comparison/jce_service.go`. |
| 3.5 | `apps/main` + `apps/admin` migration | `DONE` | `apps/main` composer and `apps/admin` comparison pages call `/jce/*`. |
| 4 | Resolver registry | `DONE` | `internal/comparison/resolver/` with university, course, space, visa, city, content, etc. |
| 5 | Permissions / visibility | `DONE` | Quick comparisons enforce owner/admin visibility and private defaults. |
| 6–9 | `apps/jce` frontend | `DONE` | Routes, API client, types, and picker pages exist. Some polish and i18n may remain. |
| 10 | Guest selection | `DONE` | `POST /jce/selections` and `POST /jce/comparisons/from-token` implemented with key `jc:compare:selection:{token}` (15m TTL). |
| 11 | AI `depth` | `DONE` | `internal/ai/tools_comparison.go` handles `depth` and `item_ref`; migration `000541` updated the tool schema. |
| 12–13 | Admin | `DONE` | `apps/admin` comparison pages and `comparisonAdminApi` call `/jce/*`; depth and promoted-from columns present. |
| 14 | Docker / Nginx | `DONE` | `jce` service added to `docker-compose.yml`; Nginx proxies `/compare*` and `/comparisons*` to `jce:3006`. |
| 15 | Tests / QA | `DONE` | `internal/comparison/jce_test.go` and `apps/jce/src/lib/api/jce.test.ts` added; full suite passes. |

## Critical Audit Corrections

- **Migration numbering:** The original plan called for `000540`, but `migrations/000540_comparison_content_metadata.up.sql` and `000541_update_ai_comparison_create_schema.up.sql` already exist. Migrations `000542` through `000550` also exist (including the JCE seed migrations `000545` and `000546`). The next *new* migration (if one is needed later) is `000551`.
- **Route removal and default flag:** The canonical JCE API is `/api/v1/jce/*`. The legacy `/api/v1/comparisons` route group has been removed from `internal/server/server.go`; the only comparison endpoints are now under `/api/v1/jce/*`. `feature.jce` defaults to `true` so `apps/main` redirects `/compare` and `/comparisons` to `apps/jce` without manual toggling.
- **Conventions path:** Update references from `docs/superpowers/conventions/` to `.jeos/conventions/`. The valkey key pattern `jc:compare:selection:{token}` is already recorded in `.jeos/conventions/valkey-keys-catalog.md`.
- **Preselect contract:** `?preselect` on `/compare` is a comma-separated list of entity slugs/IDs, not a full `item_ref` JSON object. The `+page.server.ts` resolves each via `GET /jce/entities/{type}/{id}`.
- **JCE origin:** Redirects from `apps/main` to `apps/jce` read `siteSettings['jce.origin']` and default to `http://localhost:3006`. Promotion from `apps/jce` to `apps/main` uses `siteSettings['jce.main_origin']`. Both keys are now documented in `.jeos/conventions/system-settings-catalog.md`.
- **Guest selection token:** Stored at `jc:compare:selection:{token}` with a 15-minute TTL, consumed by `POST /api/v1/jce/comparisons/from-token`, and already cataloged. The TTL is now configurable via `jce.selection_token_ttl_minutes` in `system_settings` (Rule 3) and falls back to 15 minutes.

---

## Phase 0 — Preparation

**Goal:** Update JEOS, create the feature-flag entries, and scaffold the new app.

**Status:** `PARTIAL` — the `apps/jce` SvelteKit app and feature flags are in place; Docker Compose and Nginx routing still need wiring (see Phase 14).

1. Review and update JEOS:
   - Section §30 (Generic Comparison System) for `comparison_content_metadata`, `comparison_items.item_ref`, `comparison_type_definitions.config`, and the quick/detailed split.
   - `.jeos/conventions/api-routes-catalog.md` to add the new `/api/v1/jce/*` routes and note that legacy `/api/v1/comparisons` routes are retained for backward compatibility (not removed).
2. Register feature flags in `internal/features/registry.go` (Section 42 — feature flags live in the `feature_flags` table, with registry defaults when no row exists):
   - `feature.jce` = `false` (default) — already added.
   - `feature.jce.nested_items` = `false` (default) — already added.
   - `feature.jce.ai_quick` = `false` (default) — already added.
3. Scaffold `apps/jce` SvelteKit SSR app — **already done**.
   - `apps/jce/package.json` (`dev`: port `3006`, name `@japaconnect/jce`).
   - `apps/jce/svelte.config.js`, `tsconfig.json`, `vite.config.ts`, `app.html`, `app.d.ts`, `hooks.server.ts`, `Dockerfile`.
   - Note: Tailwind v4 does not use a `tailwind.config.ts` in this repo; styling is configured via `vite.config.ts` and `src/app.css`.
4. Add `apps/jce` to the workspace and root `bun.lock` by running `bun install` — **already done**; `@japaconnect/jce` is present in the root `bun.lock` workspace list.
5. **Remaining:** Add `jce` service to `docker-compose.yml` and Nginx routing in `docker/nginx.conf` (see Phase 14).

**Verification:** `bun run check` in `apps/jce` passes. `docker compose up jce` starts on port 3006 once Phase 14 is complete.

---

## Phase 1 — Schema

**Goal:** Add the comparison metadata and `item_ref` support.

**Status:** `DONE` — migrations `000540` and `000541` are already applied; the next *new* migration is `000542`.

1. The schema changes below are already implemented in `migrations/000540_comparison_content_metadata.up.sql` and `000541_update_ai_comparison_create_schema.up.sql`. No new migration is required for this sub-spec; if additional schema changes are needed, use `000542`.
   ```sql
   -- up (already applied in 000540)
   CREATE TABLE IF NOT EXISTS comparison_content_metadata (
       content_id        uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
       tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
       depth             text NOT NULL CHECK (depth IN ('quick','detailed')) DEFAULT 'quick',
       promoted_from_id  uuid REFERENCES contents(id) ON DELETE SET NULL,
       promoted_at       timestamptz,
       smart_mode        text CHECK (smart_mode IN ('none','eligibility','cost','pathway','ai_advisor')) DEFAULT 'none',
       score_version     int NOT NULL DEFAULT 1,
       config            jsonb NOT NULL DEFAULT '{}',
       created_at        timestamptz NOT NULL DEFAULT now(),
       updated_at        timestamptz NOT NULL DEFAULT now()
   );

   CREATE INDEX IF NOT EXISTS idx_comparison_content_metadata_tenant_depth
       ON comparison_content_metadata(tenant_id, depth);

   CREATE INDEX IF NOT EXISTS idx_comparison_content_metadata_promoted_from
       ON comparison_content_metadata(promoted_from_id)
       WHERE promoted_from_id IS NOT NULL;

   ALTER TABLE comparison_type_definitions
       ADD COLUMN IF NOT EXISTS config jsonb NOT NULL DEFAULT '{}';

   ALTER TABLE comparison_items
       ADD COLUMN IF NOT EXISTS item_ref jsonb NOT NULL DEFAULT '{}';

   CREATE INDEX IF NOT EXISTS idx_comparison_items_item_ref
       ON comparison_items USING GIN(item_ref)
       WHERE item_ref <> '{}'::jsonb;
   ```
2. `seo_indexable` handling for quick comparisons is already implemented: `000540` backfills quick comparisons and sets `contents.seo_indexable = false` for them.
3. Existing `contents` with `content_type='comparison'` were backfilled into `comparison_content_metadata` using `CASE WHEN c.body IS NOT NULL AND length(trim(c.body)) > 0 THEN 'detailed' ELSE 'quick'`.
4. Run `make migrate` and `make sqlc-gen` to ensure the local environment is up to date.

**Verification:** `make test-integration` passes for migrations. `make sqlc-gen` produces `db/gen` methods for the metadata table.

---

## Phase 2 — Backend: canonical JCE comparison API

**Goal:** Make `/api/v1/jce/*` the canonical comparison API while retaining the legacy `/api/v1/comparisons` routes for backward compatibility.

**Status:** `DONE` — the canonical JCE service, handlers, store, and routes are implemented. Legacy routes are not removed.

1. Add types and SQL queries in `internal/db/queries/comparison.sql`:
   - `InsertComparisonContentMetadata`
   - `GetComparisonContentMetadata`
   - `UpdateComparisonContentMetadata`
   - `ListComparisonsWithFilters` (supports `q`, `sort`, `filter_depth`, `filter_type`, `filter_status`, `mine`, `page`, `page_size`).
   - Update `CreateComparisonItem` to accept `item_ref`.
   - Update `UpdateComparisonItem` to accept `item_ref`.
   - Update `GetComparisonItems` / `ListComparisonItems` to return `item_ref`.
   - Update comparison type definition queries (`CreateComparisonType`, `UpdateComparisonType`, `GetComparisonType*`, `ListComparisonTypes*`) to include `config`.
2. Regenerate `internal/db/gen/comparison.sql.go` via `make sqlc-gen`.
3. Extend `internal/comparison/comparison.go`:
   - Add `Depth` and `Metadata` to comparison models.
   - Add `ItemRef` struct (`Type`, `ID`, `TitleCache`, `ContentType` for nested content, `Metadata`).
4. Extend `internal/comparison/store.go`:
   - Methods for metadata CRUD.
5. Wire a Valkey client into `internal/comparison/service.go` (or `store.go`) for guest selection tokens and resolver caches if needed.
6. Extend `internal/comparison/service.go`:
   - Define `CreateQuickComparisonInput` with `TypeSlug`, `Title` (optional), `ItemRefs []json.RawMessage` (or `[]ItemRef`), and `Dimensions []string` (optional overrides).
   - `CreateQuickComparison(ctx, tenantID, userID, CreateQuickComparisonInput)` — always sets `depth='quick'`, `visibility='private'`, `seo_indexable=false`. `content_hash` is handled by the existing `contentSvc.CreateContentWithQueries`: empty body → `content_hash` NULL (safe under `idx_contents_tenant_content_hash` which has `WHERE content_hash IS NOT NULL`).
   - `CreateDetailedComparison(ctx, tenantID, userID, ...)` — creates a detailed comparison draft (used by `apps/main` composer and AI agent when `depth=detailed`).
   - `GetComparison(ctx, tenantID, userID, slug)` — enforces visibility: quick is author/edit_any only; detailed follows `visibility`/`status`. Returns `ErrNotFound` (404) for unauthorized quick.
   - `GetComparisonMatrix(ctx, tenantID, userID, slug)` — same visibility as `GetComparison`.
   - `ListComparisonsWithFilters(ctx, tenantID, userID, ListComparisonsInput)` — public list returns published detailed and non-deleted; `mine=true` returns the caller's own (non-deleted) comparisons.
   - `UpdateComparison(ctx, tenantID, userID, contentID, ...)` — title, body, visibility, status, and metadata updates for quick or detailed.
   - `UpdateQuickComparison(ctx, tenantID, userID, quickContentID, ...)` — quick-only edits; still `depth='quick'`.
   - `CreateQuickComparisonFromToken(ctx, tenantID, userID, token)` — loads the guest selection from Valkey, deletes the key, and calls `CreateQuickComparison`.
   - `DeleteComparison(ctx, tenantID, userID, contentID)` — soft delete (`deleted_at=now()`).
7. Extend `internal/comparison/handlers.go` with the full canonical API:
   - Type routes:
     - `GET /api/v1/jce/comparison-types`
     - `GET /api/v1/jce/comparison-types/{slug}`
     - `POST /api/v1/jce/comparison-types`
     - `PATCH /api/v1/jce/comparison-types/{id}`
     - `DELETE /api/v1/jce/comparison-types/{id}`
     - `GET /api/v1/jce/comparison-types/{id}/access`
     - `PUT /api/v1/jce/comparison-types/{id}/access`
   - `GET /api/v1/jce/comparisons/dimension-types`
   - `POST /api/v1/jce/comparisons/auto-dimensions`
   - `GET /api/v1/jce/comparisons` — list.
   - `POST /api/v1/jce/comparisons` — create quick or detailed (driven by `depth`).
   - `GET /api/v1/jce/comparisons/{id|slug}` — detail.
   - `GET /api/v1/jce/comparisons/{id|slug}/matrix` — matrix.
   - `PATCH /api/v1/jce/comparisons/{id}` — title/body/visibility/status/metadata updates.
   - `DELETE /api/v1/jce/comparisons/{id}` — soft delete.
   - Matrix-edit routes:
     - `POST /api/v1/jce/comparisons/{id}/items`
     - `PATCH /api/v1/jce/comparisons/items/{item_id}`
     - `POST /api/v1/jce/comparisons/{id}/items/reorder`
     - `POST /api/v1/jce/comparisons/{id}/dimensions`
     - `PATCH /api/v1/jce/comparisons/dimensions/{dimension_id}`
     - `DELETE /api/v1/jce/comparisons/dimensions/{dimension_id}`
     - `PUT /api/v1/jce/comparisons/dimensions/{dimension_id}/values`
   - `POST /api/v1/jce/comparisons/from-token` — create from guest selection.
8. In `internal/server/server.go`:
   - Add the `/api/v1/jce/comparison*` route group under `feature.comparison` + `feature.jce`.
   - Add `GET /jce/comparisons/admin` for the admin comparison list and `GET/PUT /jce/comparison-types/{id}/access` for type access control.
   - Remove the legacy `/api/v1/comparisons` route group once `apps/main` and `apps/admin` have migrated.

**Verification:** `go test ./internal/comparison/...`, `go build ./...`, `make vet`.

---

## Phase 3 — Backend: promotion to detailed

**Goal:** Implement quick-to-detailed promotion.

**Status:** `DONE` — implemented in `internal/comparison/jce_service.go` and wired in `internal/server/server.go`.

1. Extend `internal/comparison/service.go`:
   - `PromoteToDetailed(ctx, tenantID, userID, quickContentID)`:
     - Verify `depth='quick'` and ownership/elevated permissions.
     - Run the entire promotion inside a database transaction.
     - Create a new `contents` row with `content_type='comparison'`, `status='draft'`, `depth='detailed'`, `visibility='private'`, `seo_indexable=false`. `content_hash` is handled by `contentSvc.CreateContentWithQueries` (empty body → NULL).
     - Clone `comparison_items`, `comparison_dimensions`, and `comparison_values` to the new content:
       - Insert new items and dimensions with the new `content_id`, building a map from old ID → new ID.
       - Insert new values using the new `dimension_id` and `item_id` pairs, preserving `value`, `value_rating`, `value_boolean`, and `metadata`.
     - Set `comparison_content_metadata.promoted_from_id` to the quick content and `promoted_at=now()`.
     - Return new `{id, slug}`.
2. Add handler `POST /api/v1/jce/comparisons/{id}/promote`.
3. Wire route.
4. Verify `apps/main` `/create?type=comparison&draft={id}` can load the new detailed draft and pre-fill the composer.

**Verification:** Integration test: `POST .../promote` → `GET /jce/comparisons/{new_slug}` (as author) → `GET /api/v1/jce/comparisons/{new_slug}/matrix` returns the cloned matrix.

---

## Phase 3.5 — Backend + frontend: migrate `apps/main` composer to `/api/v1/jce/*`

**Goal:** Move the existing `apps/main` comparison composer and wizards to the canonical JCE API. The legacy `/api/v1/comparisons` routes are retained for backward compatibility; the Admin panel still needs migration.

**Status:** `PARTIAL` — `apps/main` and `apps/jce` call the new endpoints; `apps/admin` still uses the legacy routes.

1. Update `apps/main/src/lib/api/create.ts`:
   - Replace only comparison-related calls (`createComparison`, `patchComparison`, `addComparisonItem`, etc.) from `/comparisons` to `/jce/comparisons` and `/jce/comparison-types`.
   - Add `depth` and `item_ref` fields where applicable.
   - Leave blog, guide, story, and other post-type endpoints unchanged.
2. Update `apps/main/src/routes/(app)/create/+page.server.ts`:
   - When `type=comparison` or the restored draft is `content_type='comparison'`, load comparison types from `GET /api/v1/jce/comparison-types`.
   - Load dimension types from `GET /api/v1/jce/comparisons/dimension-types` only for comparison drafts.
   - Restore comparison draft from `GET /api/v1/jce/comparisons/{id}` and `GET /api/v1/jce/comparisons/{id}/matrix` only for comparison drafts.
3. Update `apps/main/src/lib/components/create/editors/ComparisonBuilder.svelte` (used only for `content_type='comparison'`):
   - Use the new `item_ref` fields when adding/updating items.
   - Update API calls to `/jce/comparisons/{id}/items`, `/jce/comparisons/{id}/dimensions`, `/jce/comparisons/dimensions/{id}/values`.
4. Update `apps/main/src/routes/(app)/compare/universities/+page.server.ts` and `+page.svelte`:
   - Option A (recommended): redirect to `apps/jce /compare?type=universities&preselect=...`.
   - Option B: continue creating via `POST /api/v1/jce/comparisons` and redirect to `apps/jce /compare/[slug]`.
5. Update `apps/main/src/routes/(app)/compare/new/+page.server.ts`:
   - Redirect to `apps/jce /compare` or use `POST /api/v1/jce/comparisons`.
6. The legacy `apps/main` `/compare` and `/compare/[slug]` routes are redirected to `apps/jce` via `apps/main/src/routes/(app)/compare/+layout.server.ts` when `feature.jce` is enabled.
7. **Remaining (Admin):** Update `apps/admin/src/routes/comparisons/+page.svelte` and `comparison-types/+page.svelte` to use `/api/v1/jce/*` endpoints. The admin pages currently call `/comparisons` and `/comparisons/types`.

**Verification:** `bun run check` and `bun run build` in `apps/main` and `apps/admin`. Manual: composer creates and publishes a detailed comparison; promotion from `apps/jce` opens the composer with the draft loaded.

---

## Phase 4 — Backend: `item_ref` resolver registry and entity search

**Goal:** Make items resolvable and searchable across domain tools.

**Status:** `DONE` — `internal/comparison/resolver/` and JCE entity search/resolve handlers are implemented and wired.

1. Create `internal/comparison/resolver/resolver.go`:
   - `ItemRefResolver` interface with `EntityType()`, `Resolve(...)`, `Search(...)`.
   - `ResolvedItem` struct.
   - `Registry` map of `type` → resolver.
   - `Register(entityType, resolver)` and `Resolve/Search` helpers.
   - This package is introduced in `internal/comparison` for Sub-spec 1; it will be moved/extracted to `internal/jce` in Sub-spec 2/3 when the intelligence layer grows.
2. Create `internal/comparison/resolver/space.go`, `university.go`, `course.go`, `visa_route.go`, `col_city.go`, `scholarship.go`, `remittance.go`, `languagetest.go`, `organization.go`, `job.go`, `travel.go`, `mentor.go`, `content.go`.
   - Each wraps the existing `internal/{tool}` service/store or uses a thin SQL query where no search method exists.
   - Implement `Resolve(id)` and `Search(q, filters)`.
3. Add JCE service methods in `internal/comparison/jce_service.go`:
   - `SearchJCEEntities(ctx, entityType, query, filters, tenantID, limit)`.
   - `ResolveJCEEntity(ctx, entityType, idOrSlug, tenantID)`.
4. Add handlers:
   - `GET /api/v1/jce/entities/{type}` (search)
   - `GET /api/v1/jce/entities/{type}/{id}` (resolve one)
5. Register resolvers where `comparison.NewService` is constructed (`internal/server/server.go:665` area):
   - Instantiate each resolver, passing the relevant tool service/store.
   - Call `comparisonService.Resolver().Register(...)` (or set a resolver field on the service).

**Verification:** `go test ./internal/comparison/resolver/...`, `go test ./internal/comparison/...`. Spot-check each resolver with a representative ID.

---

## Phase 5 — Backend: list filters, permissions, and visibility

**Goal:** Enforce public vs private, author vs admin, and guest behavior.

**Status:** `DONE` — visibility, ownership, and feature/permission gating are implemented in `internal/comparison/jce_service.go` and `jce_handlers.go`.

1. Ensure `ListComparisonsWithFilters` returns:
   - Public default: `depth='detailed'`, `status='published'`, `visibility='public'`.
   - `mine=true`: caller's own quick and detailed, filtered by `depth` and `status`.
2. Ensure `GetComparison` returns 404 for quick comparisons unless caller is author or has `comparison.edit_any`.
3. Ensure `POST /api/v1/jce/comparisons` requires `comparison.create`.
4. Add feature-flag gating for all `/jce/` routes.
5. Add tenant isolation to all queries and handlers.

**Verification:** Unit tests for each visibility scenario. `go test ./internal/comparison/...`.

---

## Phase 6 — Frontend: `apps/jce` scaffold and shared components

**Goal:** Build the shared UI layer for `apps/jce`.

**Status:** `DONE` — the app scaffold, API client, types, and core routes exist.

1. `apps/jce/src/lib/api/jce.ts` — API client (uses `apps/jce/src/lib/api/client.ts`).
2. `apps/jce/src/lib/types/jce.ts` — local TS types for comparison, matrix, type, item.
3. Components live inline in `+page.svelte` files; a future refactor can extract them to `apps/jce/src/lib/components/compare/` and `apps/jce/src/lib/components/comparisons/`.
4. Layout and i18n setup — layout exists at `apps/jce/src/routes/+layout.svelte`; i18n keys should be added to `apps/jce/src/lib/i18n/en.json` and used through `t()`.

**Verification:** `bun run check` in `apps/jce` passes.

---

## Phase 7 — Frontend: `/compare` item-selection tool

**Goal:** The public `/compare` page.

**Status:** `DONE` — implemented at `apps/jce/src/routes/compare/+page.server.ts` and `+page.svelte`.

1. `apps/jce/src/routes/compare/+page.server.ts`:
   - Load comparison types via `GET /api/v1/jce/comparison-types`.
   - If `?type` query param, pre-select that type.
   - If `?preselect` is present, treat it as a comma-separated list of entity slugs/IDs, resolve each via `GET /jce/entities/{type}/{id}`, and pass to client.
   - If `?compare_token` and user is authenticated, call `POST /api/v1/jce/comparisons/from-token` server-side and redirect to `/compare/[slug]`.
   - Pass `mainOrigin` from `siteSettings['jce.main_origin']` (default `http://localhost:3000`) for login redirects.
2. `apps/jce/src/routes/compare/+page.svelte`:
   - Type selector, searchable item list, multi-select up to `max_item_count` from type `config`.
   - **Compare** button.
     - Guest: store selected `item_refs` via `POST /api/v1/jce/selections`, then redirect to `{mainOrigin}/login?redirect=/compare...`.
     - Auth: `POST /api/v1/jce/comparisons`, redirect to `/compare/[slug]`.
3. **Polish:** Extract reusable components to `apps/jce/src/lib/components/compare/` and wire all text through `t()`.

**Verification:** Manual: visit `/compare`, select 2 universities, click **Compare** as guest → login redirect. Log in → quick comparison created and redirected to `/compare/[slug]`.

---

## Phase 8 — Frontend: `/compare/[slug]` quick preview

**Goal:** Private quick comparison view with edit/delete/promote.

**Status:** `DONE` — implemented at `apps/jce/src/routes/compare/[slug]/+page.server.ts` and `+page.svelte`.

1. `apps/jce/src/routes/compare/[slug]/+page.server.ts`:
   - Load `GET /api/v1/jce/comparisons/{slug}` and `GET /api/v1/jce/comparisons/{slug}/matrix`.
   - 404 for non-owners and non-admins.
2. `apps/jce/src/routes/compare/[slug]/+page.svelte`:
   - Render the matrix.
   - Show **Edit**, **Delete**, **Promote** actions.
   - Promote redirects to `{mainOrigin}/create?type=comparison&draft={new_id}` where `mainOrigin` is `siteSettings['jce.main_origin']`.

**Verification:** Create quick comparison, view at `/compare/[slug]`, promote. Verify redirected to `apps/main` draft with matrix pre-filled.

---

## Phase 9 — Frontend: `/comparisons` public directory

**Goal:** Public detailed comparison list and detail.

**Status:** `DONE` — implemented at `apps/jce/src/routes/comparisons/+page.server.ts` and `+page.svelte`.

1. `apps/jce/src/routes/comparisons/+page.server.ts`:
   - `GET /api/v1/jce/comparisons` (public, published detailed) and `?mine=true` for the authenticated tab.
2. `apps/jce/src/routes/comparisons/+page.svelte`:
   - List and tab views.
3. `apps/jce/src/routes/comparisons/[slug]/+page.server.ts`:
   - Load detailed comparison and matrix.
4. `apps/jce/src/routes/comparisons/[slug]/+page.svelte`:
   - Render body and matrix; share actions for public comparisons.
5. **Polish:** Extract list/card components to `apps/jce/src/lib/components/comparisons/` and wire UI text through `t()`.

**Verification:** `bun run check` and manual spot checks: published detailed comparison appears on `/comparisons`; private quick does not.

---

## Phase 10 — Frontend and backend: guest selection preservation

**Goal:** Guests can select items and continue after login.

**Status:** `DONE` — backend handlers and frontend token flow are implemented.

1. On `/compare`, when a guest clicks **Compare**, the frontend calls `POST /api/v1/jce/selections` and the backend stores a short-lived Valkey key:
   - Key: `jc:compare:selection:{token}` with a cryptographically random token, TTL 15 min.
   - Value: `{ "type_slug": "...", "item_refs": [...], "created_at": "..." }`.
   - Token is passed in the redirect URL: `{mainOrigin}/login?redirect=/compare%3Fcompare_token%3D{token}`.
   - The pattern is already recorded in `.jeos/conventions/valkey-keys-catalog.md`.
2. After login, the user is redirected to `/compare?compare_token={token}`. `apps/jce/src/routes/compare/+page.server.ts` calls `POST /api/v1/jce/comparisons/from-token`, which loads the selection from Valkey, deletes the key, creates the quick comparison, and redirects to `/compare/[slug]`.
3. **Future:** If Valkey is unavailable, add a compact base64-json fallback in the redirect query (limit to a few items).

**Verification:** E2E test: guest selects items → login → redirected back → quick comparison created.

---

## Phase 11 — Tool integrations

**Goal:** Wire existing tool pages into the JCE picker.

**Status:** `DONE` — the `?preselect` contract and `apps/main` redirect layout are implemented; P0 compare buttons for university, visa routes, mentors, and cost-of-living are wired to `apps/jce`.

1. **P0 integrations (done):**
   - University detail: **Compare** button links to `{jceOrigin}/compare?type=universities&source=university&preselect={slug}` (backend redirect and fallback updated).
   - Visa routes: **Compare** or **Open in JCE** link to `{jceOrigin}/compare?type=visa-types&preselect={slug}`.
   - Cost of living: **Compare in JCE** link already present; canonical `?type=cost_of_living&preselect={city-slug}` is used.
   - Mentors list: compare link now uses `{jceOrigin}/compare?type=mentors&preselect={ids}`.
   - `apps/main/src/routes/(app)/compare/+layout.server.ts` redirects legacy `/compare/universities` and `/compare/mentors` to `apps/jce` with `?type=` and `?preselect=`.
2. P1 integrations (as bandwidth allows):
   - Scholarships, remittance, language tests, organizations, jobs, travel, mentors.
3. In `apps/jce` `/compare` page, `?preselect` is supported as a comma-separated list of entity slugs/IDs.
4. The Valkey key pattern `jc:compare:selection:{token}` is already recorded in `.jeos/conventions/valkey-keys-catalog.md`.

**Verification:** For each P0 tool, click **Compare** and confirm the item is pre-selected on `/compare` and the quick comparison is created correctly.

---

## Phase 12 — AI `depth` support

**Goal:** AI can create quick and detailed comparisons.

**Status:** `DONE` — `internal/ai/tools_comparison.go` handles `depth` and `item_ref`; migration `000541_update_ai_comparison_create_schema` updated the tool schema; `feature.jce.ai_quick` is registered.

1. `internal/ai/tools_comparison.go`:
   - `ComparisonCreateArgs` includes `Depth` and `ItemRefs`.
   - `ComparisonCreateHandler` respects `depth`; quick comparisons are forced to `visibility='private'` and non-indexable.
2. The AI tool schema was updated via `migrations/000541_update_ai_comparison_create_schema.up.sql` (do not edit `internal/ai/tools_register.go` directly; the schema lives in `ai_tools.config`).
3. `feature.jce.ai_quick` is registered in `internal/features/registry.go` (default `false`).

**Verification:** `go test ./internal/ai/...` passes. Manual: ask agent to create a quick comparison and verify it is private at `/compare/[slug]`.

---

## Phase 13 — Admin updates

**Goal:** Admin can configure comparison type `config` and see depth.

**Status:** `DONE` — `apps/admin` comparison pages and `comparisonAdminApi` now call the JCE endpoints.

1. `apps/admin/src/routes/comparison-types/+page.svelte`:
   - `config` is part of the create/update payload and is editable via the form.
   - API calls go through `comparisonAdminApi` to `GET/POST/PATCH/DELETE /api/v1/jce/comparison-types`.
2. `apps/admin/src/routes/comparisons/+page.svelte`:
   - List call uses `GET /api/v1/jce/comparisons/admin`.
   - Depth filter and "Promoted from" column are present.
   - Links to `apps/jce`: quick → `/compare/{slug}`, detailed → `/comparisons/{slug}`.

**Verification:** `bun run check` and `bun run build` in `apps/admin`.

---

## Phase 14 — Routing, redirects, and Docker

**Goal:** Serve `/compare` and `/comparisons` from `apps/jce`.

**Status:** `DONE`.

1. `apps/jce/Dockerfile` exists and is modeled on `apps/tools/Dockerfile`.
2. `jce` service added to `docker-compose.yml` on port `3006` with `PRIVATE_API_BASE_URL=http://go-api:8080`, `PUBLIC_JC_API_URL=/api/v1`, and `PUBLIC_MAIN_ORIGIN=http://localhost:3000`.
3. `docker/nginx.conf`:
   - `upstream japa_jce { server jce:3006; }`.
   - `/compare*` and `/comparisons*` proxied to `http://japa_jce`.
   - Dedicated `jce.japaconnect.com` server block.
4. `apps/main/src/routes/(app)/compare/+layout.server.ts` redirects legacy `/compare/*` routes to `siteSettings['jce.origin']`.
5. `feature.jce` default changed to `true` so the JCE surface is reachable without a manual feature-flag toggle.
6. Verified `docker compose config` and Nginx config syntax.

**Verification:** `docker compose up` and `bun run dev` in both apps. `/compare` resolves to `apps/jce`.

---

## Phase 15 — Testing and QA

**Goal:** All tests pass and the feature is ready for release.

**Status:** `DONE` — JCE-specific backend and frontend tests added; full suite passes.

1. **Backend tests**
   - `internal/comparison/jce_test.go`: JCE depth constants, `selectionTokenKey`, `marshalItemRef`, and `parseOptionalUUID`.
   - Resolver tests exist in `internal/comparison/resolver/`.
2. **Frontend tests**
   - `apps/jce/src/lib/api/jce.test.ts`: API client calls to `/jce/*` and quick comparison defaults.
3. **Integration tests**
   - Manual: quick creation → promotion → publish.
   - Cross-domain: `countries` comparison with `universities` as an item.
   - Guest selection preservation across login.
4. **Manual QA checklist**
   - Quick comparison is private and not indexed.
   - `/comparisons` only shows published public detailed.
   - `/compare` "My comparisons" shows quick; `/comparisons` "My comparisons" shows detailed.
   - Tool-page **Compare** buttons pre-select items.

**Verification:** `make check`, `bun run check` and `bun run build` in `apps/jce`, `bun run build` in `apps/main` and `apps/admin`.

---

## Order of work

**Recommended sequence:**

All phases are complete. Remaining polish (component extraction, i18n, tool **Compare** buttons) can be done in a follow-up sub-spec.

**Exit criteria:**

- `/compare` creates private quick comparisons.
- `/compare/[slug]` shows quick preview with promote/edit/delete.
- `/comparisons` lists public detailed comparisons.
- Promotion clones quick to detailed and opens `apps/main` composer.
- `item_ref` resolvers support at least university, course, space, visa_route, col_city, scholarship, remittance, languagetest, organization, job, travel, mentor, content.
- Tool-page **Compare** buttons work for P0 tools.
- Canonical `/api/v1/jce/*` routes are wired and gated by `feature.comparison` + `feature.jce`; `apps/main` composer, `apps/admin`, and `apps/jce` all call `/api/v1/jce/*`.
- Legacy `/api/v1/comparisons` route group removed from `internal/server/server.go`.
- `apps/admin` comparison pages are migrated to `/api/v1/jce/*`.
- `docker-compose.yml` and `docker/nginx.conf` route `/compare*` and `/comparisons*` to the `jce` service.
- JCE backend and frontend tests pass.
- `feature.jce` is enabled by default; `apps/main` redirects `/compare` and `/comparisons` to `apps/jce`.