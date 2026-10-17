# JapaConnect Sources Directory & Source Detail

**Date:** 2026-09-11  
**Status:** Draft — awaiting review  
**Scope:** Public `/sources` directory, `/sources/[slug]` source detail pages, source attribution across tools and content, and source-driven content discovery.  
**Approach:** Hybrid — `intel_sources` remains the canonical publisher/data-source registry; universities, organizations, and user profiles remain in `spaces`, `organizations`, and `users` but are discoverable from `/sources` via an optional `source_id` link.  
**Refs:** ADR-058 (app shell), ADR-112 (intelligence spine), .jeos/sections/14-content-type-system.md, .jeos/sections/73-scholarship-finder.md, .jeos/sections/74-remittance-rate-tracker.md, .jeos/AGENTS.md Rules 3, 4, 12, 16, 17

---

## Background

daily.dev’s `/sources` and `/sources/[slug]` surfaces expose a public directory of content publishers. A source page shows the publisher’s profile, a follow action, related tags, and a feed of posts that open inside daily.dev with a link back to the publisher.

JapaConnect already has the equivalent publisher registry in `intel_sources` and the intelligence spine that promotes records into scholarships, jobs, remittance rates, visa routes, wiki pages, news, and content. What is missing is the public directory and source-detail UX, plus the cross-surface attribution links that let users trust and follow a source across the platform.

This design makes `/sources` the canonical discovery surface for all publishers and data sources, while keeping universities, organizations, and user profiles in their own tables and linking them in.

---

## Goals

1. Provide a public `/sources` directory grouped by category and source kind.
2. Provide a `/sources/[slug]` detail page with source metadata, follow action, and a feed of items from that source.
3. Every item in the source feed opens inside JapaConnect (content, scholarship, job, remittance rate, video, etc.) and carries a back-link to its source.
4. Surface source attribution in tools (Scholarship Finder, Remittance Tracker, Jobs, etc.).
5. Link entity profiles (universities, organizations, users) to their source without duplicating profile data.

---

## Decisions

1. **`intel_sources` is the canonical source registry.** It already stores publisher metadata, category, source type, trust level, crawl settings, and provenance for the intelligence spine. We extend it with public-facing fields rather than creating a parallel `sources` table.
2. **Entity profiles are not duplicated.** Universities, organizations, and users stay in `spaces`, `organizations`, and `users`. An optional `source_id` foreign key on those tables links them to an `intel_sources` row when the entity is also a publisher/data source.
3. **Source detail page lives at `/sources/[slug]`.** Even when a source is linked to a university or organization, the source page is the primary landing. It shows the source’s item feed and a “View [university/org/profile]” link. This matches daily.dev’s source-first UX and keeps the `/sources` surface consistent.
4. **Items open inside JapaConnect.** A source feed card links to the promoted JapaConnect entity (`/content/[slug]`, `/scholarships/[guide_id]`, `/jobs/[slug]`, `/tools/remittance`, `/universities/[slug]`, etc.). The target page displays a `Source: [name]` link back to `/sources/[slug]` and, when applicable, an “Originally from [domain]” external link.
5. **Source following uses the existing follow system.** `follows.entity_type` is extended to include `'source'`, or a dedicated `user_source_follows` table is used if the `CHECK` constraint is too disruptive to alter.
6. **Content provenance is explicit.** `contents` gets `intel_source_id` and `intel_record_id` so source-syndicated posts can be traced, attributed, and surfaced on source pages.

---

## Data model

### `intel_sources` extensions

- `slug` — unique per tenant, URL-safe, indexed. Used for `/sources/[slug]`.
- `description` — public one-line / short description.
- `logo_url` or `logo_media_id` — public logo.
- `follower_count` — cached follow count (updated by the follow toggle service).
- `post_count` — cached count of published items from this source (updated by the promoter when records are published or removed).
- `source_kind` — `publisher` | `institution` | `individual` | `aggregator`.
- `status` / `enabled` — existing; public directory only lists `enabled=true AND status='active'`.
- `category` — existing; drives the domain/feature table a record promotes into.
- `source_type` — existing; drives content format (rss, api, youtube, static, newsletter, etc.).

### Entity table links

- `spaces.source_id` — optional FK to `intel_sources(id) ON DELETE SET NULL`. Enables universities and geo spaces to be discovered from `/sources`.
- `organizations.source_id` — optional FK to `intel_sources(id) ON DELETE SET NULL`.
- `users.source_id` — optional FK to `intel_sources(id) ON DELETE SET NULL`.

The link is optional. Not every university/organization/user is a publisher; only those with an associated data feed get a `source_id`.

### `contents` provenance

- `contents.intel_source_id` — FK to `intel_sources(id) ON DELETE SET NULL`.
- `contents.intel_record_id` — FK to `intel_records(id) ON DELETE SET NULL`.

This lets `FeedCard` display a source attribution line and lets the source detail page query a source’s content posts directly.

### Follows

Option A (preferred): extend the existing `follows` table `entity_type` `CHECK` constraint to include `'source'`:

```sql
entity_type text NOT NULL CHECK (entity_type IN ('user','space','tag','organization','professional_profile','source'))
```

The unique key `(follower_id, entity_type, entity_id)` and the existing `follows.toggle` permission continue to work.

Option B: a separate `user_source_follows` table if the polymorphic constraint change is too risky for v1.

---

## Public API

### `GET /api/v1/sources`

Public directory of active sources.

Query params:
- `?category=` — scholarship, job, visa, remittance, news, cost_of_living, university, etc.
- `?source_kind=` — publisher, institution, individual, aggregator.
- `?q=` — text search on name, description, domain.
- `?sort=` — popular (follower_count), recent (created_at), trending (follower growth, created ≤ 30d), rising (trending restricted to created ≤ 7d).
- `?limit=` / `?offset=` or cursor pagination.

Response:
```json
{
  "data": [
    {
      "id": "...",
      "slug": "nodejs",
      "name": "Node.js",
      "description": "The Node.js blog",
      "domain": "nodejs.org",
      "source_url": "https://nodejs.org/en/blog/",
      "category": "news",
      "source_type": "rss",
      "source_kind": "publisher",
      "trust_level": 2,
      "follower_count": 1240,
      "post_count": 89,
      "logo_url": "...",
      "linked_entity": null
    }
  ],
  "meta": { "limit": 20, "offset": 0, "total": 150 }
}
```

For sources linked to an entity, `linked_entity` contains `{ type: 'space'|'organization'|'user', slug: '...', name: '...' }`.

### `GET /api/v1/sources/:slug`

Source profile.

Response: source metadata + `linked_entity` (if any) + `is_following` (for authed users) + related tags/categories.

### `GET /api/v1/sources/:slug/items`

Feed of published items from this source.

Query params:
- `?entity_type=` — filter by one or more promoted types (scholarship, job, remittance, content, video, etc.).
- `?sort=` — latest, popular, discussed, upvoted.
- `?limit=` / `?offset=` or cursor pagination.

Response:
```json
{
  "data": [
    {
      "id": "...",
      "entity_type": "scholarship",
      "title": "Chevening Scholarships 2026",
      "excerpt": "...",
      "original_url": "https://...",
      "published_at": "...",
      "vote_score": 42,
      "comment_count": 7,
      "view_count": 1203,
      "href": "/scholarships/chevening-2026",
      "source": { "name": "Chevening", "slug": "chevening" }
    }
  ],
  "meta": { ... }
}
```

The backend resolves `intel_records` by `source_id` and joins the promoted table (scholarships, jobs, remittance_rates, contents, video_sources, etc.) to produce the target `href` and engagement metrics.

### `POST /api/v1/sources/:slug/follow` / `DELETE /api/v1/sources/:slug/follow`

Toggle source follow. Requires auth and `follows.toggle` permission.

---

## Main app surfaces

### `/sources` directory

- Route: `apps/main/src/routes/(app)/sources/+page.server.ts` and `+page.svelte`.
- Renders inside `AppShell`.
- **Curated collection rails are removed.** The v2 plan (Task 4) rendered
  `GET /api/v1/sources/collections` as horizontal sections above the grid;
  this was intentionally removed — the page is only the sort-tab grid
  (Popular · Recently added · Trending · Rising). Do not re-add collection
  sections to this page; the endpoint remains for other consumers. "Rising"
  is served as a `?sort=` value, not a collection.
- No result-count line ("N sources") above the grid — removed with the
  collections cleanup; do not re-add.
- Filter tabs: All · Universities · Scholarships · Jobs · Remittance · News · Videos · Organizations · People.
- Uses `DirectoryCard` with source logo, name, category badge, trust pill, follower/post counts, and a `FollowButton`.
- A source linked to a university or organization shows its entity badge (e.g. “University” or “Organization”).

### `/sources/[slug]` detail

- Route: `apps/main/src/routes/(app)/sources/[slug]/+page.server.ts` and `+page.svelte`.
- Header:
  - source logo, name, domain, category pill, trust level, follow button.
  - description.
  - related tags (linking to `/tags/[slug]`).
  - if linked to an entity, a card linking to `/universities/[slug]`, `/org/[slug]`, or `/u/[username]`.
- Tab bar:
  - All
  - by entity type: Scholarships · Jobs · Remittance · Contents · Videos · News
  - About (source metadata: license, attribution, crawl frequency, last crawled).
- Item list:
  - Each item is a `FeedCard` variant or a domain-specific card.
  - Each card links to the JapaConnect internal page for that item.
  - Each card shows the item’s source attribution and, when applicable, an external link icon.

### Source attribution on internal pages

- Content detail (`/content/[slug]`, `/blog/[slug]`) shows `Source: [Name]` linking to `/sources/[slug]` and `Originally from [domain]` linking to `canonical_url`.
- Scholarship detail shows the same source attribution block.
- Job, remittance, and other tool detail pages include a compact source link.

---

## Tool integration

Tools continue to consume their existing domain endpoints. The change is to expose source attribution on the result cards and add a “Data sources” link.

1. **API payload enrichment.** Responses for `/scholarships`, `/jobs`, `/remittance/rates`, etc. include a lightweight `source` object: `{ name, slug, logo_url, trust_level, domain }`.
2. **Source links on result cards.** Each result shows a small attribution row: “From [Source Name] · [trust icon]” linking to `/sources/[slug]`.
3. **“Data sources” link.** Each tool page header includes a link to `/sources?category=<domain>` pre-filtered to that tool’s category.
4. **Cross-app links.** `apps/tools` is a separate SvelteKit app; it links to `main.japa.to/sources/[slug]` for source detail and discovery.

---

## Content / feature mapping

The source `category` and `entity_type` already determine where a record lands. We make that mapping public and explicit.

| `intel_sources.category` | `intel_records.entity_type` | Promoted table | JapaConnect page | Card type |
|---|---|---|---|---|
| scholarship | scholarship | `scholarships` | `/scholarships/[guide_id]` or `/tools/scholarships/[guide_id]` | Scholarship card |
| job | job | `jobs` | `/jobs/[slug]` | Job card |
| remittance | remittance | `remittance_rates` | `/tools/remittance` (contextual corridor) | Rate row |
| visa / travel_alert | visa | `visa_routes` / `contents` | `/visa-routes/[id]` or `/guide/[slug]` | Guide card |
| news / culture_lifestyle | news / wiki_page | `contents` | `/content/[slug]` or `/blog/[slug]` | `FeedCard` |
| video / youtube | (video record) | `video_sources` + `contents` | `/content/[slug]` | Video card |
| university | university | `spaces` (type=university) | `/universities/[slug]` | University card |
| cost_of_living | cost_of_living | `cost_of_living` | `/tools/cost-of-living` | Cost row |
| housing | housing | `housing_guides` | `/tools/housing-guide/[id]` | Housing card |

A single source can publish multiple entity types. The detail page groups or filters by `entity_type`.

**Extraction-side support for mixed-domain sources.** By default `intel:extract` derives one entity type per source from `intel_sources.category` (via `CategoryToEntityType`) — the limitation this design flagged. To let one source produce records under several entity types, the operator declares an explicit override in `provider_config.entity_types` (a JSON array of registered extraction entity types, e.g. `["scholarship","job"]`). When present and non-empty, `intel:extract` runs one extraction pass per declared type and persists each pass's records under that pass's `entity_type`; every `intel_records` row keeps its own type, so the `?entity_type=` filter and the mixed "latest" feed need no further changes. An empty array (or absent key) restores the category-derived single-type behaviour. The override is managed through the first-class `entity_types` field on `POST/PATCH /api/v1/intel/sources` (the backend merges it into `provider_config` so sibling keys like an API source's `endpoint` are preserved) and is surfaced as `entity_types` on the admin source responses. The set of valid values is exposed on `GET /api/v1/sources/meta` as `registered_entity_types` so operator UIs never hardcode the registry.

---

## Data flow

```text
external source (rss / api / youtube / static)
  → intel:fetch_source
  → intel:extract
  → intel:dedup
  → intel:publish_gate
    → AUTO_PUBLISH → intel:promote
      → scholarships / jobs / remittance_rates / contents / video_sources / etc.
      → source detail page reads promoted records by source_id
    → REVIEW_REQUIRED → moderation queue

source page:
  GET /api/v1/sources/:slug
  GET /api/v1/sources/:slug/items (joins promoted table by entity_type)
  → renders FeedCard / domain card
  → click → internal entity page
  → internal page shows Source: [name] → /sources/:slug
```

Source feeds stay fresh through the existing intelligence scheduler: `crawl_frequency_mins` on `intel_sources` drives `intel:schedule_crawls`, which re-fetches, extracts, deduplicates, and promotes new records on the configured cadence.

---

## Migrations and catalog updates

### Migrations

1. `000651_extend_intel_sources.up.sql`
   - Add `slug`, `description`, `logo_url`, `follower_count`, `post_count`, `source_kind` to `intel_sources`.
   - Add unique index on `(tenant_id, slug)`.
2. `000652_link_entity_profiles_to_sources.up.sql`
   - Add `source_id` to `spaces`, `organizations`, and `users`.
3. `000653_contents_source_provenance.up.sql`
   - Add `intel_source_id` and `intel_record_id` to `contents`.
4. `000654_follows_source_entity.up.sql`
   - Extend `follows.entity_type` `CHECK` to include `'source'`, or create `user_source_follows`.
5. Backfill slugs for existing `intel_sources` from `name` (URL-safe + disambiguation).

### Catalogs

- `.jeos/conventions/api-routes-catalog.md` — add the new public source routes.
- `.jeos/conventions/valkey-keys-catalog.md` — if source following uses cache or counters.
- `.jeos/conventions/system-settings-catalog.md` — no new settings unless a feature flag is needed.
- `.jeos/sections/14-content-type-system.md` — note `contents.intel_source_id` / `intel_record_id`.
- `.jeos/sections/73-scholarship-finder.md` and related tool sections — note source attribution.

---

## Testing strategy

- Unit tests for new public source handlers (`GET /sources`, `GET /sources/:slug`, `GET /sources/:slug/items`).
- Integration test: a source with mixed `intel_records` (scholarship, content) returns the correct `entity_type` and `href` for each.
- Frontend test: source directory renders `DirectoryCard` list and source detail tab switching works.
- Follow toggle test: source follow/unfollow updates `follower_count` and `follows` row.
- Tool test: scholarship/remittance result cards render source link correctly.
- Content provenance test: a source-syndicated content post displays the source attribution block.

---

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `intel_sources` is in the default tenant; public access must not leak admin-only crawl state. | Public endpoints return only `sourceDirectoryEntry` fields; admin state is omitted. |
| Source slugs may collide across tenants or with existing routes. | Unique index on `(tenant_id, slug)`; slug generator appends a short hash on collision. |
| `follows` `CHECK` constraint extension affects existing data. | Migration updates the constraint; no data rewrite needed because new values are just a new enum member. |
| Entity profiles without `source_id` are invisible from `/sources`. | This is by design per the hybrid decision; the directory can still search/link entity profiles through `/discover` and `/search`. |
| Promoted records span many tables; the `GET /sources/:slug/items` query could become complex. | Build a source-specific union query in `internal/intelligence` that delegates to existing domain stores; add `?entity_type=` to keep result sets small. |
| Source attribution on every item could clutter tool UIs. | Render it as a compact under-line link, not a full card. |

---

## Files to change

### Backend
- `migrations/000651_extend_intel_sources.up.sql`
- `migrations/000652_link_entity_profiles_to_sources.up.sql`
- `migrations/000653_contents_source_provenance.up.sql`
- `migrations/000654_follows_source_entity.up.sql`
- `internal/db/queries/intelligence.sql` — add public source listing, detail, and item-feed queries.
- `internal/intelligence/handlers/sources_public.go` (new) — `HandleSourcesDirectory`, `HandleGetSource`, `HandleListSourceItems`, `HandleFollowSource`.
- `internal/server/server.go` — register public source routes.
- `internal/content/store.go` or service — set `intel_source_id`/`intel_record_id` on source-syndicated content.
- `internal/follows/*` — extend follow toggle and counts to support `source` entity type.
- Domain services (`internal/scholarships`, `internal/jobs`, `internal/remittance`, etc.) — include `source` object in list responses.

### Main app
- `apps/main/src/routes/(app)/sources/+page.server.ts` (new)
- `apps/main/src/routes/(app)/sources/+page.svelte` (new)
- `apps/main/src/routes/(app)/sources/[slug]/+page.server.ts` (new)
- `apps/main/src/routes/(app)/sources/[slug]/+page.svelte` (new)
- `apps/main/src/lib/components/feed/FeedCard.svelte` — add source attribution line.
- `apps/main/src/lib/components/content/ContentArticlePage.svelte` — add source attribution block.
- `apps/main/src/lib/types/index.ts` — add `Source`, `SourceItem`, etc.

### Tools app
- `apps/tools/src/routes/scholarships/+page.svelte` and related tool pages — render source attribution.
- `apps/tools/src/routes/remittance/+page.svelte` — render source attribution on rate rows.
- `apps/tools/src/lib/types/index.ts` — add `source` to relevant tool types.

### Catalogs
- `.jeos/conventions/api-routes-catalog.md`
- `.jeos/sections/14-content-type-system.md`
- `.jeos/sections/73-scholarship-finder.md`
- `.jeos/sections/74-remittance-rate-tracker.md`
