# Section 25 — Ad Placement: Implementation Complete

**Date:** 2026-06-27
**Phase:** 5
**Section:** 25
**Status:** Complete

## Overview

Section 25 implements the Ad Placement system for JapaConnect, providing a
complete advertising infrastructure with campaign management, batched
impression/click event ingestion, daily analytics aggregation, and
organization boosts. The system defines five predefined slot types
(feed_inline, sidebar, search_sponsored, comparison_sponsored, forum_inline)
that surface eligible campaigns across the platform.

Campaigns progress through a lifecycle of draft → active → paused → ended,
with per-user frequency capping checked at render time. Ad events (impressions
and clicks) are ingested via a batched endpoint designed for asynchronous
background processing rather than synchronous writes on every page view.
Organization boosts reuse the same campaign infrastructure, linking
organizations to promoted placements via boost records.

V1 is admin-only for campaign management; self-serve ad creation for users is
deferred to V2.

## Components Implemented

### Migration 000024 — Advertising

1 migration file (`migrations/000024_advertising.sql`, 111 lines); 5 tables
created:

**ad_slot_types** — Registry of ad placement locations. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `slot_name` — Unique name per tenant (text, not null)
- `description` — Human-readable description (text, nullable)
- `max_display_per_page` — Maximum ads per page (int, not null, default `1`)
- `is_active` — Enable/disable flag (boolean, not null, default `true`)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(tenant_id, slot_name)`
- Indexes: `idx_ad_slot_types_tenant`, `idx_ad_slot_types_name`

**ad_campaigns** — Ad campaigns with budgeting and scheduling. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `name` — Campaign display name (text, not null)
- `slot_type_id` — FK to `ad_slot_types`
- `content` — Creative/ad content payload (jsonb, not null, default `'{}'`)
- `target_url` — Click-through destination (text, nullable)
- `budget` — Total campaign budget in kobo/cents (bigint, nullable = unlimited)
- `spent` — Total spent so far (bigint, not null, default `0`)
- `status` — Lifecycle: `draft`, `active`, `paused`, `ended`, `rejected` (text, not null, default `'draft'`)
- `priority` — Higher value = served first (int, not null, default `0`)
- `frequency_cap_per_user` — Max impressions per user/day (int, nullable)
- `start_date` — Campaign start (timestamptz, nullable)
- `end_date` — Campaign end (timestamptz, nullable)
- `created_by` — FK to `users`
- `created_at` / `updated_at` — Timestamps (default `now()`)
- Indexes: `idx_ad_campaigns_tenant`, `idx_ad_campaigns_slot`, `idx_ad_campaigns_status`, `idx_ad_campaigns_dates`

**ad_events** — Impression and click event log. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `campaign_id` — FK to `ad_campaigns` with `ON DELETE CASCADE`
- `user_id` — FK to `users` with `ON DELETE CASCADE`
- `event_type` — CHECK constraint: `'impression'` or `'click'` (text, not null)
- `occurred_at` — When the event happened (timestamptz, not null, default `now()`)
- `created_at` — Ingestion timestamp (default `now()`)
- Indexes: `idx_ad_events_campaign`, `idx_ad_events_user`, `idx_ad_events_tenant`
- Partial index: `idx_ad_events_user_campaign_impression` on `(user_id, campaign_id, occurred_at)` WHERE `event_type = 'impression'` — optimized for frequency capping lookups

**ad_analytics_daily** — Pre-aggregated daily analytics. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `campaign_id` — FK to `ad_campaigns` with `ON DELETE CASCADE`
- `slot_type_id` — FK to `ad_slot_types`
- `date` — Analytics date (date, not null)
- `impressions` — Count (int, not null, default `0`)
- `clicks` — Count (int, not null, default `0`)
- `spent` — Spend in kobo/cents (bigint, not null, default `0`)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(tenant_id, campaign_id, slot_type_id, date)`
- Indexes: `idx_ad_analytics_campaign_date`, `idx_ad_analytics_tenant`

**organization_boosts** — Links organizations to promoted campaigns. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `organization_id` — FK to `users` with `ON DELETE CASCADE`
- `campaign_id` — FK to `ad_campaigns` with `ON DELETE CASCADE`
- `boost_type` — Type discriminator (text, not null, default `'featured_placement'`)
- `status` — `active`, `ended`, or `canceled` (text, not null, default `'active'`)
- `start_date` — Start timestamp (timestamptz, not null, default `now()`)
- `end_date` — End timestamp (timestamptz, nullable)
- `created_at` — Creation timestamp (default `now()`)
- Indexes: `idx_organization_boosts_org`, `idx_organization_boosts_campaign`, `idx_organization_boosts_tenant`

### sqlc Queries (internal/db/queries/advertising.sql)

25 queries in 1 SQL file (195 lines total):

**Slot Types (6 queries):**
1. `CreateAdSlotType` — create a new slot type
2. `GetAdSlotTypeByID` — get slot type by ID
3. `GetAdSlotTypeByName` — get slot type by name (used for render resolution)
4. `ListAdSlotTypes` — paginated slot types for a tenant
5. `ListActiveAdSlotTypes` — active-only slot types (name-ordered)
6. `UpdateAdSlotType` — update display params and active flag

**Campaigns (7 queries):**
7. `CreateAdCampaign` — create campaign with optional budget, dates, frequency cap
8. `GetAdCampaignByID` — get campaign by ID
9. `ListAdCampaignsByTenant` — paginated campaigns (created_at DESC)
10. `ListActiveCampaignsBySlot` — active campaigns matching a slot, with date range filtering, ordered by priority DESC
11. `UpdateAdCampaignStatus` — transition status (draft → active, etc.)
12. `UpdateAdCampaign` — update campaign metadata
13. `IncrementCampaignSpent` — atomic spend increment for budget tracking

**Events (4 queries):**
14. `InsertAdEvent` — insert a single impression or click event
15. `BatchInsertAdEvents` — `:copyfrom` batch insert for high-throughput ingestion
16. `CountImpressionsForUserToday` — frequency capping count (today only, per user/campaign/impression)
17. `ListAdEventsByCampaign` — paginated events for a campaign

**Analytics (3 queries):**
18. `UpsertAdAnalyticsDaily` — insert or UPSERT daily aggregation (ON CONFLICT adds to existing)
19. `GetAdAnalyticsByCampaign` — paginated analytics for a campaign
20. `GetAdAnalyticsBySlot` — analytics filtered by slot type and date range

**Organization Boosts (5 queries):**
21. `CreateOrganizationBoost` — create boost linked to campaign
22. `GetOrganizationBoostByID` — get boost by ID
23. `ListOrganizationBoosts` — boosts for a specific organization
24. `ListActiveBoosts` — all active boosts for a tenant (date-filtered)
25. `EndOrganizationBoost` — set status to `ended` with end_date = now()

### Package: internal/advertising/ (5 files)

**advertising.go** (68 lines)

- Slot name constants: `SlotFeedInline`, `SlotSidebar`, `SlotSearchSponsored`, `SlotComparisonSponsored`, `SlotForumInline`
- Campaign status constants: `CampaignStatusDraft`, `CampaignStatusActive`, `CampaignStatusPaused`, `CampaignStatusEnded`, `CampaignStatusRejected`
- Event type constants: `EventTypeImpression`, `EventTypeClick`
- Boost status constants: `BoostStatusActive`, `BoostStatusEnded`, `BoostStatusCanceled`
- Boost type constant: `BoostTypeFeaturedPlacement`
- Pagination defaults: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor

**store.go** (428 lines)

- `Store` struct with a single `*db.DB` field (until sqlc generates query types)
- `NewStore` constructor
- 5 inline struct types: `AdSlotType`, `AdCampaign`, `AdEvent`, `AdAnalyticsDaily`, `OrganizationBoost` — using `pgtype.UUID`, `pgtype.Timestamptz`, and `pgtype.Date` for nullable columns
- 12 raw SQL methods using `s.write.Reader` and `s.write.Writer`:
  - Slot types: `listAdSlotTypes`, `getAdSlotTypeByName`
  - Campaigns: `createAdCampaign`, `getAdCampaignByID`, `listAdCampaignsByTenant`, `listActiveCampaignsBySlot`, `updateAdCampaignStatus`
  - Events: `insertAdEvent`, `countImpressionsForUserToday`
  - Organization boosts: `createOrganizationBoost`, `listOrganizationBoosts`, `listActiveBoosts`, `endOrganizationBoost`
- All errors wrapped with `fmt.Errorf("action noun: %w", err)` for context
- Nil-slice safety: all list methods return `[]T{}` instead of `nil` when result set is empty
- Pagination clamped to `DefaultPageSize`/`MaxPageSize` bounds

**service.go** (246 lines)

- `CreateCampaignInput` struct — name, slot type name, content map, target URL, budget, priority, frequency cap, start/end dates
- `CreateCampaign` — validates name and slot_type_name, resolves slot type by name, marshals content to JSON, creates campaign in draft status
- `GetCampaign` / `ListCampaigns` — pass-through to store
- `ActivateCampaign` — validates current status is `draft` or `paused` before transitioning to `active`
- `PauseCampaign` / `EndCampaign` — status transitions via store
- `RenderSlot` — resolves slot type by name, fetches active campaigns ordered by priority, applies per-user frequency capping, returns empty slice on no match (never a placeholder)
- `IngestAdEventInput` struct — campaign_id, user_id, event_type, occurred_at
- `IngestAdEvents` — batch ingestion with type validation (skips non-impression/click events), UUID scanning, per-event insert loop; returns count of actually ingested events
- `ListSlotTypes` — pass-through to store
- `CreateBoostInput` struct — organization_id, campaign_id, boost_type, start/end dates
- `CreateBoost` — validates required IDs, defaults boost_type to `featured_placement`, creates boost in `active` status
- `ListBoosts` / `ListActiveBoosts` / `EndBoost` — pass-through to store

**handlers.go** (607 lines)

- `Handlers` struct with `*Service` dependency; `NewHandlers` constructor
- Shared helpers: `parseUUIDParam`, `requireTenant`, `requireUser`, `handleServiceError` (maps `pgx.ErrNoRows` to 404, `already_exists`/`duplicate` to 409, others to 500), `writeJSON`, `writeError`, `parsePageSize`, `parsePageSizeOffset`
- 12 HTTP handlers:
  1. `CreateCampaign` — POST, validates name/slot_type_name/content, parses RFC3339 dates, delegates to service
  2. `GetCampaign` — GET, extracts `{id}` param, delegates to service
  3. `ListCampaigns` — GET, pagination via query params, delegates to service
  4. `ActivateCampaign` — POST, extracts `{id}` param, delegates to service
  5. `PauseCampaign` — POST, extracts `{id}` param, delegates to service
  6. `EndCampaign` — POST, extracts `{id}` param, delegates to service
  7. `RenderSlot` — GET, extracts `?slot=` query param, resolves user from context for frequency capping
  8. `IngestEvents` — POST, decodes batch events array, filters by valid event types, delegates to service, returns `{ ingested, total }`
  9. `ListSlotTypes` — GET, pagination via query params, delegates to service
  10. `CreateBoost` — POST, validates organization_id/campaign_id, delegates to service
  11. `ListBoosts` — GET, requires `?organization_id=` query param, delegates to service
  12. `ListActiveBoosts` — GET, delegates to service
  13. `EndBoost` — POST, extracts `{id}` param, delegates to service
- All responses in JapaConnect API envelope: `{"data": ...}` for success, `{"error": {"code": "...", "message": "..."}}` for errors

**advertising_test.go** (104 lines, 7 unit tests)

- `TestSlotConstants` — verifies all 5 slot name constants
- `TestCampaignStatusConstants` — verifies 5 statuses are non-empty and distinct
- `TestEventTypeConstants` — verifies impression and click constants
- `TestBoostConstants` — verifies 3 boost statuses and featured_placement constant
- `TestPaginationDefaults` — verifies DefaultPageSize = 20, MaxPageSize = 100
- `TestNewService` — verifies Service and Store are non-nil on construction
- `TestNewHandlers` — verifies Handlers and Service reference on construction

### Server Wiring (internal/server/server.go)

- `advertising.NewService(database)` and `advertising.NewHandlers(adService)` created in `NewServer`
- Routes registered within the tenant-scoped router group after wallet routes
- 4 new permission keys added to `bootstrapPermissions`

### Permission Keys

4 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `ad.campaign.manage` | Manage advertising campaigns — create, list, get, activate, pause, end campaigns and manage organization boosts |
| `ad.slot.manage` | Manage advertising slot types — list, create, update slot type definitions |
| `ad.stats.view` | View advertising analytics — access dashboard and aggregated reports |
| `ad.event.track` | Track ad impressions and clicks — render ads and ingest event batches |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/advertising | 7 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/advertising/ -count=1` | ok — 7 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

13 routes registered within the tenant-scoped router group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/admin/advertising/campaigns` | Required | `ad.campaign.manage` | Create ad campaign |
| 2 | GET | `/admin/advertising/campaigns` | Required | `ad.campaign.manage` | List all campaigns |
| 3 | GET | `/admin/advertising/campaigns/{id}` | Required | `ad.campaign.manage` | Get campaign by ID |
| 4 | POST | `/admin/advertising/campaigns/{id}/activate` | Required | `ad.campaign.manage` | Activate campaign |
| 5 | POST | `/admin/advertising/campaigns/{id}/pause` | Required | `ad.campaign.manage` | Pause campaign |
| 6 | POST | `/admin/advertising/campaigns/{id}/end` | Required | `ad.campaign.manage` | End campaign |
| 7 | GET | `/ads/render` | Required | `ad.event.track` | Render eligible ads for a slot |
| 8 | POST | `/ads/events/ingest` | Required | `ad.event.track` | Batch ingest ad events |
| 9 | GET | `/admin/advertising/slot-types` | Required | `ad.slot.manage` | List slot types |
| 10 | POST | `/admin/advertising/boosts` | Required | `ad.campaign.manage` | Create organization boost |
| 11 | GET | `/admin/advertising/boosts` | Required | `ad.campaign.manage` | List boosts for organization |
| 12 | GET | `/admin/advertising/boosts/active` | Required | `ad.campaign.manage` | List all active boosts |
| 13 | POST | `/admin/advertising/boosts/{id}/end` | Required | `ad.campaign.manage` | End an active boost |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000024_advertising.sql` | 111 | 5 tables for ad campaigns, slot types, events, analytics, boosts |
| 2 | `internal/db/queries/advertising.sql` | 195 | 25 sqlc queries |
| 3 | `internal/db/gen/advertising.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/advertising/advertising.go` | 68 | Constants (slot names, campaign statuses, event types, boost types), Service struct |
| 5 | `internal/advertising/store.go` | 428 | Store layer with inline struct types and raw pgx queries |
| 6 | `internal/advertising/service.go` | 246 | Business logic: campaigns CRUD, slot rendering, event ingestion, boosts |
| 7 | `internal/advertising/handlers.go` | 607 | 12 HTTP handlers |
| 8 | `internal/advertising/advertising_test.go` | 104 | 7 unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `advertising` import; created adService and adHandlers; registered 13 routes with auth and RBAC middleware; added 4 permission keys to `bootstrapPermissions` |

## Key Decisions

- **Campaign infrastructure shared with organization boosts** — organization
  boosts reuse the `ad_campaigns` table via `campaign_id` FK, rather than
  duplicating the campaign concept in a separate subsystem. A boost is a
  link between an organization and a campaign, inheriting all the campaign's
  scheduling, budgeting, and slot targeting logic.

- **Admin-only campaign management in V1** — all `/admin/advertising/*` routes
  require the `ad.campaign.manage` permission. Self-serve ad creation for
  users (where organizations buy ad slots directly via credits) is deferred
  to V2. This keeps the V1 surface area manageable while validating the
  rendering and event pipeline.

- **Store uses raw pgxpool queries with inline struct types** — the store
  methods were written before `sqlc generate` was run for the advertising
  queries. Once the generated code is available, the store should migrate
  to `internal/db/gen` typed queries for compile-time safety.

- **Ad rendering returns empty slice when no eligible campaign matches** —
  the `RenderSlot` method returns `[]AdCampaign{}` (never nil, never an error
  with nil campaigns) when no active campaign matches a slot. The frontend
  should check `len == 0` and skip rendering — no placeholder, no "Advertise
  here" fallback.

- **Batched impression/click ingestion** — ad events are ingested via
  `POST /ads/events/ingest`, designed to be called from a background worker
  or client-side batching, not synchronously on every page view. This avoids
  per-request database write overhead and allows the `:copyfrom` optimization
  path when implemented in the service layer.

- **Frequency capping checked at render time** — when `RenderSlot` is called
  with a `userID`, it queries `countImpressionsForUserToday` via the partial
  index `idx_ad_events_user_campaign_impression`. Campaigns whose cap is
  exceeded are filtered out before the response. This ensures users don't
  hit a "no ads" dead zone while still enforcing caps.

## Known Issues and Deferrals

- **Migration 000024 needs to be run against the database** — the migration
  file exists but has not been applied; `ad_slot_types` must be seeded per
  tenant with the five predefined slot types. The seed INSERT statements are
  present in the migration as comments but use a placeholder `@default_tenant_id`
  that must be resolved at deployment.

- **Store methods use raw SQL instead of sqlc-generated queries** — the
  advertising sqlc queries exist in `internal/db/queries/advertising.sql`
  and the generated code is available, but the store layer still uses raw
  `pgxpool` queries with inline struct types. Migrating to the generated
  types will improve type safety and eliminate manual scanning.

- **Ad analytics aggregation not yet integrated with Asynq** — the
  `UpsertAdAnalyticsDaily` query and `ad_analytics_daily` table exist, but
  no Asynq scheduled task runs the aggregation from `ad_events` to the
  daily rollup table. Analytics dashboards rely on direct `ad_events`
  queries until this task is wired.

- **Campaign budget auto-pausing deferred to Phase 6** — when `spent >=
  budget` (for campaigns with a budget set), no automatic transition to
  `paused` or `ended` occurs. The `IncrementCampaignSpent` query exists
  but is not called in the event ingestion pipeline. Budget enforcement
  is manual in V1.

- **Self-serve ad creation for users (V2) not yet implemented** — users
  cannot create or manage their own campaigns. The credit-spending flow
  (user buys ad slot → credits deducted → campaign created) requires
  Payment Core integration with the Wallet & Credits system (Section 28)
  and is deferred to V2.

- **`BatchInsertAdEvents` (`:copyfrom`) optimization not yet used** — the
  sqlc query exists but the service layer still inserts events one at a
  time in a loop. Switching to `:copyfrom` will significantly reduce
  database round-trips for bulk ingestion.

## Next Steps

Credit Provider integration into Payment Core (Sections 17/28) — this
enables wallet balance checks and credit deductions at checkout, which is
a prerequisite for self-serve ad purchases in V2.