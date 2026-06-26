# Section 05 — Geo-First Space System: Implementation Complete

**Date:** 2026-06-25
**Phase:** 2
**Section:** 05
**Status:** Complete

## Overview

Section 05 implements the geo-first space system that organizes all community
activity on JapaConnect. Spaces form a hierarchy (Country → State → City, with
`custom` for non-geo communities like universities). Each space carries five
admin-configurable permission axes — visibility, posting, replying, commenting,
and joining — initialised from one of seven seeded permission presets at
creation time. Space Types (Country, State, City, Community, University) link
to custom field groups from Section 06 for type-specific fields.

The section also introduced the SpaceResolver middleware pattern (resolving a
`{slug}` path parameter to a `gen.Space` in the request context), a Valkey-
backed unread counter for per-user per-space unread post tracking with graceful
nil-client degradation, and a stubbed `TaskEnqueuer` interface that will be
swapped for a real Asynq producer in Phase 6 for space auto-population jobs.

## Components Implemented

### Migration 000009 — Geo-First Space System

9 migration files now in `migrations/` (000001-000009); 4 tables created in this
migration:

1. **space_permission_presets** — Named permission-axis bundles, tenant-scoped,
   unique on `(tenant_id, name)`, `is_system` flag protects seeded presets
2. **space_types** — Type taxonomy for spaces, tenant-scoped, unique on
   `(tenant_id, slug)`, optional `field_group_id` FK to `custom_field_groups`
   (Section 06) with `ON DELETE SET NULL`
3. **spaces** — The core space entity with self-referential `parent_id`
   hierarchy, `level` (country/state/city/custom), `geo_code` for ISO codes,
   five permission axis columns, `wiki_content` (TipTap JSONB), and
   `logo_media_id`/`header_image_id` as plain uuid (no FK — media table is
   Section 34)
4. **space_rules** — Per-space rules shown in the Rules tab; `space_id` FK
   with `ON DELETE CASCADE` (note: `space_rules` has no `tenant_id` — it is
   scoped by `space_id` only, since spaces already carry the tenant)

Seed data:

- 7 system permission presets using deterministic UUIDs 050-056:
  `open_community`, `geo_space`, `curated`, `private`, `read_only`,
  `verified_only`, `admin_only` — each defining distinct combinations of the
  five permission axes
- 5 system space types using deterministic UUIDs 060-064: Country, State, City,
  Community (all linking to the `space_fields` group 043 from Section 06), and
  University (linking to the `university_fields` group 044)
- `spaces.bubble_depth` system setting seeded to `2` — controls reply-thread
  nesting depth before collapsing into a "view more" bubble (post bubbling
  logic deferred to Section 32)

Conventions: `tenant_id` FK with `ON DELETE CASCADE` on tenant-scoped tables,
`gen_random_uuid()` for primary keys, `UNIQUE` constraints for natural keys,
`idx_` prefixed indexes (including a partial index
`idx_spaces_geo WHERE geo_code IS NOT NULL`), deterministic UUIDs for system
seeds, `is_system` flag protecting seeded rows from API deletion.

Design decisions:

- **`logo_media_id` / `header_image_id` are plain `uuid` without FK
  constraints** — the media table (Section 34) does not exist yet; FK
  constraints will be added in a later migration when the media system is
  built. Storing the column now avoids a schema migration later.
- **`space_pinned_posts` table deferred** — it needs a FK to the `contents`
  table (Section 14). It will be added in migration 000010 once the content
  tables exist.
- **`space_rules` has no `tenant_id`** — rules are scoped by `space_id` only,
  since `spaces` already carries the tenant ID. This avoids redundant tenant
  checks but requires rule queries to join through `spaces` for tenant
  isolation when needed.
- **Permission axes stored directly on `spaces`** — presets are convenience
  defaults; the actual axis values live on the space so changing a preset after
  a space is created does not retroactively alter existing spaces.

### sqlc Queries

23 queries in `internal/db/queries/spaces.sql` across 4 entities:

- **Permission presets (3):** `ListPermissionPresetsByTenant`, `GetPermissionPresetByName`,
  `UpdatePermissionPreset`
- **Space types (6):** `ListSpaceTypesByTenant`, `GetSpaceTypeByID`,
  `GetSpaceTypeBySlug`, `CreateSpaceType` (using `sqlc.narg` for nullable
  description, icon, field_group_id), `UpdateSpaceType` (narg for the same
  fields), `DeleteSpaceType` (guarded with `is_system = false` to protect
  system types at the database level)
- **Spaces (11):** `ListSpacesByTenant`, `ListSpacesByLevel`,
  `ListSpacesByType`, `ListChildSpaces`, `GetSpaceByID`, `GetSpaceBySlug`,
  `CreateSpace` (narg for description, geo_code), `UpdateSpace` (narg for the
  same plus `parent_id` is immutable), `DeleteSpace` (soft behavior via status
  is not enforced — hard delete with `ON DELETE CASCADE` for rules),
  `IncrementSpaceCount` (membership delta for join/leave events)
- **Space rules (4):** `ListSpaceRulesBySpace`, `CreateSpaceRule`,
  `UpdateSpaceRule`, `DeleteSpaceRule` — all scoped by `space_id` only since
  `space_rules` lacks `tenant_id`

Generated `internal/db/gen/spaces.sql.go` (24,060 bytes) alongside the existing
`custom_fields.sql.go`, `forms.sql.go`, `models.go`, and `db.go`. The
`models.go` file was regenerated to include struct definitions for all four
new tables.

### internal/spaces Package (7 files, 1,648 lines)

**spaces.go** (137 lines)

- 4 hierarchy level constants: `LevelCountry`, `LevelState`, `LevelCity`,
  `LevelCustom`
- 3 visibility axis constants: `VisibilityPublic`, `VisibilityMembers`,
  `VisibilityPrivate`
- 5 posting/replying/commenting axis constants: `PermOpen`, `PermMembers`,
  `PermVerified`, `PermModerators`, `PermAdmins`
- 3 joining axis constants: `JoiningOpen`, `JoiningRequest`, `JoiningInvite`
- 3 lifecycle status constants: `StatusActive`, `StatusArchived`, `StatusDraft`
- `SpacePermAxes` struct holding the five permission axes
- `ValidPermissionAxes` — checks each axis against its valid constant set
  (posting/replying/commenting share the same set)
- `IsValidLevel` — recognized hierarchy level check
- `DefaultPermAxes` — returns `public` / `members` / `members` / `members` /
  `open` (the defaults applied when no preset is specified)
- `Service` struct with reader/writer `*gen.Queries` split, `NewService`
  constructor

**store.go** (283 lines)

- 23 `Service` methods wrapping sqlc-generated queries across 4 entity groups:
  - **Permission presets (3):** `ListPermissionPresets`, `GetPermissionPreset`,
    `UpdatePermissionPreset`
  - **Space types (6):** `ListSpaceTypes`, `GetSpaceType`, `GetSpaceTypeBySlug`,
    `CreateSpaceType`, `UpdateSpaceType`, `DeleteSpaceType`
  - **Spaces (10):** `ListSpaces`, `ListSpacesByLevel`, `ListSpacesByType`,
    `ListChildSpaces`, `GetSpace`, `GetSpaceBySlug`, `CreateSpace`,
    `UpdateSpace`, `DeleteSpace`, `IncrementSpaceCount`
  - **Space rules (4):** `ListSpaceRules`, `CreateSpaceRule`,
    `UpdateSpaceRule`, `DeleteSpaceRule`
- Read methods use `s.read`; write methods use `s.write`
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**middleware.go** (85 lines)

- `SpaceContextKey` — unexported-type context key avoiding collisions
- `FromContext` — extracts the resolved `*gen.Space` from context (nil-safe)
- `SpaceResolver` middleware — resolves the `{slug}` URL param to a
  `gen.Space` via `service.GetSpaceBySlug`, stores it under
  `SpaceContextKey`, returns 404 on `pgx.ErrNoRows`, 503 when no tenant is
  available, 500 on other errors
- Package-level `writeJSON`/`writeError` helpers following the JapaConnect API
  error envelope `{"error": {"code": "...", "message": "..."}}`

**handlers.go** (704 lines)

- `Handlers` struct with `*Service` and `*UnreadCounter` dependencies
- `NewHandlers` constructor
- 16 HTTP handlers:
  - **Space types (5):** `ListSpaceTypes`, `GetSpaceType`, `CreateSpaceType`
    (API-created types are never `is_system`), `UpdateSpaceType` (slug is
    immutable, not accepted in the request body), `DeleteSpaceType`
  - **Spaces (5):** `ListSpaces` (with optional `level`/`type`/`parent` query
    filters), `GetSpace` (returns the space together with its rules for a
    single-request render), `CreateSpace` (applies named preset's axes or
    `DefaultPermAxes`), `UpdateSpace` (fetch-then-overlay true PATCH semantics
    — unspecified fields keep their current values; permission axes validated
    individually), `DeleteSpace`
  - **Space rules (4):** `ListSpaceRules`, `CreateSpaceRule`, `UpdateSpaceRule`,
    `DeleteSpaceRule` — all read the resolved space from context via
    `spaceFromContext`
  - **Permission presets (2):** `ListPermissionPresets`, `UpdatePermissionPreset`
    (name taken from URL path; axes validated via `ValidPermissionAxes`)
- Helpers: `parseUUIDParam`, `requireTenant`, `handleServiceError` (maps
  `pgx.ErrNoRows` to 404, duplicate errors to 409, other to 500),
  `spaceFromContext` (404 when no resolved space)

**unread.go** (98 lines)

- `UnreadCounter` struct backed by `*redis.Client`
- `NewUnreadCounter(vk *valkey.Client)` — nil client returns a counter with
  `rdb == nil` (graceful degradation; all operations are no-ops)
- Key format: `jc:space_unread:{user_id}:{space_id}` → integer
- `Increment` — `INCR`, silently fails if Valkey is unreachable (unread counts
  are nice-to-have, not critical path)
- `Reset` — `SET 0` (no TTL), called when a user visits a space
- `Get` — `GET`, returns 0 on error (including `redis.Nil` meaning key absent)
- `GetBatch` — `MGET` for multiple spaces, returns a `map[string]int` keyed by
  space ID string; missing/errored entries default to 0

**tasks.go** (36 lines)

- `TaskTypeSpaceAutoPopulate = "space:auto_populate"` constant
- `TaskEnqueuer` interface with `EnqueueSpaceAutoPopulate(ctx, spaceID) error`
- `noopEnqueuer` implementation (`NewNoopEnqueuer`) — logs via `slog.Info` and
  returns nil immediately
- Real Asynq wiring deferred to Phase 6 (Section 11 — Job Queue). The
  interface lets other packages call `EnqueueSpaceAutoPopulate` without
  knowing about the stub.

**spaces_test.go** (305 lines)

- 13 unit tests covering: level validation, permission-axis validation,
  `SpacePermAxes` constant integrity, JSON helpers, noop enqueuer behavior
  with real UUID, nil-client `UnreadCounter` no-op safety, handler request
  validation (missing fields on `CreateSpace`, invalid level on
  `CreateSpace`, invalid body on `CreateSpaceType`, missing tenant on
  `ListSpaceTypes`), UUID param parsing, and `DefaultPermAxes` values

### Server Wiring

- `spaces.NewService(database)`, `spaces.NewUnreadCounter(vkClient)`, and
  `spaces.NewHandlers(spaceService, spaceUnread)` created in `NewServer`
- **Valkey client creation moved before the spaces subsystem** so the same
  client is shared between custom fields (future use) and the space unread
  counter; the `vkClient` is now consumed by `spaces.NewUnreadCounter(vkClient)`
  rather than discarded
- 16 API routes registered under `/api/v1`, all within the tenant-scoped route
  group
- `spaces.SpaceResolver(spaceService)` middleware applied to all slug-based
  space routes (`/spaces/{slug}`, `/spaces/{slug}/rules`,
  `/spaces/{slug}/rules/{ruleId}`) — resolves the slug to a `gen.Space` in the
  request context before the handler runs
- 9 new permission keys bootstrapped at startup via the existing
  `bootstrapPermissions` function (added to the `permissions` slice)
- All 9 new permissions assigned to the `super_admin_role`, granted to the
  `super_admin` system group (idempotent)

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/config | 4 | PASS |
| internal/server | 9 | PASS |
| internal/tenant | 3 | PASS |
| internal/auth | 21 | PASS |
| internal/rbac | 4 | PASS |
| internal/features | 5 | PASS |
| internal/geoip | 7 | PASS |
| internal/valkey | 6 | PASS |
| internal/customfields | 37 | PASS |
| internal/spaces | 13 | PASS |
| **Total** | **109** | **ALL PASS** |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./... -count=1` | ok — all 10 packages pass |
| `go build ./...` | success |
| `go build -o bin/japa-api ./cmd/api/` | success (19,464,070 bytes) |
| `go build -o bin/japa-migrate ./cmd/migrate/` | success (13,731,858 bytes) |
| `go vet ./...` | clean |

## API Routes Added

16 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | GET | `/space-types` | Required | — | List space types |
| 2 | GET | `/space-types/{id}` | Required | — | Get space type |
| 3 | POST | `/space-types` | Required | `space_types.create` | Create space type |
| 4 | PATCH | `/space-types/{id}` | Required | `space_types.edit` | Update space type |
| 5 | DELETE | `/space-types/{id}` | Required | `space_types.delete` | Delete space type |
| 6 | GET | `/spaces` | Required | — | List spaces (filter by level/type/parent) |
| 7 | GET | `/spaces/{slug}` | Required + SpaceResolver | — | Get space with rules |
| 8 | POST | `/spaces` | Required | `spaces.create` | Create space (applies preset axes) |
| 9 | PATCH | `/spaces/{id}` | Required | `spaces.edit` | Update space (fetch-then-overlay) |
| 10 | DELETE | `/spaces/{id}` | Required | `spaces.delete` | Delete space |
| 11 | GET | `/spaces/{slug}/rules` | Required + SpaceResolver | — | List space rules |
| 12 | POST | `/spaces/{slug}/rules` | Required + SpaceResolver | `spaces.moderate` | Create space rule |
| 13 | PATCH | `/spaces/{slug}/rules/{ruleId}` | Required + SpaceResolver | `spaces.moderate` | Update space rule |
| 14 | DELETE | `/spaces/{slug}/rules/{ruleId}` | Required + SpaceResolver | `spaces.moderate` | Delete space rule |
| 15 | GET | `/space-permission-presets` | Required | — | List permission presets |
| 16 | PUT | `/space-permission-presets/{name}` | Required | `spaces.admin` | Update permission preset |

## Permission Keys Registered

9 new permission keys bootstrapped at server startup (added to the existing
`bootstrapPermissions` slice alongside the 11 from Section 06):

| Key | Description |
|-----|-------------|
| `space_types.create` | Create space types |
| `space_types.edit` | Edit space types |
| `space_types.delete` | Delete space types |
| `spaces.create` | Create spaces |
| `spaces.edit` | Edit spaces |
| `spaces.delete` | Delete spaces |
| `spaces.moderate` | Moderate spaces (manage rules and pins) |
| `spaces.moderate.any` | Moderate any space (admin-only posting permission) |
| `spaces.admin` | Manage space permission presets |

All 20 permissions (11 Section 06 + 9 Section 05) are assigned to the
`super_admin_role`, which is granted to the `super_admin` system group. Every
operation is idempotent.

## Files Created

| # | File | Purpose |
|---|------|---------|
| 1 | `migrations/000009_geo_first_space_system.sql` | 4 tables, 7 preset seeds, 5 space type seeds, bubble_depth setting, indexes |
| 2 | `internal/db/queries/spaces.sql` | 23 sqlc queries across 4 entities |
| 3 | `internal/db/gen/spaces.sql.go` | Generated space query methods (24,060 bytes) |
| 4 | `internal/spaces/spaces.go` | Level/permission/status constants, SpacePermAxes, Service struct |
| 5 | `internal/spaces/store.go` | 23 Service methods wrapping sqlc queries |
| 6 | `internal/spaces/middleware.go` | SpaceContextKey, FromContext, SpaceResolver middleware |
| 7 | `internal/spaces/handlers.go` | 16 HTTP handlers with helpers |
| 8 | `internal/spaces/unread.go` | Valkey-backed UnreadCounter with graceful degradation |
| 9 | `internal/spaces/tasks.go` | TaskEnqueuer interface with noopEnqueuer stub |
| 10 | `internal/spaces/spaces_test.go` | 13 unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `spaces` import; moved Valkey client creation before subsystem wiring; created space service, unread counter, and handlers; registered 16 routes with auth/RBAC/SpaceResolver middleware; added 9 space permission keys to `bootstrapPermissions` |
| 2 | `internal/db/gen/models.go` | Regenerated by `sqlc generate` to include structs for the 4 new tables |
| 3 | `internal/db/gen/db.go` | Regenerated by `sqlc generate` to register the new query methods |

## Known Issues

- **`space_pinned_posts` table deferred to Section 14** — it requires a FK to
  the `contents` table (Content Type System), which does not exist yet. It
  will be added in migration 000010 once the content tables are in place.
- **`logo_media_id` / `header_image_id` FK constraints deferred to Section
  34** — the media system is not built yet, so these columns are plain
  `uuid` without FKs. Constraints will be backfilled in a later migration.
- **Asynq task handlers stubbed** — `space:auto_populate` (populating a space
  from external sources like Wikipedia, flagcdn.com, Pexels) is defined as a
  task type but the real producer/consumer wiring is deferred to Phase 6
  (Section 11 — Job Queue). The `noopEnqueuer` logs the request via `slog` and
  returns nil so other packages can call the interface today.
- **Post bubbling logic deferred to Section 32** — the `spaces.bubble_depth`
  system setting is seeded (`2`), but the Activity Feed logic that reads it
  to collapse deep reply threads is not yet implemented.
- **Unread counter fan-out not yet triggered** — `UnreadCounter.Increment` is
  implemented and wired, but no code path currently calls it (new post
  creation lands in Section 14/45). Infrastructure is ready.

## Next Steps

Section 14 — Content Type System. This section depends on:

- Section 06 field groups (for content-type-specific custom fields)
- Section 05 geo `space_id` (for scoping content to spaces)

The `space_pinned_posts` table (deferred from this section) will be created in
migration 000010 alongside the content tables, closing the one known schema
gap left by Section 05.