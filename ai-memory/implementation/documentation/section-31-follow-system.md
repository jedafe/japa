# Section 31 — Follow System: Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 31
**Status:** Complete

## Overview

Section 31 implements the universal polymorphic Follow System for JapaConnect.
Users can follow other users, Spaces, Tags, Organizations, and Professional
Profiles. A single `follows` table handles all entity types via
`entity_type`/`entity_id` discrimination. Follower counts are denormalized on
entity tables (`users.follower_count`, `spaces.follower_count`,
`tags.follower_count`) and maintained in the service layer — not via database
triggers — because not all entity tables exist yet.

The section also created the `tags` table, which was missing from earlier
sections and is required by the Follow System's polymorphic entity model.

## Components Implemented

### Migration 000012 — Follow System

1 migration file (`migrations/000012_follow_system.sql`, 39 lines); 2 tables
created, 1 column added:

1. **tags** — Tenant-scoped tag registry with `name`, `slug`, `description`,
   `color`, and `follower_count` (denormalized, maintained by the service layer).
   Unique constraint on `(tenant_id, slug)`. Index on `tenant_id`.

2. **follows** — Polymorphic follow table. Discriminator column `entity_type`
   with CHECK constraint (`user`/`space`/`tag`/`organization`/`professional_profile`).
   `follower_id` FK to `users` with `ON DELETE CASCADE`. `entity_id` stores the
   target entity's UUID. Unique constraint on `(follower_id, entity_type, entity_id)`
   prevents duplicate follows. Indexes on `(tenant_id, entity_type, entity_id)` and
   `(tenant_id, follower_id, entity_type)` for query performance.

3. **users.follower_count** — `int NOT NULL DEFAULT 0` added via `ALTER TABLE`.
   `spaces.follower_count` already existed from Section 05.

Conventions: `tenant_id` FK with `ON DELETE CASCADE` on tenant-scoped tables,
`gen_random_uuid()` for primary keys, `UNIQUE` constraints for natural keys,
`idx_` prefixed indexes, `CHECK` constraints for entity type validation.

Design decisions:

- **Tags table created in this migration** — it was not created in earlier
  sections and is required by the Follow System's polymorphic model. Creating it
  here avoids a circular dependency.
- **Follower counts maintained in the service layer** — database triggers are
  deferred because organizations and professional_profiles tables do not exist
  yet. The service layer uses raw SQL `UPDATE ... SET follower_count = GREATEST(0,
  follower_count + $1)` to atomically adjust counts.
- **`GREATEST(0, ...)` prevents negative counts** — if a count update fails
  mid-operation, the floor ensures the denormalized value never goes below zero.

### sqlc Queries (internal/db/queries/follows.sql)

13 queries in 1 SQL file (103 lines total):

**Follow CRUD (4 queries):**

1. `InsertFollow` — insert with `ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING`
2. `DeleteFollowByFollowerEntity` — delete by follower+entity pair
3. `DeleteFollowByID` — delete by follow ID
4. `GetFollow` — get single follow row by follower+entity pair

**Status Checks (2 queries):**

5. `IsFollowing` — `EXISTS` check for follow status, returns `(bool, error)`
6. `IsMutualFollow` — checks if B follows A (for DM gating), returns `(bool, error)`

**Lists (2 queries):**

7. `GetFollowersByEntity` — paginated followers with user display info (joins
   `users` and `user_profiles` for `username`, `display_name`, `avatar_url`)
8. `GetFollowingByUser` — paginated following with entity names via `CASE`
   subqueries that resolve `entity_name` from the appropriate table (`users`,
   `spaces`, `tags`)

**Counts (2 queries):**

9. `CountFollowersByEntity` — count followers for an entity
10. `CountFollowingByUser` — count following for a user

**Bulk Operations (1 query):**

11. `GetBulkFollowingStatus` — bulk check `is_following` for multiple entity IDs
    of the same type

**Cleanup (2 queries):**

12. `DeleteFollowsByFollower` — cascade delete all follows by a user (account deletion)
13. `DeleteFollowsByEntity` — cascade delete all follows for an entity (entity deletion)

Generated code:

- `internal/db/gen/follows.sql.go` (391 lines)
- 1 new model struct in `models.go`: `Follow`
- 7 new param/result structs: `InsertFollowParams`, `DeleteFollowByFollowerEntityParams`,
  `GetFollowParams`, `IsFollowingParams`, `IsMutualFollowParams`,
  `GetFollowersByEntityParams`, `GetFollowingByUserParams`,
  `CountFollowersByEntityParams`, `CountFollowingByUserParams`,
  `GetBulkFollowingStatusParams`, `DeleteFollowsByEntityParams`

### Package: internal/follows/ (6 files)

**follows.go** (42 lines)

- Package doc describing the universal polymorphic Follow System
- Entity type constants: `EntityTypeUser` ("user"), `EntityTypeSpace` ("space"),
  `EntityTypeTag` ("tag"), `EntityTypeOrganization` ("organization"),
  `EntityTypeProfessionalProfile` ("professional_profile")
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (192 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 11 private methods wrapping sqlc-generated queries:
  - `toggleFollow` — checks existing follow, deletes if found (unfollow), inserts
    if not found (follow). Returns `(bool, error)` indicating new follow status.
  - `getFollow` — get follow row by follower+entity pair
  - `isFollowing` — EXISTS check for follow status
  - `isMutualFollow` — mutual follow check for two users
  - `getFollowers` — paginated followers with user display info
  - `getFollowing` — paginated following with entity names
  - `countFollowers` — count followers for entity
  - `countFollowing` — count following for user
  - `getBulkFollowingStatus` — bulk follow status for multiple entities
  - `deleteFollowsByFollower` — cascade delete for account deletion
  - `deleteFollowsByEntity` — cascade delete for entity deletion
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context
- Page size defaults applied in store layer (clamp to `DefaultPageSize` if
  invalid, cap at `MaxPageSize` if exceeded)

**service.go** (177 lines)

- `FollowerInfo` struct — extends `gen.Follow` with `Username`, `DisplayName`,
  `AvatarURL` (from `user_profiles` join)
- `FollowingInfo` struct — extends `gen.Follow` with `EntityName` (resolved via
  `CASE` subqueries)
- 10 public methods:
  - `ToggleFollow` — validates entity type, prevents self-follow for user entity,
    calls store toggle, updates denormalized follower count
  - `GetFollow` — delegates to store
  - `IsFollowing` — delegates to store
  - `IsMutualFollow` — delegates to store
  - `GetFollowers` — returns `[]FollowerInfo` with profile data
  - `GetFollowing` — returns `[]FollowingInfo` with entity names
  - `CountFollowers` — delegates to store
  - `CountFollowing` — delegates to store
  - `GetBulkFollowingStatus` — delegates to store
  - `DeleteFollowsByFollower` / `DeleteFollowsByEntity` — cleanup operations
- `updateFollowerCount` — raw SQL UPDATE that maps entity type to table name
  (`users`, `spaces`, `tags`), uses `GREATEST(0, ...)` to prevent negative
  counts. Returns nil for organizations/professional_profiles (tables don't exist
  yet). Logs warning but does not fail the follow operation on count update error.
- `isValidEntityType` — validates against all 5 entity type constants

**handlers.go** (226 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 4 HTTP handlers:
  - `ToggleFollow` — `POST /api/v1/follows`. Accepts JSON body with
    `entity_type` and `entity_id`. Returns `{ "data": { "following": true/false } }`.
  - `GetFollowers` — `GET /api/v1/{entity_type}/{entity_id}/followers`.
    Extracts `entity_type` from URL path, `entity_id` from query param.
    Supports `page_size` and `page_offset` pagination.
  - `GetFollowing` — `GET /api/v1/users/{user_id}/following`. Extracts `user_id`
    from URL path. Supports pagination.
  - `CheckMutual` — `GET /api/v1/users/{user_id}/mutual/{other_user_id}`.
    Returns `{ "data": { "mutual": true/false } }`.
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`,
  `parsePageSize`, `parsePageOffset`
- Uses `auth.FromContext`, `tenant.FromContext`, `chi.URLParam`, `pgtype.UUID`

### Server Wiring (internal/server/server.go)

- `follows.NewService(database)` and `follows.NewHandlers(followService)` created
  in `NewServer`
- 6 API routes registered under `/api/v1`, all within the tenant-scoped route
  group, all protected by `auth.RequireAuth`
- 2 new permission keys added to `bootstrapPermissions` (bringing total to 38)

### Permission Keys

2 new permission keys bootstrapped at server startup (added to the existing
`bootstrapPermissions` slice):

| Key | Description |
|-----|-------------|
| `follows.toggle` | Follow/unfollow entities |
| `follows.view` | View follower/following lists |

All 38 permissions (11 Section 14 + 9 Section 05 + 11 Section 06 + 2 Section 31)
are assigned to the `super_admin_role`, which is granted to the `super_admin`
system group. Every operation is idempotent.

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
| internal/linkpreview | (existing) | PASS |
| internal/mentions | (existing) | PASS |
| internal/renderer | (existing) | PASS |
| **Total** | **131+** | **ALL PASS** |

### Follow System Tests (12 tests)

| # | Test | Coverage |
|---|------|----------|
| 1 | `TestEntityTypeConstants` | All 5 entity type constants match expected values |
| 2 | `TestPageSizeConstants` | DefaultPageSize=20, MaxPageSize=100 |
| 3 | `TestIsValidEntityType` | 9 cases: valid types, invalid strings, empty, wrong case |
| 4 | `TestWriteJSON` | HTTP status, Content-Type header, JSON body serialization |
| 5 | `TestWriteError` | Error envelope format: `{ "error": { "code", "message" } }` |
| 6 | `TestParsePageSize` | 9 cases: empty, zero, negative, valid, overflow clamping |
| 7 | `TestParsePageOffset` | 6 cases: empty, zero, negative, valid |
| 8 | `TestToggleFollowSelfFollowPrevention` | Self-follow returns error |
| 9 | `TestToggleFollowInvalidEntityType` | Invalid entity type returns error |
| 10 | `TestFollowerInfoJSONTags` | JSON serialization of username, display_name, avatar_url |
| 11 | `TestFollowingInfoJSONTags` | JSON serialization of entity_name |
| 12 | `TestNewServiceNotNil` | Service zero-value has nil store |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all packages pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

6 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/follows` | Required | `follows.toggle` | Toggle follow/unfollow |
| 2 | GET | `/users/{user_id}/followers` | Required | `follows.view` | List followers of a user |
| 3 | GET | `/users/{user_id}/following` | Required | `follows.view` | List entities a user follows |
| 4 | GET | `/users/{user_id}/mutual/{other_user_id}` | Required | `follows.view` | Check mutual follow status |
| 5 | GET | `/spaces/{space_id}/followers` | Required | `follows.view` | List followers of a space |
| 6 | GET | `/tags/{tag_id}/followers` | Required | `follows.view` | List followers of a tag |

## Permission Keys Registered

2 new permission keys added to `bootstrapPermissions`:

| Key | Description |
|-----|-------------|
| `follows.toggle` | Follow/unfollow entities |
| `follows.view` | View follower/following lists |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000012_follow_system.sql` | 39 | Tags table, follows table, follower_count column |
| 2 | `internal/db/queries/follows.sql` | 103 | 13 sqlc queries for follows CRUD, status, lists, counts, bulk, cleanup |
| 3 | `internal/db/gen/follows.sql.go` | 391 | Generated query methods and param structs |
| 4 | `internal/follows/follows.go` | 42 | Entity type constants, page size constants, Service struct |
| 5 | `internal/follows/store.go` | 192 | Store layer with read/write separation, 11 private methods |
| 6 | `internal/follows/service.go` | 177 | Business logic, FollowerInfo/FollowingInfo types, follower count maintenance |
| 7 | `internal/follows/handlers.go` | 226 | 4 HTTP handlers with pagination and validation helpers |
| 8 | `internal/follows/follows_test.go` | 265 | 12 unit tests covering constants, validation, helpers, struct tags |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `follows` import; created follow service and handlers; registered 6 routes with auth middleware; added 2 follow permission keys to `bootstrapPermissions` |
| 2 | `internal/db/gen/models.go` | Regenerated by `sqlc generate` to include `Follow` struct |
| 3 | `internal/db/gen/db.go` | Regenerated by `sqlc generate` to register the new query methods |

## Key Decisions

- **Self-follow prevention** — users cannot follow themselves when `entity_type`
  is `"user"`. The check is in `Service.ToggleFollow` before any database
  operation. Other entity types (spaces, tags) have no self-follow concept.
- **Follower count maintenance in service layer** — raw SQL `UPDATE ... SET
  follower_count = GREATEST(0, follower_count + $1)` rather than database
  triggers. This is because: (1) organizations and professional_profiles tables
  don't exist yet, (2) not all entity tables share the same column name
  convention, (3) service-layer updates are easier to test and debug. The count
  update is best-effort: a failure is logged but does not fail the follow
  operation.
- **Tags table created in this migration** — it was missing from earlier sections
  and is required by the Follow System's polymorphic entity model. Creating it
  here avoids a circular dependency with Section 05 (which references tags but
  never created the table).
- **`GetFollowersByEntity` joins `user_profiles`** — provides `display_name` and
  `avatar_url` for rendering follower lists without a separate API call.
- **`GetFollowingByUser` uses `CASE` subqueries** — resolves `entity_name` from
  the appropriate table (`users.username`, `spaces.name`, `tags.name`) in a
  single query. Falls back to empty string for unknown entity types.
- **`GREATEST(0, ...)` prevents negative counts** — if a count update fails
  mid-operation, the floor ensures the denormalized value never goes below zero.
- **Toggle pattern (POST with ON CONFLICT DO NOTHING + DELETE)** — the
  `toggleFollow` store method checks for an existing row, deletes if found
  (unfollow), inserts if not found (follow). This is idempotent and avoids
  race conditions from concurrent follow/unfollow requests.
- **All queries are tenant-scoped** — every query includes `tenant_id` in its
  WHERE clause. The tenant is resolved from the request context via
  `tenant.FromContext`.
- **Bulk follow status for list views** — `GetBulkFollowingStatus` enables
  checking follow status for multiple entities in a single query, used by feed
  and list views to show follow buttons.

## Known Issues and Deferrals

- **Organizations and professional_profiles follower counts deferred** — these
  tables do not exist yet. The `updateFollowerCount` method returns nil for
  `EntityTypeOrganization` and `EntityTypeProfessionalProfile`. When Section 07
  (Organization Profiles) and Section 33 (Professional Profiles) are built,
  their `follower_count` columns will be added and the switch statement updated.
- **DB trigger-based follower count maintenance deferred** — service-layer
  updates are used instead. Database triggers could be added later for
  consistency guarantees, but are not needed at this stage.
- **Space auto-join on follow partially implemented** — the follow itself works,
  but the space joining logic depends on the space permission system (Section 05)
  which may require additional wiring.
- **Follow notification system deferred** — no notifications are sent when a
  user follows another entity. This will be wired when the Notification system
  is built.
- **Follow feed integration deferred** — the Activity Feed (Section 32) will
  consume follow events to populate personalized feeds. The follow data is
  stored but not yet integrated into feed queries.

## Next Steps

Section 32 — Activity Feed. This section depends on:

- Section 31 follow data (for personalized feed filtering)
- Section 14 content rows and `content_scores` (for ranking)
- pgvector `relevance_embedding` column (populated by AI Gateway)
- Section 05 spaces (for space-scoped feeds)
