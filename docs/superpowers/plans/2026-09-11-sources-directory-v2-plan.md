# Sources Directory & Source Detail — V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Prerequisite:** `docs/superpowers/plans/2026-09-11-sources-directory-plan.md` (v1) is complete and in production.

**Goal:** Extend the v1 `/sources` surface with community submission, source blocking, unified non-intel source registries, curated directory sections, and richer source profiles.

**Architecture:** Build on the v1 `intel_sources` registry; add `user_source_blocks`, public source proposal, admin approval workflow, `video_sources` and `comparison_data_sources` linkage, and a `source_collections` table for curated directory sections.

**Tech Stack:** Go 1.23, Chi, sqlc/pgx, PostgreSQL, SvelteKit 2/Svelte 5 (Bun), Tailwind CSS v4, Bits UI, LayerChart.

**Spec:** `docs/superpowers/specs/2026-09-11-sources-directory-design.md`

## Global Constraints

- Use `bun` for all JavaScript/TypeScript tooling (Rule 18).
- All frontend data from the API (Rule 16).
- Migrations + `make sqlc-gen` for DB changes (Rule 2).
- Valkey keys with `jc:` prefix (Rule 4).
- Standard API error format (Rule 5).
- No AI attribution in commits or docs (Rule 17).

---

## File Structure

### Backend
- `migrations/0007XX_add_user_source_blocks.up.sql`
- `migrations/0007XX_link_video_and_comparison_sources.up.sql`
- `migrations/0007XX_add_source_collections.up.sql`
- `migrations/0007XX_source_profile_extensions.up.sql`
- `internal/db/queries/intelligence.sql` — source block, proposal, collection queries.
- `internal/db/queries/video_sources.sql` — link to `intel_sources`.
- `internal/db/queries/comparison_data_sources.sql` — link to `intel_sources`.
- `internal/intelligence/handlers/sources_public.go` — propose and block endpoints.
- `internal/intelligence/handlers/moderation.go` — approve/reject source proposals.
- `internal/spam/handlers.go` or `internal/sources/` — block service.
- `internal/server/server.go` — route registration.

### Main app
- `apps/main/src/routes/(app)/sources/submit/+page.server.ts` and `+page.svelte`
- `apps/main/src/lib/components/source/SourceBlockButton.svelte`
- `apps/main/src/routes/(app)/sources/+page.svelte` — curated sections.
- `apps/main/src/routes/(app)/sources/[slug]/+page.svelte` — richer profile (social links, stats).

### Tools app
- `apps/tools/src/routes/sources/+page.svelte` (optional tools-side directory) — deferred.

### Catalogs
- `.jeos/conventions/api-routes-catalog.md` — `POST /sources/propose`, `POST /sources/{slug}/block`, etc.
- `.jeos/sections/106-video-content-and-import.md` — `video_sources.source_id`.
- `.jeos/conventions/jce-data-sources.md` — `comparison_data_sources.source_id`.

---

## Task 1: Public "Suggest new source" workflow

**Files:**
- Modify: `internal/intelligence/handlers/sources_public.go`
- Modify: `internal/server/server.go`
- Modify: `internal/intelligence/service.go` or store
- Create: `apps/main/src/routes/(app)/sources/submit/+page.server.ts`, `+page.svelte`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Produces: `POST /api/v1/sources/propose`.
- Consumes: existing `intel_sources` proposal columns (`status`, `proposed_by`, `proposed_at`).

- [ ] **Step 1: Add public propose endpoint**

```go
func (h *Handlers) HandleProposeSource(w http.ResponseWriter, r *http.Request) { ... }
```

Request body:

```json
{
  "name": "Node.js Blog",
  "domain": "nodejs.org",
  "source_url": "https://nodejs.org/en/blog/",
  "category": "news",
  "source_type": "rss",
  "description": "Official Node.js releases and announcements"
}
```

Rules:
- Requires auth and `sources.propose` permission (or `user` if no permission node exists yet).
- Insert into `intel_sources` with `status='proposed'`, `enabled=false`, `proposed_by=user_id`, `proposed_at=now()`, `trust_level=5` (community / untrusted).
- Return `201` with the proposed source slug and an `admin_review_url`.

- [ ] **Step 2: Register route**

```go
r.With(auth.RequireAuth(...)).Post("/sources/propose", intelHandlers.HandleProposeSource)
```

- [ ] **Step 3: Add submission page**

`apps/main/src/routes/(app)/sources/submit/+page.svelte`:
- Form with fields: name, domain, source URL, category (dropdown from API), source type (rss / youtube / api / static), description.
- Submit to `POST /api/v1/sources/propose`.
- Show success with "Pending admin review" message.

- [ ] **Step 4: Add tests**

- Test: authed user can propose a source; row is created with `status='proposed'` and `enabled=false`.
- Test: duplicate `source_url` returns `409 duplicate_source`.

- [ ] **Step 5: Commit**

```bash
git commit -m "feat: public source proposal workflow"
```

---

## Task 2: Source blocking

**Files:**
- Create: `migrations/0007XX_add_user_source_blocks.up.sql`, `.down.sql`
- Create: `internal/sources/blocks.go` or `internal/spam/source_blocks.go`
- Modify: `internal/intelligence/handlers/sources_public.go`
- Modify: `internal/db/queries/intelligence.sql`
- Modify: `internal/intelligence/handlers/radar.go` and `sources_public.go` item feed
- Test: `go test ./internal/sources/...` or `./internal/intelligence/handlers/...`

**Interfaces:**
- Produces: `POST /api/v1/sources/{slug}/block`, `DELETE /api/v1/sources/{slug}/block`.
- Produces: `GET /api/v1/me/blocked-sources`.
- Produces: feed endpoints exclude items from blocked sources.

- [ ] **Step 1: Add migration**

```sql
-- migrations/0007XX_add_user_source_blocks.up.sql
BEGIN;

CREATE TABLE user_source_blocks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    blocker_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id   uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, blocker_id, source_id)
);

CREATE INDEX idx_user_source_blocks_blocker ON user_source_blocks(tenant_id, blocker_id);
CREATE INDEX idx_user_source_blocks_source ON user_source_blocks(tenant_id, source_id);

COMMIT;
```

- [ ] **Step 2: Add block toggle handler**

```go
func (h *Handlers) HandleToggleSourceBlock(w http.ResponseWriter, r *http.Request) { ... }
```

Rules:
- Requires auth.
- Resolve source slug to `source_id`.
- Insert or delete `user_source_blocks` row.
- Return `{ "data": { "is_blocked": bool } }`.

- [ ] **Step 3: Filter blocked sources from feeds**

- Update `HandleListSourceItems` to exclude blocked sources for the caller.
- Update `HandleRadarFeed` to exclude items from blocked sources for the caller.
- Add a helper `ListBlockedSourceIDs(ctx, tenantID, userID)` in `internal/sources/blocks.go`.

- [ ] **Step 4: Add UI block button**

`SourceBlockButton.svelte`:
- Render "Block source" / "Unblock source" next to the follow button on source detail.
- Optimistic toggle with rollback on error.

- [ ] **Step 5: Add tests and commit**

- Test: blocked source items do not appear in `/sources/{slug}/items` for the blocker.
- Test: blocked source items do not appear in radar feed for the blocker.
- Test: unblocking restores items.

```bash
git commit -m "feat: source blocking and feed filtering"
```

---

## Task 3: Unify `video_sources` and `comparison_data_sources` with `/sources`

**Files:**
- Create: `migrations/0007XX_link_video_and_comparison_sources.up.sql`, `.down.sql`
- Modify: `internal/db/queries/video_sources.sql`, `internal/db/queries/comparison_data_sources.sql`
- Modify: `internal/intelligence/handlers/sources_public.go`
- Modify: `internal/video/handlers.go`, `internal/jce/handlers.go`
- Test: `go test ./internal/intelligence/handlers/... ./internal/video/... ./internal/jce/...`

**Interfaces:**
- Produces: `video_sources.source_id` and `comparison_data_sources.source_id` as optional FKs to `intel_sources`.
- Produces: `/sources` directory includes video and comparison sources when linked.

- [ ] **Step 1: Add migration**

```sql
-- migrations/0007XX_link_video_and_comparison_sources.up.sql
BEGIN;

ALTER TABLE video_sources
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX idx_video_sources_source_id ON video_sources(source_id) WHERE source_id IS NOT NULL;

ALTER TABLE comparison_data_sources
    ADD COLUMN IF NOT EXISTS source_id uuid REFERENCES intel_sources(id) ON DELETE SET NULL;
CREATE INDEX idx_comparison_data_sources_source_id ON comparison_data_sources(source_id) WHERE source_id IS NOT NULL;

COMMIT;
```

- [ ] **Step 2: Link existing rows**

For existing `video_sources` and `comparison_data_sources`, create or link matching `intel_sources` rows:

```sql
INSERT INTO intel_sources (tenant_id, name, domain, source_url, category, source_type, trust_level, crawl_method, crawl_frequency, source_kind)
SELECT tenant_id, name, domain, source_url, 'video', 'video_upload', 2, 'manual', 'manual', 'publisher'
FROM video_sources
WHERE source_id IS NULL;

UPDATE video_sources v
SET source_id = s.id
FROM intel_sources s
WHERE v.tenant_id = s.tenant_id AND v.source_url = s.source_url AND v.source_id IS NULL;
```

Repeat for `comparison_data_sources` with `category='comparison'` and `source_type='comparison_api'`.

- [ ] **Step 3: Update source directory query**

Modify `ListPublicSources*` queries in `internal/db/queries/intelligence.sql` to include video and comparison sources via the `source_id` link when `video_sources` or `comparison_data_sources` rows exist. Alternatively, build a `sources` view or materialized directory table.

For v2, the simpler path is a union in the handler:

```go
intelRows := h.intelStores.ListPublicSources(...)
videoRows := h.videoStores.ListVideoSourcesLinked(...)
comparisonRows := h.jceStores.ListComparisonSourcesLinked(...)
// merge, sort, paginate in Go
```

- [ ] **Step 4: Update item feed to include video and comparison items**

Extend `HandleListSourceItems` to include `video_sources` and `comparison_data_sources` items when the source is linked. Add converters `videoToRadarItem` and `comparisonToRadarItem` in `radar.go` style.

- [ ] **Step 5: Update tools to set `source_id`**

When video or comparison sources are created in admin tools, create or set the linked `intel_sources` row.

- [ ] **Step 6: Add tests and commit**

- Test: a linked video source appears in `/sources`.
- Test: a linked comparison source appears in `/sources?category=comparison`.
- Test: source detail item feed includes the linked source's video/comparison items.

```bash
git commit -m "feat: link video and comparison sources to sources directory"
```

---

## Task 4: Curated directory sections

**Files:**
- Create: `migrations/0007XX_add_source_collections.up.sql`, `.down.sql`
- Create: `internal/intelligence/handlers/source_collections.go`
- Modify: `apps/main/src/routes/(app)/sources/+page.svelte`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Produces: `source_collections` table, `GET /api/v1/sources/collections`.
- Produces: `/sources` page renders sections ("Trending", "Top video sources", "Recently added", "Rising").

- [ ] **Step 1: Add migration**

```sql
-- migrations/0007XX_add_source_collections.up.sql
BEGIN;

CREATE TABLE source_collections (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    slug        text NOT NULL,
    name        text NOT NULL,
    description text,
    collection_type text NOT NULL CHECK (collection_type IN ('manual', 'query')),
    query_json  jsonb, -- stores filters for query-based collections
    sort        text,
    limit       int NOT NULL DEFAULT 10,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE source_collection_items (
    collection_id   uuid NOT NULL REFERENCES source_collections(id) ON DELETE CASCADE,
    source_id       uuid NOT NULL REFERENCES intel_sources(id) ON DELETE CASCADE,
    position        int NOT NULL DEFAULT 0,
    PRIMARY KEY (collection_id, source_id)
);

CREATE INDEX idx_source_collections_tenant ON source_collections(tenant_id, collection_type);
CREATE INDEX idx_source_collection_items_collection ON source_collection_items(collection_id, position);

COMMIT;
```

- [ ] **Step 2: Implement collection API**

```go
func (h *Handlers) HandleListSourceCollections(w http.ResponseWriter, r *http.Request) { ... }
```

Returns:

```json
{
  "data": [
    {
      "slug": "trending",
      "name": "Trending",
      "sources": [ ... ]
    }
  ]
}
```

- Query-based collections: execute stored filters (e.g. `category=video&sort=popular&limit=10`).
- Manual collections: return ordered `source_collection_items`.

- [ ] **Step 3: Seed default collections**

Seed collections:
- `trending` — `sort=trending`
- `recently-added` — `sort=recent`
- `top-video-sources` — `category=video&sort=popular`
- `rising` — `sort=trending&created_after=7d`

- [ ] **Step 4: Update directory page**

`apps/main/src/routes/(app)/sources/+page.svelte`:
- Load `GET /api/v1/sources/collections`.
- Render each collection as a horizontal carousel or section, plus the existing category filter list.

- [ ] **Step 5: Add tests and commit**

- Test: collection API returns sources matching the collection query.
- Test: manual collections respect item order.

```bash
git commit -m "feat: curated source directory collections"
```

---

## Task 5: Auto-create `intel_sources` for entity profiles

**Files:**
- Modify: `internal/spaces/service.go` (CreateSpace)
- Modify: `internal/organizations/service.go` (CreateOrganization)
- Modify: `internal/users/service.go` (CreateUser or profile setup)
- Modify: `internal/intelligence/service.go`
- Test: `go test ./internal/spaces/... ./internal/organizations/... ./internal/users/...`

**Interfaces:**
- Produces: universities, organizations, and users that are publishers can auto-create a linked `intel_sources` row.

- [ ] **Step 1: Add helper to create source from entity**

In `internal/intelligence/service.go`:

```go
func (s *Service) CreateSourceFromEntity(ctx context.Context, tenantID, entityID pgtype.UUID, entityType, name, slug, sourceURL, category string) (pgtype.UUID, error) { ... }
```

Rules:
- Generate `intel_sources` row with `source_kind='institution'` or `'individual'`, `category` derived from entity type (university -> `university`, org -> `community_organisation` or `business`, user -> `news` or `culture_lifestyle` if a content creator).
- Set `slug` from entity slug.
- Link entity table's `source_id` to the new `intel_sources.id`.

- [ ] **Step 2: Wire into entity creation**

- When a `space` with `space_type='university'` is created, optionally call `CreateSourceFromEntity` if `create_source=true` is passed or a system setting `sources.auto_create_for_universities` is `true`.
- Same for `organizations` and `users` (for verified content creators / media orgs).

- [ ] **Step 3: Add admin toggle**

Add system settings:
- `sources.auto_create_for_universities` (default `false`)
- `sources.auto_create_for_organizations` (default `false`)
- `sources.auto_create_for_users` (default `false`)

Document in `.jeos/conventions/system-settings-catalog.md`.

- [ ] **Step 4: Add tests and commit**

- Test: creating a university with the setting enabled creates a linked `intel_sources` row.
- Test: source directory shows the new university source.

```bash
git commit -m "feat: auto-create intel_sources for verified entity profiles"
```

---

## Task 6: Richer source profiles

**Files:**
- Create: `migrations/0007XX_source_profile_extensions.up.sql`, `.down.sql`
- Modify: `internal/intelligence/handlers/sources_public.go`
- Modify: `apps/main/src/routes/(app)/sources/[slug]/+page.svelte`
- Test: `go test ./internal/intelligence/handlers/...`

**Interfaces:**
- Produces: extended source fields: `social_links`, `tags`, `language`, `country`, `founded_at`, `stats` (total views, total engagements, last crawl status).

- [ ] **Step 1: Add migration**

```sql
-- migrations/0007XX_source_profile_extensions.up.sql
BEGIN;

ALTER TABLE intel_sources
    ADD COLUMN IF NOT EXISTS social_links jsonb NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS language text,
    ADD COLUMN IF NOT EXISTS country_code text,
    ADD COLUMN IF NOT EXISTS tags jsonb NOT NULL DEFAULT '[]';

COMMIT;
```

- [ ] **Step 2: Update source detail API**

`HandleGetSource` now returns:

```json
{
  "social_links": { "twitter": "...", "youtube": "..." },
  "language": "en",
  "country_code": "US",
  "tags": ["node.js", "javascript"],
  "last_crawl_status": "success",
  "stats": {
    "total_views": 12000,
    "total_engagements": 450
  }
}
```

- [ ] **Step 3: Update source detail UI**

- Add social icons, country/language pills, tags, and stats cards.
- Add an "About" tab with source metadata (license, attribution, crawl frequency, last crawled).

- [ ] **Step 4: Add tests and commit**

- Test: `GET /sources/{slug}` includes extended fields.
- Test: admin can update `social_links` and `tags`.

```bash
git commit -m "feat: richer source profiles"
```

---

## Verification

- [ ] Run `go test ./internal/intelligence/... ./internal/sources/... ./internal/spaces/... ./internal/organizations/... ./internal/users/... ./internal/video/... ./internal/jce/...`
- [ ] Run `bun run check` in `apps/main` and `apps/tools`.
- [ ] Manual QA:
  - Propose a source; verify it appears in admin "Proposed sources".
  - Approve a proposed source; verify it appears in `/sources`.
  - Block a source; verify its items disappear from radar and source feed.
  - Link a video source and a comparison source; verify they appear in `/sources`.
  - Verify curated collections render on `/sources`.
  - Create a university with auto-source enabled; verify source directory includes it.
  - Verify richer source profile fields render.

---

## Out of scope for v3+

- Source-level notifications ("new post from a source you follow").
- Source revenue / sponsorship features.
- Source reputation scoring beyond `trust_level`.
- Machine-learning source recommendations.
- Source federation / external source imports (OPML, RSS OPML upload).
