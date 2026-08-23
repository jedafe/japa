# Japa Comparison Engine (JCE) — Sub-spec 1 Implementation Plan

**Spec:** `docs/superpowers/specs/2026-08-22-jce-subspec-1-design.md`

This plan implements the JCE foundation:
- New `apps/jce` SvelteKit app.
- Quick (private) vs detailed (publishable) comparison split.
- `/compare` item-selection tool and `/comparisons` public directory.
- Quick-to-detailed promotion into `apps/main` `/create?type=comparison&draft={id}`.
- Polymorphic `item_ref` and resolver registry for cross-domain/tool integration.
- AI `comparison.create` supports `depth`.
- **Migrate existing `apps/main` composer and comparison wizards to the new `/api/v1/jce/*` comparison API, then remove the old `/api/v1/comparisons` routes.**

Out of scope for this plan: JapaScore, eligibility, user profiles, data ingestion, AI decision agent — those are Sub-specs 2–4.

---

## Phase 0 — Preparation

**Goal:** Update JEOS, create the feature-flag entries, and scaffold the new app.

1. Review and update JEOS:
   - Section §30 (Generic Comparison System) for `comparison_content_metadata`, `comparison_items.item_ref`, `comparison_type_definitions.config`, and the quick/detailed split.
   - `conventions/api-routes-catalog.md` to add the new `/api/v1/jce/*` routes and mark old `/api/v1/comparisons` routes as removed/deprecated in this slice.
2. Register feature flags in `internal/features/registry.go` (Section 42 — feature flags live in the `feature_flags` table, with registry defaults when no row exists):
   - `feature.jce` = `false` (default).
   - `feature.jce.nested_items` = `false` (default).
   - `feature.jce.ai_quick` = `false` (default).
   - Optionally seed `feature_flags` rows for these keys in a migration if the project convention is to seed them.
3. Scaffold `apps/jce` SvelteKit SSR app.
   - Copy base config from `apps/tools` (SvelteKit SSR, adapter-node, Tailwind 4, port pattern).
   - `package.json` with `dev` on port `3006`, named `@japaconnect/jce`.
   - `svelte.config.js`, `tsconfig.json`, `vite.config.ts`, `tailwind.config.ts`, `app.html`, `app.d.ts`, `hooks.server.ts`, `Dockerfile`.
   - Add to `docker-compose.yml` under a `jce` service.
4. Add `apps/jce` to the workspace and root `bun.lock` by running `bun install`.

**Verification:** `bun run check` in `apps/jce` passes (with the starter page). `docker compose up jce` starts on port 3006.

---

## Phase 1 — Schema

**Goal:** Add the comparison metadata and `item_ref` support.

1. Create migration `000540_comparison_content_metadata.up.sql`/`.down.sql` (next after `000539`).
   ```sql
   -- up
   CREATE TABLE comparison_content_metadata (
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

   CREATE INDEX idx_comparison_content_metadata_tenant_depth
       ON comparison_content_metadata(tenant_id, depth);

   CREATE INDEX idx_comparison_content_metadata_promoted_from
       ON comparison_content_metadata(promoted_from_id)
       WHERE promoted_from_id IS NOT NULL;

   -- Add config column to comparison_type_definitions
   ALTER TABLE comparison_type_definitions
       ADD COLUMN config jsonb NOT NULL DEFAULT '{}';

   -- Add item_ref to comparison_items
   ALTER TABLE comparison_items
       ADD COLUMN item_ref jsonb NOT NULL DEFAULT '{}';

   CREATE INDEX idx_comparison_items_item_ref
       ON comparison_items USING GIN(item_ref)
       WHERE item_ref <> '{}'::jsonb;
   ```
   Down migration:
   ```sql
   DROP INDEX IF EXISTS idx_comparison_items_item_ref;
   ALTER TABLE comparison_items DROP COLUMN IF EXISTS item_ref;
   ALTER TABLE comparison_type_definitions DROP COLUMN IF EXISTS config;
   DROP INDEX IF EXISTS idx_comparison_content_metadata_promoted_from;
   DROP INDEX IF EXISTS idx_comparison_content_metadata_tenant_depth;
   DROP TABLE IF EXISTS comparison_content_metadata;
   ```
2. If needed, add `seo_indexable` handling for quick comparisons (the column already exists on `contents`; logic will set it to `false`).
3. Backfill existing `contents` with `content_type='comparison'` into `comparison_content_metadata` with `depth='quick'` if there is no body, or `depth='detailed'` if there is a body.
4. Run `make migrate` and `make sqlc-gen`.

**Verification:** `make test-integration` passes for migrations. `make sqlc-gen` produces new `db/gen` methods for the metadata table.

---

## Phase 2 — Backend: canonical JCE comparison API

**Goal:** Make `/api/v1/jce/*` the single comparison API and remove the old `/api/v1/comparisons` path group.

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
   - Remove the old `/api/v1/comparisons` route group.
   - Add the new `/api/v1/jce/comparison*` route group under `feature.comparison` + `feature.jce`.
   - Ensure all existing handler methods work with the new `depth`/`item_ref` fields so the same handlers serve quick and detailed comparisons.

**Verification:** `go test ./internal/comparison/...`, `go build ./...`, `make vet`.

---

## Phase 3 — Backend: promotion to detailed

**Goal:** Implement quick-to-detailed promotion.

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

**Goal:** Move the existing `apps/main` comparison composer and wizards to the canonical JCE API so the old `/api/v1/comparisons` routes can be removed.

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
6. Remove or redirect `apps/main/src/routes/(app)/compare/[slug]/+page.svelte` and `+page.server.ts`:
   - These are superseded by `apps/jce /compare/[slug]`.
7. Remove or redirect `apps/main/src/routes/(app)/compare/+page.svelte`:
   - Superseded by `apps/jce /comparisons`.
8. Update `apps/admin/src/routes/comparisons/+page.svelte` and `comparison-types/+page.svelte` to use `/api/v1/jce/*`.

**Verification:** `bun run check` and `bun run build` in `apps/main` and `apps/admin`. Manual: composer creates and publishes a detailed comparison; promotion from `apps/jce` opens the composer with the draft loaded.

---

## Phase 4 — Backend: `item_ref` resolver registry and entity search

**Goal:** Make items resolvable and searchable across domain tools.

1. Create `internal/comparison/resolver/resolver.go`:
   - `ItemRefResolver` interface with `EntityType()`, `Resolve(...)`, `Search(...)`.
   - `ResolvedItem` struct.
   - `Registry` map of `type` → resolver.
   - `Register(entityType, resolver)` and `Resolve/Search` helpers.
   - This package is introduced in `internal/comparison` for Sub-spec 1; it will be moved/extracted to `internal/jce` in Sub-spec 2/3 when the intelligence layer grows.
2. Create `internal/comparison/resolver/space.go`, `university.go`, `course.go`, `visa_route.go`, `col_city.go`, `scholarship.go`, `remittance.go`, `languagetest.go`, `organization.go`, `job.go`, `travel.go`, `mentor.go`, `content.go`.
   - Each wraps the existing `internal/{tool}` service/store or uses a thin SQL query where no search method exists.
   - Implement `Resolve(id)` and `Search(q, filters)`.
3. Add `internal/comparison/service.go` methods:
   - `ResolveItemRef(ctx, tenantID, itemRef)`.
   - `SearchEntities(ctx, tenantID, entityType, q, filters, limit, offset)`.
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

1. `apps/jce/src/lib/api.ts` — API client mirroring `apps/main` pattern.
2. `apps/jce/src/lib/types/comparison.ts` — local TS types for comparison, matrix, type, item.
3. Shared components in `apps/jce/src/lib/components/shared/`:
   - `TypeSelector.svelte`
   - `ItemPicker.svelte`
   - `DimensionEditor.svelte`
   - `ValueMatrix.svelte`
   - `ComparisonMatrix.svelte`
4. Layout and i18n setup.

**Verification:** `bun run check` in `apps/jce` passes.

---

## Phase 7 — Frontend: `/compare` item-selection tool

**Goal:** The public `/compare` page.

1. `apps/jce/src/routes/(app)/compare/+page.server.ts`:
   - Load comparison types.
   - If `?type` query param, load entities for that type.
   - If `?preselect`, pass to client.
   - If `?compare_token` and user is authenticated, call a server-side endpoint to create the quick comparison from the stored selection and redirect to `/compare/[slug]`.
2. `apps/jce/src/routes/(app)/compare/+page.svelte`:
   - Type selector (tabs or dropdown).
   - Searchable item list (`CompareItemGrid`/`CompareItemPicker`).
   - Multi-select with 2–max counter.
   - **Compare** button.
     - Guest: store selected `item_refs` in Valkey (`jc:compare:selection:{token}`) with a 15-min TTL, then redirect to `/login?redirect=/compare&compare_token={token}`.
     - Auth: `POST /api/v1/jce/comparisons`, redirect to `/compare/[slug]`.
3. `apps/jce/src/lib/components/compare/CompareItemGrid.svelte`
4. `apps/jce/src/lib/components/compare/CompareSelectedItems.svelte`
5. `apps/jce/src/lib/components/compare/CompareTypeSelector.svelte`

**Verification:** Manual: visit `/compare`, select 2 universities, click **Compare** as guest → login redirect. Log in → quick comparison created and redirected to `/compare/[slug]`.

---

## Phase 8 — Frontend: `/compare/[slug]` quick preview

**Goal:** Private quick comparison view with edit/delete/promote.

1. `apps/jce/src/routes/(app)/compare/[slug]/+page.server.ts`:
   - Load `GET /api/v1/jce/comparisons/{slug}` and `/matrix`.
   - 404 for non-owners.
2. `apps/jce/src/routes/(app)/compare/[slug]/+page.svelte`:
   - Render `ComparisonMatrix`.
   - Show **Edit**, **Delete**, **Promote** actions.
   - Inline editing for title, dimensions, values.
3. `apps/jce/src/lib/components/detail/QuickActions.svelte`
4. `apps/jce/src/lib/components/detail/PromoteButton.svelte`:
   - Confirmation modal.
   - Calls `POST .../promote`.
   - Redirects to `{MAIN_ORIGIN}/create?type=comparison&draft={new_id}` (env-driven, e.g. `PUBLIC_MAIN_ORIGIN`).

**Verification:** Create quick comparison, view at `/compare/[slug]`, promote. Verify redirected to `apps/main` draft with matrix pre-filled.

---

## Phase 9 — Frontend: `/comparisons` public directory

**Goal:** Public detailed comparison list and detail.

1. `apps/jce/src/routes/(app)/comparisons/+page.server.ts`:
   - `GET /api/v1/jce/comparisons` (public, published detailed).
2. `apps/jce/src/routes/(app)/comparisons/+page.svelte`:
   - Search input, sort, type filter, pagination, page-size selector.
   - Card grid.
   - **My comparisons** tab for authenticated users (`?mine=true`, depth=detailed only).
3. `apps/jce/src/routes/(app)/comparisons/[slug]/+page.server.ts`:
   - Load detailed comparison and matrix.
4. `apps/jce/src/routes/(app)/comparisons/[slug]/+page.svelte`:
   - Render body and matrix.
   - Share actions for public comparisons.
5. Components in `apps/jce/src/lib/components/comparisons/`:
   - `ComparisonList.svelte`
   - `ComparisonListFilters.svelte`
   - `ComparisonPagination.svelte`
   - `ComparisonCard.svelte`
   - `MyDetailedComparisons.svelte`

**Verification:** `bun run check` and manual spot checks: published detailed comparison appears on `/comparisons`; private quick does not.

---

## Phase 10 — Frontend and backend: guest selection preservation

**Goal:** Guests can select items and continue after login.

1. On `/compare`, when a guest clicks **Compare**, serialize the selected comparison type and `item_refs` into a short-lived Valkey key:
   - Key: `jc:compare:selection:{token}` with a random token, TTL 15 min.
   - Value: `{ "type_slug": "...", "item_refs": [...], "created_at": "..." }`.
   - Token is passed in the redirect URL: `/login?redirect=/compare&compare_token={token}`.
   - Add the key pattern to `conventions/valkey-keys-catalog.md`.
2. After login, the user is redirected to `/compare?compare_token={token}`. The `/compare` `+page.server.ts` calls `POST /api/v1/jce/comparisons/from-token`, which loads the selection from Valkey, deletes the key, creates the quick comparison, and returns the new slug. The server then redirects to `/compare/[slug]`.
3. Fallback: if Valkey is unavailable, store a compact base64-json in the redirect query (limit to a few items).

**Verification:** E2E test: guest selects items → login → redirected back → quick comparison created.

---

## Phase 11 — Tool integrations

**Goal:** Wire existing tool pages into the JCE picker.

1. P0 integrations:
   - University detail/list: add **Compare** button linking to `/compare?type=universities&preselect=...`.
   - Visa routes: add **Compare** or **Open in JCE** link to `/compare?type=visa_routes&preselect=...`.
   - Cost of living: add **Compare cities** linking to `/compare?type=cost_of_living&preselect=...`.
2. P1 integrations (as bandwidth allows):
   - Scholarships, remittance, language tests, organizations, jobs, travel, mentors.
3. In `apps/jce` `/compare` page, support `?preselect` and pre-select item(s) in the picker.
4. Add Valkey key pattern `jc:compare:selection:{token}` to `conventions/valkey-keys-catalog.md` if using Valkey.

**Verification:** For each P0 tool, click **Compare** and confirm the item is pre-selected on `/compare` and the quick comparison is created correctly.

---

## Phase 12 — AI `depth` support

**Goal:** AI can create quick and detailed comparisons.

1. Update `internal/ai/tools_comparison.go`:
   - Extend `comparisonCreateArgs` with `Depth` (quick/detailed) and `ItemRefs []ItemRef`.
   - `ComparisonCreateHandler` accepts `depth` and `visibility`.
   - When `depth='quick'`, force `visibility='private'` and `seo_indexable=false`; resolve `ItemRefs` through the resolver registry (or use title-only items if no refs supplied).
   - When `depth='detailed'`, create as draft with requested visibility.
2. Update the AI tool schema (`internal/ai/tools_register.go` or equivalent) and prompt.
3. Add `feature.jce.ai_quick` flag.

**Verification:** `go test ./internal/ai/...` passes. Manual: ask agent to create a quick comparison and verify it is private at `/compare/[slug]`.

---

## Phase 13 — Admin updates

**Goal:** Admin can configure comparison type `config` and see depth.

1. `apps/admin/src/routes/comparison-types/+page.svelte`:
   - Add **Config** JSONB textarea with validation.
   - Update API calls to `GET/POST/PATCH/DELETE /api/v1/jce/comparison-types`.
2. `apps/admin/src/routes/comparisons/+page.svelte`:
   - Update list call to `GET /api/v1/jce/comparisons`.
   - Add depth filter (quick/detailed).
   - Add "Promoted from" column.
   - Link to `apps/jce`: quick → `/compare/{slug}`, detailed → `/comparisons/{slug}`.

**Verification:** `bun run check` and `bun run build` in `apps/admin`.

---

## Phase 14 — Routing, redirects, and Docker

**Goal:** Serve `/compare` and `/comparisons` from `apps/jce`.

1. Create `apps/jce/Dockerfile` (modeled on `apps/tools/Dockerfile`).
2. Add a `jce` service to `docker-compose.yml` on port `3006` with `PRIVATE_API_BASE_URL` pointing at `go-api:8080` and `PUBLIC_JC_API_URL=/api/v1`.
3. Update `docker/nginx.conf`:
   - Add `upstream japa_jce { server jce:3006; }`.
   - Route `/compare*` and `/comparisons*` to `http://japa_jce` when `feature.jce` is enabled. In local dev, `apps/jce` is tested directly on `localhost:3006`; the Nginx change is for the docker compose stack. Because Nginx cannot read `feature.jce` at runtime, this config toggle is managed with the feature release (deployment/infrastructure change).
4. In `apps/main`, add fallback redirects:
   - If `apps/main` happens to receive a request at `/compare` or `/comparisons` while `feature.jce` is on, redirect to the canonical JCE origin (env-driven, e.g. `PUBLIC_JCE_ORIGIN`). This is a safety net, not the primary routing mechanism.
5. Verify `apps/jce` and `apps/main` can coexist in dev on ports 3006 and 3000.

**Verification:** `docker compose up` and `bun run dev` in both apps. `/compare` resolves to `apps/jce`.

---

## Phase 15 — Testing and QA

**Goal:** All tests pass and the feature is ready for release.

1. **Backend tests**
   - `internal/comparison/service_test.go`: quick creation, promotion, visibility rules, list filters, `item_ref` resolution.
   - `internal/comparison/handlers_test.go`: new JCE routes, permissions, guest behavior.
   - `internal/comparison/resolver/...`: resolver tests for each entity type.
2. **Frontend tests**
   - `apps/jce` route tests for `/compare`, `/comparisons`, `/compare/[slug]`.
   - Component tests for `CompareItemGrid`, `ComparisonList`, `PromoteButton`.
3. **Integration tests**
   - End-to-end quick creation → promotion → publish.
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

1. Phase 0, Phase 1, Phase 14 (scaffold, schema, dev routing) can be done in parallel.
2. Phase 2, Phase 3, Phase 4, Phase 5 (backend core) in sequence. Phase 4 (entity search) must be done before any frontend item picker work.
3. Phase 3.5 (migrate `apps/main` composer/wizards) must start after Phase 2 and Phase 3 because it depends on the canonical JCE API and promotion; it can run in parallel with Phase 4/5.
4. Phase 6 (shared components) can start after Phase 4.
5. Phase 7, Phase 8, Phase 9 (`apps/jce` frontend routes) in sequence; Phase 7 needs Phase 4, Phase 8 needs Phase 3 (promotion), Phase 9 needs Phase 5.
6. Phase 10 (guest redirect) and Phase 11 (tool integrations) after Phase 7.
7. Phase 12 (AI), Phase 13 (admin), Phase 15 (QA) in parallel or as polish.

**Exit criteria:**

- `/compare` creates private quick comparisons.
- `/compare/[slug]` shows quick preview with promote/edit/delete.
- `/comparisons` lists public detailed comparisons.
- Promotion clones quick to detailed and opens `apps/main` composer.
- `item_ref` resolvers support at least university, course, space, visa_route, col_city, scholarship, remittance, languagetest, organization, job, travel, mentor, content.
- Tool-page **Compare** buttons work for P0 tools.
- Old `/api/v1/comparisons` routes are removed from `server.go`; `apps/main` composer and wizards call `/api/v1/jce/*`.
- All tests pass and `feature.jce` is off by default.