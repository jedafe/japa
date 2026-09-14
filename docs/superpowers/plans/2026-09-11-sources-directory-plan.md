# Sources Directory & Source Detail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build public `/sources` and `/sources/[slug]` surfaces in the main app, add source attribution across tools and content, and link entity profiles (universities, organizations, users) to `intel_sources` without duplicating them.

**Architecture:** Extend `intel_sources` to be the canonical source-of-truth for publishers and data feeds; add optional `source_id` to `spaces`, `organizations`, and `users`; expose new public API routes; build SvelteKit directory/detail pages inside `AppShell`; enrich domain tool responses with source metadata; update `contents` provenance and `FeedCard` attribution.

**Tech Stack:** Go 1.23, Chi, sqlc/pgx, PostgreSQL, SvelteKit 2/Svelte 5 (Bun), Tailwind CSS v4, Bits UI, LayerChart.

**Spec:** `docs/superpowers/specs/2026-09-11-sources-directory-design.md`

## Global Constraints

- Use `bun` for all JavaScript/TypeScript tooling (Rule 18). Never use `npm`, `npx`, `pnpm`, or `yarn`.
- All frontend data must come from the backend API; no hardcoded arrays or mock data in production components (Rule 16).
- All DB changes go through migrations; generated `internal/db/gen` files are produced by `make sqlc-gen`, never hand-edited (Rule 2).
- All Valkey keys must use the `jc:` prefix (Rule 4).
- All API errors follow `{ "error": { "code": "...", "message": "...", "status": N } }` (Rule 5).
- Follow toggle endpoints are gated by the existing `follows.toggle` permission node.
- Main app surfaces render inside the unified `AppShell` (ADR-058).
- No AI attribution in commits, comments, or documentation (Rule 17).

---

## File Structure

### Backend
- `migrations/000651_extend_intel_sources.up.sql` / `.down.sql`
- `migrations/000652_link_entity_profiles_to_sources.up.sql` / `.down.sql`
- `migrations/000653_contents_source_provenance.up.sql` / `.down.sql`
- `migrations/000654_follows_source_entity.up.sql` / `.down.sql`
- `internal/db/queries/intelligence.sql` — public source listing/detail/item queries.
- `internal/db/queries/follows.sql` — source follow support (if using dedicated table) or existing follow queries.
- `internal/intelligence/handlers/sources_public.go` — new public source handlers.
- `internal/intelligence/service.go` — source read helpers.
- `internal/server/server.go` — route registration.
- `internal/follows/follows.go` — add `EntityTypeSource` constant.
- `internal/follows/service.go` — validate `source` entity, update `intel_sources.follower_count`.
- `internal/scholarships/service.go` — include `source` in `ScholarshipResult`.
- `internal/jobs/service.go` — include `source` in job results.
- `internal/remittance/service.go` — include `source` in rate results.
- `internal/content/store.go` — set `intel_source_id`/`intel_record_id` on promoted/created content.

### Main app
- `apps/main/src/lib/types/index.ts` — add `Source`, `SourceItem`, `SourceListFilters`.
- `apps/main/src/lib/server/api.ts` — add helpers for new routes if needed.
- `apps/main/src/lib/components/source/SourceHeader.svelte` (new) — detail header.
- `apps/main/src/lib/components/source/SourceItemList.svelte` (new) — feed of source items.
- `apps/main/src/routes/(app)/sources/+page.server.ts` (new)
- `apps/main/src/routes/(app)/sources/+page.svelte` (new)
- `apps/main/src/routes/(app)/sources/[slug]/+page.server.ts` (new)
- `apps/main/src/routes/(app)/sources/[slug]/+page.svelte` (new)
- `apps/main/src/lib/components/feed/FeedCard.svelte` — add optional source attribution row.
- `apps/main/src/lib/components/content/ContentArticlePage.svelte` — add source attribution block.

### Tools app
- `apps/tools/src/lib/types/index.ts` — add `source` to scholarship, remittance, job types.
- `apps/tools/src/routes/scholarships/+page.svelte` — render source attribution link.
- `apps/tools/src/routes/remittance/+page.svelte` — render source attribution on rate rows.
- `apps/tools/src/routes/jobs/+page.svelte` — render source attribution link.

### Catalogs
- `.jeos/conventions/api-routes-catalog.md` — register `GET /sources`, `GET /sources/:slug`, `GET /sources/:slug/items`, `POST/DELETE /sources/:slug/follow`.
- `.jeos/sections/14-content-type-system.md` — document `contents.intel_source_id` / `intel_record_id`.
- `.jeos/sections/73-scholarship-finder.md` — source attribution.
- `.jeos/sections/74-remittance-rate-tracker.md` — source attribution.

---

## Task 1: Extend `intel_sources` schema

**Files:**
- Create: `migrations/000651_extend_intel_sources.up.sql`, `migrations/000651_extend_intel_sources.down.sql`
- Modify: `internal/db/queries/intelligence.sql`
- Test: `go test ./internal/intelligence/...` after `make sqlc-gen`

**Interfaces:**
- Produces: `intel_sources` columns `slug`, `description`, `logo_url`, `follower_count`, `post_count`, `source_kind`, and unique index `idx_intel_sources_tenant_slug`.
- Produces: sqlc query `GetIntelSourceBySlug :one`.

- [ ] **Step 1: Write migration**

```sql
-- migrations/000651_extend_intel_sources.up.sql
BEGIN;

ALTER TABLE intel_sources
    ADD COLUMN IF NOT EXISTS slug text,
    ADD COLUMN IF NOT EXISTS description text,
    ADD COLUMN IF NOT EXISTS logo_url text,
    ADD COLUMN IF NOT EXISTS follower_count int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS post_count int NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS source_kind text NOT NULL DEFAULT 'publisher'
        CHECK (source_kind IN ('publisher','institution','individual','aggregator'));

CREATE UNIQUE INDEX IF NOT EXISTS idx_intel_sources_tenant_slug
    ON intel_sources (tenant_id, slug) WHERE slug IS NOT NULL;

COMMIT;
```

```sql
-- migrations/000651_extend_intel_sources.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_intel_sources_tenant_slug;
ALTER TABLE intel_sources
    DROP COLUMN IF EXISTS slug,
    DROP COLUMN IF EXISTS description,
    DROP COLUMN IF EXISTS logo_url,
    DROP COLUMN IF EXISTS follower_count,
    DROP COLUMN IF EXISTS post_count,
    DROP COLUMN IF EXISTS source_kind;
COMMIT;
```

- [ ] **Step 2: Add sqlc query**

Add to `internal/db/queries/intelligence.sql`:

```sql
-- name: GetIntelSourceBySlug :one
SELECT * FROM intel_sources WHERE tenant_id = $1 AND slug = $2 AND enabled = true;
```

- [ ] **Step 3: Run migrations and regenerate sqlc**

Run:
```bash
make migrate-up
make sqlc-gen
```

- [ ] **Step 4: Verify generated code**

Check that `internal/db/gen/intelligence.sql.go` contains `GetIntelSourceBySlug` and the new fields in `IntelSource`.

- [ ] **Step 5: Commit**

```bash
git add migrations/000651_extend_intel_sources.up.sql migrations/000651_extend_intel_sources.down.sql internal/db/queries/intelligence.sql
git commit -m "feat: extend intel_sources with public source fields"
```

---

## Task 2: Link entity profiles to sources

**Files:**
- Create: `migrations/000652_link_entity_profiles_to_sources.up.sql`, `.down.sql`
- Test: `go test ./internal/db/...` after `make sqlc-gen`

**Interfaces:**
- Produces: `spaces.source_id`, `organizations.source_id`, `users.source_id` as nullable FKs to `intel_sources(id)`.
- Produces: sqlc queries `GetSpaceBySourceID`, `GetOrganizationBySourceID`, `GetUserBySourceID` (optional).

- [ ] **Step 1: Write migration**

```sql
-- migrations/000652_link_entity_profiles_to_sources.up.sql
BEGIN;

ALTER TABLE spaces
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_spaces_source_id ON spaces (source_id) WHERE source_id IS NOT NULL;

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_organizations_source_id ON organizations (source_id) WHERE source_id IS NOT NULL;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_source_id ON users (source_id) WHERE source_id IS NOT NULL;

COMMIT;
```

```sql
-- migrations/000652_link_entity_profiles_to_sources.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_spaces_source_id;
DROP INDEX IF EXISTS idx_organizations_source_id;
DROP INDEX IF EXISTS idx_users_source_id;
ALTER TABLE spaces DROP COLUMN IF EXISTS source_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS source_id;
ALTER TABLE users DROP COLUMN IF EXISTS source_id;
COMMIT;
```

- [ ] **Step 2: Run migrations and regenerate sqlc**

```bash
make migrate-up
make sqlc-gen
```

- [ ] **Step 3: Verify generated models**

Check `internal/db/gen/models.go` for new `SourceID` fields and `internal/db/gen/spaces.sql.go`, `internal/db/gen/organizations.sql.go`, `internal/db/gen/users.sql.go` for generated query compatibility.

- [ ] **Step 4: Commit**

```bash
git add migrations/000652_link_entity_profiles_to_sources.*
git commit -m "feat: link spaces, organizations, and users to intel_sources"
```

---

## Task 3: Add content provenance columns

**Files:**
- Create: `migrations/000653_contents_source_provenance.up.sql`, `.down.sql`
- Modify: `internal/db/queries/content.sql`
- Test: `go test ./internal/content/...` after `make sqlc-gen`

**Interfaces:**
- Produces: `contents.intel_source_id` and `contents.intel_record_id` nullable FKs.

- [ ] **Step 1: Write migration**

```sql
-- migrations/000653_contents_source_provenance.up.sql
BEGIN;
ALTER TABLE contents
    ADD COLUMN IF NOT EXISTS intel_source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS intel_record_id uuid REFERENCES intel_records(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_contents_intel_source ON contents (intel_source_id) WHERE intel_source_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_contents_intel_record ON contents (intel_record_id) WHERE intel_record_id IS NOT NULL;
COMMIT;
```

```sql
-- migrations/000653_contents_source_provenance.down.sql
BEGIN;
DROP INDEX IF EXISTS idx_contents_intel_source;
DROP INDEX IF EXISTS idx_contents_intel_record;
ALTER TABLE contents DROP COLUMN IF EXISTS intel_source_id;
ALTER TABLE contents DROP COLUMN IF EXISTS intel_record_id;
COMMIT;
```

- [ ] **Step 2: Update sqlc queries**

Ensure `CreateContent` and `UpdateContent` queries in `internal/db/queries/content.sql` accept and return `intel_source_id` / `intel_record_id`.

- [ ] **Step 3: Run migrations and regenerate sqlc**

```bash
make migrate-up
make sqlc-gen
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000653_contents_source_provenance.* internal/db/queries/content.sql
git commit -m "feat: add intel source provenance to contents"
```

---

## Task 4: Allow follows on sources

**Files:**
- Create: `migrations/000654_follows_source_entity.up.sql`, `.down.sql`
- Modify: `internal/follows/follows.go`, `internal/follows/service.go`, `internal/follows/store.go` (if needed), `internal/db/queries/follows.sql`
- Test: `go test ./internal/follows/...`

**Interfaces:**
- Produces: `EntityTypeSource = "source"` constant.
- Produces: `follows` table accepts `entity_type = 'source'`.
- Produces: `validateEntityExists` and `updateFollowerCount` handle `source`.

- [ ] **Step 1: Update CHECK constraint**

```sql
-- migrations/000654_follows_source_entity.up.sql
BEGIN;
ALTER TABLE follows DROP CONSTRAINT IF EXISTS follows_entity_type_check;
ALTER TABLE follows ADD CONSTRAINT follows_entity_type_check
    CHECK (entity_type IN ('user','space','tag','organization','professional_profile','source'));
COMMIT;
```

```sql
-- migrations/000654_follows_source_entity.down.sql
BEGIN;
ALTER TABLE follows DROP CONSTRAINT IF EXISTS follows_entity_type_check;
ALTER TABLE follows ADD CONSTRAINT follows_entity_type_check
    CHECK (entity_type IN ('user','space','tag','organization','professional_profile'));
COMMIT;
```

- [ ] **Step 2: Update follows package**

In `internal/follows/follows.go`:

```go
const (
    EntityTypeUser         = "user"
    EntityTypeSpace        = "space"
    EntityTypeTag          = "tag"
    EntityTypeOrganization = "organization"
    EntityTypeSource       = "source"
)
```

In `internal/follows/service.go`, update `isValidEntityType`:

```go
func isValidEntityType(entityType string) bool {
    switch entityType {
    case EntityTypeUser, EntityTypeSpace, EntityTypeTag, EntityTypeOrganization, EntityTypeSource:
        return true
    default:
        return false
    }
}
```

Update `validateEntityExists`:

```go
func (s *Service) validateEntityExists(...) error {
    ...
    case EntityTypeSource:
        table = "intel_sources"
    ...
}
```

Update `updateFollowerCount` to handle `source`:

```go
case EntityTypeSource:
    _, err := db.Exec(ctx,
        "UPDATE intel_sources SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
        delta, entityID)
```

- [ ] **Step 3: Run migrations, regenerate sqlc, run tests**

```bash
make migrate-up
make sqlc-gen
go test ./internal/follows/...
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000654_follows_source_entity.* internal/follows/
git commit -m "feat: support following intel sources"
```

---

## Task 5: Public source directory API

**Files:**
- Create: `internal/intelligence/handlers/sources_public.go`
- Modify: `internal/db/queries/intelligence.sql`
- Modify: `internal/server/server.go`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Consumes: `IntelSource` model with new fields.
- Produces: `GET /api/v1/sources` returning `[]SourceDirectoryEntry` + meta.

- [ ] **Step 1: Add sqlc queries per sort mode**

In `internal/db/queries/intelligence.sql` add three queries so the handler can switch by `?sort=`:

```sql
-- name: ListPublicSourcesPopular :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR category = $2)
  AND ($3::text IS NULL OR source_kind = $3)
  AND ($4::text IS NULL OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
ORDER BY follower_count DESC, name ASC
LIMIT $5 OFFSET $6;
```

```sql
-- name: ListPublicSourcesRecent :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR category = $2)
  AND ($3::text IS NULL OR source_kind = $3)
  AND ($4::text IS NULL OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
ORDER BY created_at DESC, name ASC
LIMIT $5 OFFSET $6;
```

```sql
-- name: ListPublicSourcesTrending :many
SELECT * FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR category = $2)
  AND ($3::text IS NULL OR source_kind = $3)
  AND ($4::text IS NULL OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%')
  AND created_at > now() - interval '30 days'
ORDER BY follower_count DESC, post_count DESC
LIMIT $5 OFFSET $6;
```

```sql
-- name: CountPublicSources :one
SELECT COUNT(*) FROM intel_sources
WHERE tenant_id = $1
  AND enabled = true
  AND status = 'active'
  AND ($2::text IS NULL OR category = $2)
  AND ($3::text IS NULL OR source_kind = $3)
  AND ($4::text IS NULL OR name ILIKE '%' || $4 || '%' OR description ILIKE '%' || $4 || '%' OR domain ILIKE '%' || $4 || '%');
```

- [ ] **Step 2: Implement handler**

In `internal/intelligence/handlers/sources_public.go`:

```go
func (h *Handlers) HandleSourcesDirectory(w http.ResponseWriter, r *http.Request) { ... }
```

Rules for the handler:
- Read `tenant_id` from context; if invalid, fall back to `defaultIntelID` (same as `HandleSourcesDirectory` in `moderation.go`).
- Parse `?category=`, `?source_kind=`, `?q=`, `?sort=` (popular | recent | trending; default `popular`), `?limit`/`?offset`.
- Pick the appropriate sqlc query based on `sort`.
- For `linked_entity`, left-join `spaces`, `organizations`, and `users` on `source_id = source.id` (optional single query or in-memory join after listing).
- Return the JSON shape:

```json
{
  "data": [
    {
      "id": "...",
      "slug": "...",
      "name": "...",
      "description": "...",
      "domain": "...",
      "category": "...",
      "source_type": "...",
      "source_kind": "...",
      "trust_level": 2,
      "follower_count": 0,
      "post_count": 0,
      "logo_url": "...",
      "linked_entity": null
    }
  ],
  "meta": { "limit": 20, "offset": 0, "total": 0 }
}
```

- [ ] **Step 3: Register public route with optional auth**

In `internal/server/server.go` under the public/v1 API group:

```go
r.Group(func(r chi.Router) {
    r.Use(auth.OptionalUserFromHeader) // or existing optional auth middleware
    r.Get("/sources", intelHandlers.HandleSourcesDirectory)
})
```

- [ ] **Step 4: Write handler test**

Create `internal/intelligence/handlers/sources_public_test.go`:

```go
func TestHandleSourcesDirectory(t *testing.T) {
    // Setup test source, call handler, assert 200 and JSON shape.
}
```

- [ ] **Step 5: Run tests and commit**

```bash
go test ./internal/intelligence/handlers/...
git add internal/intelligence/handlers/sources_public.go internal/intelligence/handlers/sources_public_test.go internal/db/queries/intelligence.sql internal/server/server.go
git commit -m "feat: public source directory API with sort and auth"
```

---

## Task 6: Public source detail and item feed API

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go`
- Modify: `internal/db/queries/intelligence.sql`
- Modify: `internal/server/server.go`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Consumes: `GetIntelSourceBySlug`, `ListPublicSources`.
- Produces: `GET /api/v1/sources/:slug`, `GET /api/v1/sources/:slug/items`.

- [ ] **Step 1: Add source detail and item queries**

In `internal/db/queries/intelligence.sql`:

```sql
-- name: GetIntelSourceBySlugWithLinkedEntity :one
SELECT s.*,
       sp.slug AS space_slug, sp.name AS space_name,
       o.slug AS org_slug, o.name AS org_name,
       u.username AS user_slug, u.display_name AS user_name
FROM intel_sources s
LEFT JOIN spaces sp ON sp.source_id = s.id
LEFT JOIN organizations o ON o.source_id = s.id
LEFT JOIN users u ON u.source_id = s.id
WHERE s.tenant_id = $1 AND s.slug = $2 AND s.enabled = true;
```

For the item feed, reuse the radar pattern. Add `BySource` listing queries for each domain (or add `source_id` filter to existing list queries). For v1, add:

```sql
-- name: ListScholarshipsBySource :many
SELECT * FROM scholarships
WHERE tenant_id = $1 AND intel_record_id IN (
    SELECT id FROM intel_records WHERE tenant_id = $1 AND source_id = $2 AND status = 'published'
)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
```

```sql
-- name: ListJobsBySource :many
SELECT * FROM jobs
WHERE tenant_id = $1 AND source_id = $2 AND status = 'published'
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
```

```sql
-- name: ListRemittanceRatesBySource :many
SELECT * FROM remittance_rates
WHERE tenant_id = $1 AND intel_record_id IN (
    SELECT id FROM intel_records WHERE tenant_id = $1 AND source_id = $2 AND status = 'published'
)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4;
```

```sql
-- name: ListContentsBySource :many
SELECT * FROM contents
WHERE tenant_id = $1 AND intel_source_id = $2 AND status = 'published'
ORDER BY published_at DESC
LIMIT $3 OFFSET $4;
```

- [ ] **Step 2: Implement handlers**

In `internal/intelligence/handlers/sources_public.go`:

```go
func (h *Handlers) HandleGetSource(w http.ResponseWriter, r *http.Request) { ... }
func (h *Handlers) HandleListSourceItems(w http.ResponseWriter, r *http.Request) { ... }
```

`HandleGetSource`:
- Resolve slug to source using `GetIntelSourceBySlugWithLinkedEntity`.
- Fall back to `defaultIntelID` if the request tenant is empty.
- Compute `is_following` with `auth.OptionalUser` and `follows.IsFollowing`.
- Build `linked_entity` from the joined `space_*`, `org_*`, or `user_*` columns.

`HandleListSourceItems`:
- Parse `?entity_type=` and `?sort=` (latest | popular | discussed | upvoted; default `latest`).
- Load published records from `intel_records` by `source_id` and `entity_type` filter, or use the domain-specific `BySource` queries.
- Reuse the `radarItem` converters in `internal/intelligence/handlers/radar.go` (`scholarshipToRadarItem`, `jobToRadarItem`, `remittanceRateToRadarItem`, etc.) to build a uniform `SourceItem` list with `title`, `excerpt`, `entity_type`, `href`, `original_url`, `published_at`, `vote_score`, `comment_count`, `view_count`.
- Sort items in Go by the requested sort field (extracted_at/published_at, vote_score, comment_count, view_count).

- [ ] **Step 3: Register public routes with optional auth**

```go
r.Group(func(r chi.Router) {
    r.Use(auth.OptionalUserFromHeader)
    r.Get("/sources/{slug}", intelHandlers.HandleGetSource)
    r.Get("/sources/{slug}/items", intelHandlers.HandleListSourceItems)
})
```

- [ ] **Step 4: Add tests**

```go
func TestHandleGetSource(t *testing.T) { ... }
func TestHandleListSourceItems(t *testing.T) { ... }
```

- Test: `GET /sources/nodejs` returns 200 with source metadata and `linked_entity`.
- Test: `GET /sources/nodejs/items` returns published records with correct `entity_type` and `href`.
- Test: `?entity_type=scholarship` filters the feed.
- Test: `?sort=popular` sorts by `vote_score` descending.

- [ ] **Step 5: Run tests and commit**

```bash
go test ./internal/intelligence/handlers/...
git commit -m "feat: public source detail and item feed API"
```

---

## Task 7: Source follow endpoints

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go`, `internal/follows/service.go`
- Modify: `internal/server/server.go`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Consumes: `follows.Service.ToggleFollow`.
- Produces: `POST /api/v1/sources/{slug}/follow`, `DELETE /api/v1/sources/{slug}/follow`.

- [ ] **Step 1: Implement handler**

In `internal/intelligence/handlers/sources_public.go`:

```go
func (h *Handlers) HandleToggleSourceFollow(w http.ResponseWriter, r *http.Request) { ... }
```

Resolve the source slug to `intel_sources.id`, call `h.follows.ToggleFollow(ctx, ID, userID, follows.EntityTypeSource, sourceID)`, and return `{ "data": { "is_following": bool, "follower_count": int } }`.

- [ ] **Step 2: Register routes under auth**

```go
r.With(auth.RequireAuth(...)).Post("/sources/{slug}/follow", intelHandlers.HandleToggleSourceFollow)
r.With(auth.RequireAuth(...)).Delete("/sources/{slug}/follow", intelHandlers.HandleToggleSourceFollow)
```

- [ ] **Step 3: Add test**

- Test: authed user can follow and unfollow a source; `follower_count` updates.

- [ ] **Step 4: Run tests and commit**

```bash
go test ./internal/intelligence/handlers/...
git commit -m "feat: source follow toggle endpoints"
```

---

## Task 8: Content provenance wiring

**Files:**
- Modify: `internal/intelligence/promoter/*.go` (scholarship, job, news, wiki, etc.)
- Modify: `internal/content/store.go` (CreateContent)
- Test: `go test ./internal/intelligence/promoter/...`

**Interfaces:**
- Consumes: `intel_record.source_id` and `intel_record.id`.
- Produces: `contents.intel_source_id` and `contents.intel_record_id` set on promoted/syndicated content; `intel_sources.post_count` updated on publish/unpublish.

- [ ] **Step 1: Update content creation**

Where promoters create `contents` rows, include `IntelSourceID` and `IntelRecordID` from the record.

For example, in `internal/intelligence/promoter/scholarship.go`:

```go
contentParams := gen.CreateContentParams{
    ...
    IntelSourceID: record.SourceID,
    IntelRecordID: record.ID,
    ...
}
```

- [ ] **Step 2: Add source post_count maintenance queries**

In `internal/db/queries/intelligence.sql`:

```sql
-- name: IncrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = post_count + 1
WHERE tenant_id = $1 AND id = $2;
```

```sql
-- name: DecrementSourcePostCount :exec
UPDATE intel_sources
SET post_count = GREATEST(0, post_count - 1)
WHERE tenant_id = $1 AND id = $2;
```

- [ ] **Step 3: Update promoters to maintain post_count**

When a record reaches `status = 'published'` and has a promoted entity, call `IncrementSourcePostCount` with `record.SourceID` in the same transaction. When a published record is rejected, removed, or unpublished, call `DecrementSourcePostCount`.

- [ ] **Step 4: Update existing content service**

Ensure `internal/content/store.go` `CreateContent` passes the new IDs through to the generated query.

- [ ] **Step 5: Run tests**

```bash
go test ./internal/intelligence/promoter/...
go test ./internal/content/...
```

- [ ] **Step 6: Commit**

```bash
git commit -m "feat: wire intel source/record provenance and maintain post counts"
```

---

## Task 9: Add source attribution to scholarship responses

**Files:**
- Modify: `internal/scholarships/service.go`, `internal/scholarships/handlers.go`
- Modify: `internal/db/queries/scholarships_intel.sql` (if needed)
- Test: `go test ./internal/scholarships/...`

**Interfaces:**
- Produces: `ScholarshipResult.Source` with `{ id, slug, name, logo_url, trust_level, domain }`.

- [ ] **Step 1: Update query/service**

Update `FindScholarships` in `internal/scholarships/service.go` to join `intel_sources` on `scholarships.intel_record_id → intel_records.source_id` (or use cached columns on `scholarships` if denormalized).

Add `Source` to `ScholarshipResult`.

- [ ] **Step 2: Update handler response**

Ensure `HandleFindScholarships` serializes the `Source` field.

- [ ] **Step 3: Run tests and commit**

```bash
go test ./internal/scholarships/...
git commit -m "feat: include source attribution in scholarship results"
```

---

## Task 10: Add source attribution to job and remittance responses

**Files:**
- Modify: `internal/jobs/service.go`, `internal/jobs/handlers.go`
- Modify: `internal/remittance/service.go`, `internal/remittance/handlers.go`
- Test: `go test ./internal/jobs/... ./internal/remittance/...`

**Interfaces:**
- Produces: job and remittance result types with `Source` object.

- [ ] **Step 1: Repeat Task 9 pattern for jobs and remittance**

Join `intel_sources` on `intel_record_id` and add `Source` to the response structs.

- [ ] **Step 2: Run tests and commit**

```bash
go test ./internal/jobs/... ./internal/remittance/...
git commit -m "feat: include source attribution in job and remittance results"
```

---

## Task 11: Main app `/sources` directory page

**Files:**
- Create: `apps/main/src/lib/types/source.ts` or add to `apps/main/src/lib/types/index.ts`
- Create: `apps/main/src/routes/(app)/sources/+page.server.ts`
- Create: `apps/main/src/routes/(app)/sources/+page.svelte`
- Modify: `apps/main/src/lib/i18n/en.json` and `fr.json`
- Test: `bun run check` in `apps/main`

**Interfaces:**
- Consumes: `GET /api/v1/sources`.
- Produces: server `load()` returning `{ sources, meta, filters }`.

- [ ] **Step 1: Add types**

```ts
export interface Source {
    id: string;
    slug: string;
    name: string;
    description: string | null;
    domain: string;
    category: string;
    source_type: string;
    source_kind: string;
    trust_level: number;
    follower_count: number;
    post_count: number;
    logo_url: string | null;
    linked_entity: { type: string; slug: string; name: string } | null;
    is_following?: boolean;
}

export interface SourceItem {
    id: string;
    entity_type: string;
    title: string;
    excerpt: string | null;
    original_url: string | null;
    published_at: string;
    vote_score: number;
    comment_count: number;
    view_count: number;
    href: string;
}
```

- [ ] **Step 2: Write server load**

`apps/main/src/routes/(app)/sources/+page.server.ts`:

```ts
import { apiContext, apiList } from '$lib/server/api';
import type { PageServerLoad } from './$types';
import type { Source } from '$lib/types';

export const load: PageServerLoad = async (event) => {
    const ctx = apiContext(event);
    const params = event.url.searchParams;
    const category = params.get('category') ?? '';
    const q = params.get('q') ?? '';
    const qs = new URLSearchParams();
    if (category) qs.set('category', category);
    if (q) qs.set('q', q);
    qs.set('limit', '24');
    qs.set('offset', '0');
    const res = await apiList<Source>(ctx, `/sources?${qs.toString()}`);
    return { sources: res.items, meta: res.meta, category, q };
};
```

- [ ] **Step 3: Write page component**

Render a tab bar for categories, a search input, and a grid of `DirectoryCard` components. Each card links to `/sources/[source.slug]`.

- [ ] **Step 4: Run checks**

```bash
cd apps/main
bun run check
```

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/routes/(app)/sources apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat: sources directory page in main app"
```

---

## Task 12: Main app `/sources/[slug]` detail page

**Files:**
- Create: `apps/main/src/routes/(app)/sources/[slug]/+page.server.ts`
- Create: `apps/main/src/routes/(app)/sources/[slug]/+page.svelte`
- Create: `apps/main/src/lib/components/source/SourceHeader.svelte`
- Create: `apps/main/src/lib/components/source/SourceItemList.svelte`
- Test: `bun run check` in `apps/main`

**Interfaces:**
- Consumes: `GET /api/v1/sources/:slug` and `GET /api/v1/sources/:slug/items`.
- Produces: server `load()` with `{ source, items, meta }`.

- [ ] **Step 1: Write server load**

```ts
export const load: PageServerLoad = async (event) => {
    const ctx = apiContext(event);
    const slug = event.params.slug;
    const [sourceRes, itemsRes] = await Promise.all([
        apiGet<Source>(ctx, `/sources/${slug}`),
        apiList<SourceItem>(ctx, `/sources/${slug}/items?limit=20`)
    ]);
    return { source: sourceRes, items: itemsRes.items, meta: itemsRes.meta };
};
```

- [ ] **Step 2: Write detail page**

- `SourceHeader.svelte`: logo, name, domain, description, follow button, trust badge, linked entity card.
- `SourceItemList.svelte`:
  - **Entity-type tabs:** All / Scholarships / Jobs / Remittance / Contents / Videos / News / About.
  - **Sort tabs:** Latest / Popular / Discussed / Upvoted. Pass `?sort=` to the `GET /sources/:slug/items` API.
  - Render `FeedCard` or domain card list.
- `+page.svelte`: compose header and item list; handle follow toggle with optimistic UI and rollback.

- [ ] **Step 3: Derive and display related tags**

Use `source.category` plus the top N recurring tags from the source's published items to render a related-tags row. Add i18n keys:

```json
"sources.follow": "Follow",
"sources.unfollow": "Unfollow",
"sources.about": "About",
"sources.latest": "Latest",
"sources.popular": "Popular",
"sources.discussed": "Discussed",
"sources.upvoted": "Upvoted",
"sources.dataSources": "Data sources",
"sources.viewProfile": "View profile"
```

- [ ] **Step 4: Run checks and commit**

```bash
cd apps/main
bun run check
git commit -m "feat: source detail page and source item feed"
```

---

## Task 13: Add source attribution to feed and content cards

**Files:**
- Modify: `apps/main/src/lib/components/feed/FeedCard.svelte`
- Modify: `apps/main/src/lib/types/index.ts` (FeedItem source field)
- Modify: `apps/main/src/lib/components/content/ContentArticlePage.svelte`
- Modify: backend responses that populate `FeedItem` to include `source`.
- Test: `bun run check` in `apps/main`

**Interfaces:**
- Consumes: `item.source` on `FeedItem`.
- Produces: source link on every card and content detail page.

- [ ] **Step 1: Extend FeedItem type**

```ts
export interface FeedItem {
    ...
    source?: { name: string; slug: string; logo_url: string | null } | null;
}
```

- [ ] **Step 2: Update FeedCard**

Add a source attribution row under the title/excerpt:

```svelte
{#if item.source}
    <a href="/sources/{item.source.slug}" class="text-xs text-[var(--color-muted)] hover:text-[var(--color-primary)]">
        From {item.source.name}
    </a>
{/if}
```

- [ ] **Step 3: Update ContentArticlePage**

Add a source attribution block near the author line:

```svelte
{#if data.content.source}
    <p class="text-sm text-[var(--color-muted)]">
        Source: <a href="/sources/{data.content.source.slug}">{data.content.source.name}</a>
        {#if data.content.canonical_url}
            · <a href={data.content.canonical_url} target="_blank" rel="noopener">Originally from {data.content.source.domain}</a>
        {/if}
    </p>
{/if}
```

- [ ] **Step 4: Run checks and commit**

```bash
cd apps/main
bun run check
git commit -m "feat: source attribution on feed cards and content detail"
```

---

## Task 14: Tool source attribution

**Files:**
- Modify: `apps/tools/src/lib/types/index.ts`
- Modify: `apps/tools/src/routes/scholarships/+page.svelte`
- Modify: `apps/tools/src/routes/remittance/+page.svelte`
- Modify: `apps/tools/src/routes/jobs/+page.svelte` (if exists)
- Test: `bun run check` in `apps/tools`

**Interfaces:**
- Consumes: `source` object added to scholarship, remittance, and job API responses.
- Produces: compact source link on each tool result row and a “Data sources” header link.

- [ ] **Step 1: Add source to tool types**

```ts
export interface Scholarship {
    ...
    source: { name: string; slug: string; logo_url: string | null; trust_level: number } | null;
}
```

- [ ] **Step 2: Render source link in scholarships tool**

In the result card/list item:

```svelte
{#if scholarship.source}
    <a href="{mainAppUrl(`/sources/${scholarship.source.slug}`)}" class="text-xs text-[var(--color-muted)]">
        From {scholarship.source.name}
    </a>
{/if}
```

- [ ] **Step 3: Repeat for remittance and jobs**

- [ ] **Step 4: Add “Data sources” link**

Add a link in each tool page header to `{mainAppUrl(`/sources?category=${domain}`)}`.

- [ ] **Step 5: Run checks and commit**

```bash
cd apps/tools
bun run check
git commit -m "feat: source attribution and data source links in tools"
```

---

## Task 15: Backfill source slugs and post counts

**Files:**
- Create: `migrations/000655_backfill_source_slugs.up.sql`, `.down.sql` (or include in 000651 if not yet run)
- Test: `make migrate-up`, spot check DB.

**Interfaces:**
- Produces: slugs for existing `intel_sources` rows; initial `post_count`.

- [ ] **Step 1: Write backfill migration**

```sql
-- migrations/000655_backfill_source_slugs.up.sql
BEGIN;
UPDATE intel_sources
SET slug = lower(regexp_replace(name, '[^a-zA-Z0-9]+', '-', 'g'))
WHERE slug IS NULL;

-- Deduplicate any collisions by appending a short hash.
UPDATE intel_sources s
SET slug = s.slug || '-' || substr(md5(s.id::text || s.tenant_id::text), 1, 6)
WHERE s.id IN (
    SELECT id FROM (
        SELECT id, row_number() OVER (PARTITION BY tenant_id, slug ORDER BY created_at, id) AS rn
        FROM intel_sources
        WHERE slug IS NOT NULL
    ) t WHERE rn > 1
);
COMMIT;
```

```sql
-- migrations/000655_backfill_source_slugs.down.sql
BEGIN;
UPDATE intel_sources SET slug = NULL WHERE slug IS NOT NULL;
COMMIT;
```

- [ ] **Step 2: Backfill post_count**

A script or migration counts `intel_records` per source and sets `post_count`. For v1, run a one-off script or include it in the migration:

```sql
UPDATE intel_sources s
SET post_count = (
    SELECT COUNT(*) FROM intel_records r
    WHERE r.tenant_id = s.tenant_id AND r.source_id = s.id AND r.status = 'published'
)
WHERE s.post_count = 0;
```

- [ ] **Step 3: Run and verify**

```bash
make migrate-up
psql -c "SELECT slug, post_count, follower_count FROM intel_sources LIMIT 5;"
```

- [ ] **Step 4: Commit**

```bash
git add migrations/000655_backfill_source_slugs.*
git commit -m "feat: backfill source slugs and post counts"
```

---

## Task 16: Update JEOS catalogs

**Files:**
- Modify: `.jeos/conventions/api-routes-catalog.md`
- Modify: `.jeos/sections/14-content-type-system.md`
- Modify: `.jeos/sections/73-scholarship-finder.md`
- Modify: `.jeos/sections/74-remittance-rate-tracker.md`

**Interfaces:**
- Produces: documented public routes and schema changes.

- [ ] **Step 1: Update api-routes-catalog.md**

Add under Public / v1:

```
GET    /sources                                list public sources
GET    /sources/{slug}                         get source profile
GET    /sources/{slug}/items                   list source's published items
POST   /sources/{slug}/follow                  follow a source
DELETE /sources/{slug}/follow                  unfollow a source
```

- [ ] **Step 2: Update sections**

Document `contents.intel_source_id` / `intel_record_id` in Section 14 and source attribution in Sections 73 and 74.

- [ ] **Step 3: Commit**

```bash
git add .jeos/
git commit -m "docs: update api routes and section docs for sources"
```

---

## Verification

- [ ] Run `go test ./internal/intelligence/... ./internal/follows/... ./internal/scholarships/... ./internal/jobs/... ./internal/remittance/... ./internal/content/...`
- [ ] Run `make sqlc-gen` and verify no hand-edited generated files.
- [ ] Run `bun run check` in `apps/main` and `apps/tools`.
- [ ] Run a final conformance pass:
  - Check no hardcoded source data in SvelteKit (Rule 16).
  - Check all new API errors use the standard `{error: {code, message, status}}` shape (Rule 5).
  - Check all Valkey keys use `jc:` prefix (Rule 4).
  - Check no AI attribution in commits or docs (Rule 17).
- [ ] Manual QA:
  - Navigate to `/sources`; verify directory renders with sort tabs (Popular / Recent / Trending) and category filters.
  - Click a source; verify detail page shows entity-type tabs and sort tabs (Latest / Popular / Discussed / Upvoted).
  - Click an item; verify it opens in JapaConnect with a source back-link.
  - In Scholarship Finder / Remittance tool, verify source attribution links.
  - Follow a source and refresh; verify `follower_count` updates.

---

## Out of scope for v1 (explicit)

The plan is scoped to the hybrid publisher/entity-profile model we agreed on. The following daily.dev-adjacent features are intentionally not in this plan and should be added in later phases unless the scope changes:

1. **"Suggest new source" UI** — daily.dev allows users to propose new publishers. This requires a new admin/moderation queue and is not in v1.
2. **"Block source" action** — daily.dev has a block action in addition to follow. JapaConnect's current block system is user/space oriented; extending it to sources is a separate feature.
3. **Unifying `video_sources` and `comparison_data_sources`** — the canonical v1 registry is `intel_sources`. `video_sources` and `comparison_data_sources` remain independent; a later phase can link or fold them in.
4. **"Top video sources" / "Recently added" / "Rising" directory sections** — the v1 directory supports category and sort tabs; curated section carousels can be added later.
5. **Auto-creating `intel_sources` for every university/organization/user** — only entities that are actual publishers/data feeds get a `source_id`. Bulk auto-creation is not in v1.

---

## Self-review

**Spec coverage:**
- `/sources` directory with sort tabs (Popular / Recent / Trending) — Task 5 + 11.
- `/sources/[slug]` detail with entity-type and sort tabs (Latest / Popular / Discussed / Upvoted) — Task 6 + 12.
- Item feed opens inside JapaConnect with source link — Task 6 + 13.
- Tool source attribution — Task 14.
- Hybrid entity profile links — Task 2 + detail page `linked_entity`.
- Follow sources — Task 4 + 7.
- Content provenance and source `post_count` maintenance — Task 3 + 8.
- Schema extensions — Task 1.
- Public route auth/tenant fallback — Task 5 + 6.

**Placeholder scan:** No TBD/TODO placeholders. Migration numbers use 000651–000655 and assume the current latest migration is 000650.

**Type consistency:** `Source` type in `apps/main/src/lib/types` matches the API response shape. `FeedItem.source` is optional and matches the backend addition. Follow entity type `source` is added consistently in DB, service, and package constants. `SourceItem` `sort` values match the backend `?sort=` enum in Task 6.
