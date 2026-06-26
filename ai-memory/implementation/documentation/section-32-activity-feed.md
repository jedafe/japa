# Section 32 — Activity Feed / Timeline: Implementation Complete

**Date:** 2026-06-26
**Phase:** 3
**Section:** 32
**Status:** Complete

## Overview

Section 32 implements the Activity Feed / Timeline system for JapaConnect.
The feed supports four timelines (explore, primary, space, profile) and four
sort modes (latest, top, rising, relevant). A hybrid fan-out strategy
distributes posts to follower feeds: push mode writes feed rows for spaces
with ≤ 5000 members; pull mode (deferred to Phase 6) marks content in a hot
cache for larger spaces. Content ranking uses a velocity score based on the
Hacker News formula: `engagement / (hours_old + 2)^1.8`.

The section reuses the `content_scores` table from Section 14 (migration
000010) rather than creating a duplicate, keeping the schema DRY. A new
`feeds` table stores the personalized (push) feed per user.

## Components Implemented

### Migration 000014 — Activity Feed

1 migration file (`migrations/000014_activity_feed.sql`, 17 lines); 1 table
created:

1. **feeds** — Per-user feed rows for the push side of hybrid fan-out.
   Columns: `id` (PK), `tenant_id` (FK to tenants), `user_id` (FK to users),
   `content_id` (FK to contents), `space_id` (FK to spaces, nullable),
   `content_type` (text), `score` (numeric, default 0), `created_at`.
   `UNIQUE(user_id, content_id)` prevents duplicate feed entries. Three indexes:
   - `idx_feeds_user_created` — `(tenant_id, user_id, created_at DESC)` for
     primary feed pagination
   - `idx_feeds_user_score` — `(tenant_id, user_id, score DESC)` for
     score-sorted queries
   - `idx_feeds_space` — `(tenant_id, space_id, created_at DESC) WHERE
     space_id IS NOT NULL` partial index for space feeds

Conventions: `tenant_id` FK with `ON DELETE CASCADE`, `gen_random_uuid()` for
primary keys, `UNIQUE` constraints for natural keys, `idx_` prefixed indexes,
partial index with WHERE clause for nullable FK.

Design decisions:

- **content_scores reused from Section 14** — the `content_scores` table
  (migration 000010) already stores `vote_score`, `comment_count`,
  `view_count`, `bookmark_count`, and `velocity_score`. No duplicate table
  was created. The feed queries JOIN `content_scores` for ranking data.
- **feeds table is push-only** — the `feeds` table stores rows written during
  fan-out for small spaces. The pull side (large spaces) is deferred to Phase
  6 and will query `contents` + `content_scores` directly.
- **Partial index on space_id** — the `WHERE space_id IS NOT NULL` clause
  keeps the index small since most feed rows are user-scoped, not space-scoped.

### sqlc Queries (internal/db/queries/feeds.sql)

12 queries in 1 SQL file (118 lines total):

**Feed CRUD (1 query):**

1. `InsertFeedItem` — insert with `ON CONFLICT (user_id, content_id) DO
   NOTHING` (idempotent)

**Feed Reads (6 queries):**

2. `GetExploreFeed` — trending published content for guests, JOIN
   `content_scores`, ordered by `velocity_score DESC, vote_score DESC`
3. `GetPrimaryFeed` — user's personalized feed from the `feeds` table (push
   side), JOIN `contents` for title/slug/author, ordered by `created_at DESC`
4. `GetSpaceFeed` — published posts in a specific space, JOIN
   `content_scores`, ordered by `created_at DESC`
5. `GetProfileFeed` — all public published posts by a specific user, JOIN
   `content_scores`, ordered by `created_at DESC`
6. `GetTopFeed` — top content in a 7-day rolling window, filtered by
   `created_at > now() - interval '7 days'`, ordered by `vote_score DESC`
7. `GetRisingFeed` — rising content sorted by `velocity_score DESC`

**Score Updates (2 queries):**

8. `UpdateContentScore` — update vote/comment/view/bookmark counts and
   `last_scored_at` for a single content item
9. `UpdateVelocityScores` — batch update velocity scores for posts < 7 days
   old using the formula `engagement / (hours_old + 2)^1.8` via PostgreSQL
   `POWER()` function

**Cleanup (3 queries):**

10. `DeleteFeedItemsByContent` — remove all feed rows for a content ID
11. `DeleteFeedItemsByUser` — remove all feed rows for a user
12. `GetFeedItemCount` — count total feed items for a user

Generated code:

- `internal/db/gen/feeds.sql.go` (529 lines)
- New model struct in `models.go`: `Feed`
- 7 new param/result structs: `InsertFeedItemParams`, `GetExploreFeedParams`,
  `GetExploreFeedRow`, `GetPrimaryFeedParams`, `GetPrimaryFeedRow`,
  `GetSpaceFeedParams`, `GetSpaceFeedRow`, `GetProfileFeedParams`,
  `GetProfileFeedRow`, `GetTopFeedParams`, `GetTopFeedRow`,
  `GetRisingFeedParams`, `GetRisingFeedRow`, `UpdateContentScoreParams`

### Package: internal/feed/ (6 files)

**feed.go** (45 lines)

- Package doc describing the Activity Feed / Timeline system
- Sort mode constants: `SortLatest` ("latest"), `SortTop` ("top"),
  `SortRising` ("rising"), `SortRelevant` ("relevant")
- `FanOutThreshold = 5000` — member count at which fan-out switches from
  push to pull
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (168 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 11 private methods wrapping sqlc-generated queries:
  - `insertFeedItem` — idempotent feed row insertion
  - `getExploreFeed` — trending content for guests
  - `getPrimaryFeed` — personalized feed from feeds table
  - `getSpaceFeed` — space-scoped feed
  - `getProfileFeed` — user's public posts
  - `getTopFeed` — top content in 7-day window
  - `getRisingFeed` — velocity-sorted content
  - `updateContentScore` — single content score update
  - `updateVelocityScores` — batch velocity recalculation
  - `deleteFeedItemsByContent` / `deleteFeedItemsByUser` — cleanup
  - `getFeedItemCount` — count feed items for a user
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context
- Page size defaults applied in store layer (clamp to `DefaultPageSize` if
  invalid, cap at `MaxPageSize` if exceeded)

**service.go** (341 lines)

- `FeedItem` struct — normalized feed item across all timelines, with JSON
  tags for `id`, `title`, `slug`, `content_type`, `author_id`, `space_id`,
  `created_at`, `view_count`, `comment_count`, `vote_score`, `velocity_score`
- `PrimaryFeedItem` struct — personalized feed item from the push side
- 8 public methods:
  - `FanOutPost` — placeholder for Phase 6 Asynq task (currently no-op)
  - `GetFeed` — main entry point that routes to the correct timeline/sort
    combination via a switch statement
  - `UpdateVelocityScores` — batch velocity recalculation for posts < 7 days
  - `UpdateContentScore` — single content score update
  - `DeleteFeedItemsByContent` / `DeleteFeedItemsByUser` — cleanup operations
  - `GetFeedItemCount` — count feed items for a user
- `ComputeVelocity` — pure function implementing the HN velocity formula:
  `engagement / (hours_old + 2)^1.8`
- Row-to-item converters: `exploreRowsToItems`, `topRowsToItems`,
  `risingRowsToItems`, `primaryRowsToItems`, `spaceRowsToItems`,
  `profileRowsToItems` — map sqlc row types to the normalized `FeedItem`
- Helper functions: `uuidToString`, `nullableUUIDToString`,
  `pgNumericToFloat64`

**handlers.go** (203 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 4 HTTP handlers:
  - `GetExploreFeed` — `GET /api/v1/feed/explore`. Public endpoint, no auth
    required. Resolves tenant from context, parses pagination and sort params.
  - `GetPrimaryFeed` — `GET /api/v1/feed/primary`. Requires authentication.
    Resolves user from auth context.
  - `GetSpaceFeed` — `GET /api/v1/feed/space/{space_id}`. Requires
    authentication. Parses `space_id` from URL path.
  - `GetProfileFeed` — `GET /api/v1/feed/profile/{user_id}`. Requires
    authentication. Parses `user_id` from URL path.
- Helpers: `requireTenant`, `optionalUser`, `writeJSON`, `writeError`,
  `parsePageSize`, `parsePageOffset`, `parseSort`
- Uses `auth.FromContext`, `tenant.FromContext`, `chi.URLParam`, `pgtype.UUID`

**tasks.go** (19 lines)

- Task name constants: `TaskFanOutToFeeds` ("content:fan_out_to_feeds"),
  `TaskUpdateVelocityScores` ("maintenance:update_velocity_scores")
- `FanOutPayload` struct with `ContentID`, `SpaceID`, `ContentType`
- `UpdateVelocityPayload` struct with `TenantID`

### Server Wiring (internal/server/server.go)

- `feed.NewService(database)` and `feed.NewHandlers(feedService)` created
  in `NewServer`
- 4 API routes registered under `/api/v1`, within the tenant-scoped route
  group. The explore feed is public; the other three require auth.

## Test Summary

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
| internal/follows | 12 | PASS |
| internal/feed | 23 | PASS |
| internal/linkpreview | (existing) | PASS |
| internal/mentions | (existing) | PASS |
| internal/renderer | (existing) | PASS |
| **Total** | **154+** | **ALL PASS** |

### Activity Feed Tests (23 tests)

| # | Test | Coverage |
|---|------|----------|
| 1 | `TestSortModeConstants` | All 4 sort mode constants match expected values |
| 2 | `TestFanOutThreshold` | FanOutThreshold = 5000 |
| 3 | `TestPageSizeConstants` | DefaultPageSize=20, MaxPageSize=100 |
| 4 | `TestComputeVelocity` | 4 table-driven cases: fresh post, old post, no engagement, very new post |
| 5 | `TestComputeVelocityDenominatorGrowsWithAge` | Velocity decreases with age for same engagement |
| 6 | `TestComputeVelocityEngagementScalesLinearly` | Double engagement doubles velocity |
| 7 | `TestFeedItemStructFields` | All FeedItem fields serialize correctly |
| 8 | `TestFeedItemNilSpaceID` | SpaceID is nil when not set |
| 9 | `TestPrimaryFeedItemStructFields` | PrimaryFeedItem fields serialize correctly |
| 10 | `TestFanOutPayloadJSONTags` | FanOutPayload JSON serialization |
| 11 | `TestUpdateVelocityPayloadJSONTags` | UpdateVelocityPayload JSON serialization |
| 12 | `TestGetFeedUnknownTimeline` | Unknown timeline returns error |
| 13 | `TestGetFeedPrimaryRequiresUserID` | Primary feed with nil user_id returns error |
| 14 | `TestGetFeedSpaceRequiresSpaceID` | Space feed with nil space_id returns error |
| 15 | `TestGetFeedProfileRequiresAuthorID` | Profile feed with nil author_id returns error |
| 16 | `TestExploreRowsToItemsEmpty` | Empty input returns empty slice |
| 17 | `TestTopRowsToItemsEmpty` | Empty input returns empty slice |
| 18 | `TestRisingRowsToItemsEmpty` | Empty input returns empty slice |
| 19 | `TestPrimaryRowsToItemsEmpty` | Empty input returns empty slice |
| 20 | `TestSpaceRowsToItemsEmpty` | Empty input returns empty slice |
| 21 | `TestProfileRowsToItemsEmpty` | Empty input returns empty slice |
| 22 | `TestComputeVelocityScoreUpdateLogic` | Higher engagement yields higher velocity |
| 23 | `TestComputeVelocityScoreDecay` | Recent velocity > old velocity |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

4 routes registered under `/api/v1`, within the tenant-scoped route group:

| # | Method | Path | Auth | Description |
|---|--------|------|------|-------------|
| 1 | GET | `/feed/explore` | None (public) | Trending content for guests |
| 2 | GET | `/feed/primary` | Required | Personalized feed for logged-in user |
| 3 | GET | `/feed/space/{space_id}` | Required | Space-specific feed |
| 4 | GET | `/feed/profile/{user_id}` | Required | User's public posts |

All endpoints accept optional query parameters: `page_size` (default 20, max
100), `page_offset` (default 0), `sort` (default "latest"; valid values:
"latest", "top", "rising", "relevant").

## Permission Keys Registered

No new permission keys were added. The explore feed is intentionally public
(no auth required) to attract guest users. The authenticated endpoints
(primary, space, profile) are protected by `auth.RequireAuth` but do not
require specific permission keys — they rely on authentication only.

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000014_activity_feed.sql` | 17 | Feeds table with unique constraint and 3 indexes |
| 2 | `internal/db/queries/feeds.sql` | 118 | 12 sqlc queries for feed reads, writes, score updates, cleanup |
| 3 | `internal/db/gen/feeds.sql.go` | 529 | Generated query methods and param/result structs |
| 4 | `internal/feed/feed.go` | 45 | Sort mode constants, FanOutThreshold, Service struct |
| 5 | `internal/feed/store.go` | 168 | Store layer with read/write separation, 11 private methods |
| 6 | `internal/feed/service.go` | 341 | FeedItem types, GetFeed routing, ComputeVelocity, row converters |
| 7 | `internal/feed/handlers.go` | 203 | 4 HTTP handlers with pagination and sort parsing |
| 8 | `internal/feed/tasks.go` | 19 | Asynq task name constants and payload structs |
| 9 | `internal/feed/feed_test.go` | 365 | 23 unit tests covering constants, velocity math, structs, edge cases |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `feed` import; created feed service and handlers; registered 4 routes (1 public, 3 with auth) |
| 2 | `internal/db/gen/models.go` | Regenerated by `sqlc generate` to include `Feed` struct |

## Key Decisions

- **content_scores reused from Section 14** — the `content_scores` table
  already existed from migration 000010 with `vote_score`, `comment_count`,
  `view_count`, `bookmark_count`, and `velocity_score` columns. No duplicate
  table was created. Feed queries JOIN `content_scores` for ranking data.
- **Fan-out threshold: 5000 members** — spaces with ≤ 5000 members use push
  mode (write feed rows to the `feeds` table). Larger spaces use pull mode
  (followers query content directly on next request). The threshold is a
  constant that can be tuned without a migration.
- **Velocity formula: HN-style** — `engagement / (hours_old + 2)^1.8` where
  `engagement = vote_score + comment_count + view_count + bookmark_count`.
  The `+2` offset prevents division by zero for very new posts. The `1.8`
  exponent provides aggressive time decay. This is computed in both Go
  (pure function `ComputeVelocity`) and PostgreSQL (batch `UpdateVelocityScores`).
- **Explore feed is public** — guest users see trending content without
  authentication. This is intentional for SEO and user acquisition. The
  endpoint only requires a tenant (from the request context).
- **FanOutPost is a no-op placeholder** — the actual fan-out logic will be
  implemented as an Asynq task in Phase 6. The method signature and payload
  structs are defined now so the interface is stable.
- **Primary feed uses the feeds table** — personalized feeds read from the
  `feeds` table (push side). The `UNIQUE(user_id, content_id)` constraint
  prevents duplicate entries. The explore, space, and profile feeds read
  directly from `contents` + `content_scores` (pull side).
- **Sort modes route through a switch** — `GetFeed` dispatches to private
  methods based on timeline, then each method dispatches based on sort. The
  "latest" and "relevant" sort modes both use `velocity_score DESC` since
  relevance is not yet implemented (deferred to pgvector in Section 32).
- **4 sort modes defined but not all fully differentiated** — "latest" and
  "relevant" currently use the same query (velocity_score DESC). Full
  relevance scoring using pgvector `relevance_embedding` is deferred to a
  future section. "top" uses a 7-day rolling window with vote_score. "rising"
  uses velocity score.
- **All queries are tenant-scoped** — every query includes `tenant_id` in its
  WHERE clause. The tenant is resolved from the request context via tenant
  middleware.

## Known Issues and Deferrals

- **Fan-out push logic deferred to Phase 6** — `FanOutPost` is a no-op.
  When Phase 6 Asynq wiring is complete, it will: (1) query space member
  count, (2) if ≤ FanOutThreshold, fetch followers and insert feed rows,
  (3) if > FanOutThreshold, mark in hot cache for pull.
- **Fan-out pull logic not implemented** — large spaces (> 5000 members) will
  need a pull-side query that reads from `contents` + `content_scores`
  filtered by followed spaces. This is deferred to Phase 6.
- **Velocity score batch update deferred to Phase 6** — `UpdateVelocityScores`
  is implemented as a sqlc query but the Asynq cron task that calls it is
  not yet wired. The score will be stale until the task runs.
- **Relevance sort uses velocity fallback** — the "relevant" sort mode
  currently falls through to velocity_score DESC. Full relevance scoring
  using pgvector `relevance_embedding` (from Section 14) is deferred to a
  future section when the AI Gateway populates embeddings.
- **No notification on new feed items** — when content appears in a user's
  feed, no notification is sent. This will be wired when the Notification
  system (Section 35) is built.
- **No feed item read tracking** — there is no mechanism to mark feed items
  as read or track which items the user has seen. This is a UX enhancement
  for a future section.

## Next Steps

Section 33 — Professional Profiles. This section depends on:

- Section 05 spaces (for professional profile space associations)
- Section 14 content (for portfolio items)
- Section 31 follows (for professional profile follow counts)
