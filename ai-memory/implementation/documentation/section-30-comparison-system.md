# Section 30 — Generic Comparison System: Implementation Complete

**Date:** 2026-06-26
**Phase:** 4
**Section:** 30
**Status:** Complete

## Overview

Section 30 implements a generic, type-driven comparison system for JapaConnect.
Any entity type (space, professional profile, service provider) can be compared
side-by-side along configurable dimensions. The system defines comparison type
templates (e.g., "Visa Consultant", "Relocation Service", "Co-Working Space"),
each with a set of dimensions (text, number, boolean, select, rating) that
determine what attributes can be compared.

Comparison items are entity references (`entity_type`/`entity_id`) linked to a
comparison type. Values are stored in a separate `comparison_values` table keyed
by `(item_id, dimension_id)`, enabling sparse storage — only dimensions with
actual values are stored.

## Components Implemented

### Migration 000020 — Comparison System

1 migration file (`migrations/000020_comparison_system.sql`, 84 lines); 5 tables
created:

**comparison_type_definitions** — Templates for comparison types. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `name` — Type name (text, not null)
- `slug` — URL-safe slug (text, not null)
- `description` — Type description (text, nullable)
- `icon` — Display icon (text, nullable)
- `is_system` — System-defined type flag (boolean, default `false`)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, slug)`

**comparison_type_access** — Access control per comparison type. Columns:
- `type_id` — FK to `comparison_type_definitions` with `ON DELETE CASCADE`
- `allowed_role` — Role key allowed to use this type (text, not null)
- `created_at` — Creation timestamp (default `now()`)
- Composite primary key on `(type_id, allowed_role)`

**comparison_items** — Entity references in a comparison. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `type_id` — FK to `comparison_type_definitions` with `ON DELETE CASCADE`
- `entity_type` — Discriminator: `'space'`, `'professional'`, etc. (text, not null)
- `entity_id` — UUID of the compared entity (not null)
- `name` — Display name (text, not null)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(tenant_id, type_id, entity_type, entity_id)`

**comparison_dimensions** — Attributes that can be compared. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `type_id` — FK to `comparison_type_definitions` with `ON DELETE CASCADE`
- `name` — Dimension name (text, not null)
- `key` — Machine-readable key (text, not null)
- `dimension_type` — Data type: `'text'`, `'number'`, `'boolean'`, `'select'`, `'rating'` (text, not null)
- `unit` — Unit of measurement for numbers (text, nullable)
- `options` — JSON array of select options (jsonb, nullable)
- `position` — Display ordering (integer)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(type_id, key)`

**comparison_values** — Stored values per dimension per item. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `item_id` — FK to `comparison_items` with `ON DELETE CASCADE`
- `dimension_id` — FK to `comparison_dimensions` with `ON DELETE CASCADE`
- `text_value` — Value for text/boolean types (text, nullable)
- `number_value` — Value for number/rating types (numeric, nullable)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(item_id, dimension_id)`

Indexes:
- `idx_comparison_items_type` — `(tenant_id, type_id, created_at DESC)`
- `idx_comparison_values_item` — `(item_id, dimension_id)`

### sqlc Queries (internal/db/queries/comparison.sql)

20 queries in 1 SQL file (184 lines total):

**Type Definitions (6 queries):**
1. `InsertComparisonType` — create comparison type
2. `GetComparisonTypeByID` — get type by ID
3. `GetComparisonTypeBySlug` — get type by slug
4. `ListComparisonTypesByTenant` — paginated types for a tenant
5. `UpdateComparisonType` — update type metadata
6. `DeleteComparisonType` — delete type

**Type Access (3 queries):**
7. `SetTypeAccess` — replace allowed roles for a type
8. `GetTypeAccess` — list allowed roles for a type
9. `CheckTypeAccess` — check if a role has access

**Items (4 queries):**
10. `InsertComparisonItem` — add entity to comparison
11. `ListComparisonItems` — list items for a type
12. `DeleteComparisonItem` — remove item from comparison
13. `GetComparisonItemByID` — get item by ID

**Dimensions (3 queries):**
14. `InsertComparisonDimension` — add dimension to type
15. `ListComparisonDimensions` — list dimensions for a type
16. `DeleteComparisonDimension` — remove dimension

**Values (4 queries):**
17. `UpsertComparisonValue` — insert or update value (ON CONFLICT)
18. `ListComparisonValues` — list values for an item
19. `GetComparisonGrid` — get all values for a type (items x dimensions matrix)
20. `DeleteComparisonValuesByItem` — delete all values for an item

### Package: internal/comparison/ (5 files)

**comparison.go** (77 lines)

- Dimension type constants: `DimTypeText`, `DimTypeNumber`, `DimTypeBoolean`, `DimTypeSelect`, `DimTypeRating`
- Default type names: `DefaultTypeVisaConsultant`, `DefaultTypeRelocationService`, `DefaultTypeCoWorkingSpace`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (279 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 18 public methods wrapping sqlc-generated queries across types, access, items, dimensions, and values
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**service.go** (404 lines)

- Business logic for comparison types, items, dimensions, values, and grid assembly
- `CreateComparisonType` — validates name, generates slug, creates type + default dimensions
- `AddComparisonItem` — validates entity_type, checks for duplicates, inserts item
- `SetComparisonValue` — validates dimension_type, upserts value
- `GetComparisonGrid` — assembles the full items x dimensions matrix for rendering
- Input validation at service layer with descriptive error messages

**handlers.go** (806 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 19 HTTP handlers covering types, access, items, dimensions, values, and grid
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`, `parsePageSize`

**comparison_test.go** (11 unit tests)

- Constants, dimension types, JSON serialization, pagination helpers, grid assembly logic

### Server Wiring (internal/server/server.go)

- `comparison.NewService(database)` and `comparison.NewHandlers(comparisonService)` created in `NewServer`
- 18 API routes registered under `/api/v1`, all within the tenant-scoped route group
- 12 new permission keys added to `bootstrapPermissions`

### Permission Keys

12 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `comparison.type.create` | Create comparison types |
| `comparison.type.edit` | Edit comparison types |
| `comparison.type.delete` | Delete comparison types |
| `comparison.type.access.manage` | Manage type access control |
| `comparison.item.add` | Add items to comparisons |
| `comparison.item.edit` | Edit comparison items |
| `comparison.item.delete` | Delete comparison items |
| `comparison.dimension.manage` | Manage comparison dimensions |
| `comparison.value.set` | Set comparison values |
| `comparison.value.view` | View comparison values |
| `comparison.grid.view` | View comparison grid |
| `comparison.admin` | Full comparison administration |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/comparison | 11 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/comparison/ -count=1` | ok — 11 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

18 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/comparison/types` | Required | `comparison.type.create` | Create comparison type |
| 2 | GET | `/comparison/types` | Required | — | List comparison types |
| 3 | GET | `/comparison/types/{id}` | Required | — | Get type by ID |
| 4 | PATCH | `/comparison/types/{id}` | Required | `comparison.type.edit` | Update type |
| 5 | DELETE | `/comparison/types/{id}` | Required | `comparison.type.delete` | Delete type |
| 6 | PUT | `/comparison/types/{id}/access` | Required | `comparison.type.access.manage` | Set type access roles |
| 7 | GET | `/comparison/types/{id}/access` | Required | — | Get type access roles |
| 8 | POST | `/comparison/types/{id}/items` | Required | `comparison.item.add` | Add item to comparison |
| 9 | GET | `/comparison/types/{id}/items` | Required | — | List items for a type |
| 10 | DELETE | `/comparison/items/{id}` | Required | `comparison.item.delete` | Remove item |
| 11 | POST | `/comparison/types/{id}/dimensions` | Required | `comparison.dimension.manage` | Add dimension |
| 12 | GET | `/comparison/types/{id}/dimensions` | Required | — | List dimensions |
| 13 | DELETE | `/comparison/dimensions/{id}` | Required | `comparison.dimension.manage` | Remove dimension |
| 14 | PUT | `/comparison/items/{id}/values` | Required | `comparison.value.set` | Set value for dimension |
| 15 | GET | `/comparison/items/{id}/values` | Required | `comparison.value.view` | List values for item |
| 16 | GET | `/comparison/types/{id}/grid` | Required | `comparison.grid.view` | Get full comparison grid |
| 17 | DELETE | `/comparison/items/{id}` | Required | `comparison.item.delete` | Delete item and values |
| 18 | GET | `/comparison/types/{id}/access/check` | Required | — | Check if current user has access |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000020_comparison_system.sql` | 84 | 5 tables for comparison types, items, dimensions, values |
| 2 | `internal/db/queries/comparison.sql` | 184 | 20 sqlc queries for all comparison entities |
| 3 | `internal/db/gen/comparison.sql.go` | (generated) | Generated query methods and param structs (862 lines) |
| 4 | `internal/comparison/comparison.go` | 77 | Constants, dimension types, Service struct |
| 5 | `internal/comparison/store.go` | 279 | Store layer with read/write separation |
| 6 | `internal/comparison/service.go` | 404 | Business logic for types, items, dimensions, grid |
| 7 | `internal/comparison/handlers.go` | 806 | 19 HTTP handlers |
| 8 | `internal/comparison/comparison_test.go` | 192 | 11 unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `comparison` import; created comparison service and handlers; registered 18 routes with auth middleware; added 12 permission keys to `bootstrapPermissions` |

## Key Decisions

- **Sparse value storage** — `comparison_values` stores only dimensions that
  have values. Empty/null dimensions have no row. This avoids NULL columns in a
  wide table and makes the grid assembly efficient — the `GetComparisonGrid`
  query JOINs items with their values and the handler pivots the result into a
  matrix.
- **Dimension types drive validation** — the `dimension_type` column determines
  which value column (`text_value` or `number_value`) is populated. The service
  layer validates that the correct column is set for the dimension type.
- **Access control per type** — `comparison_type_access` links comparison types
  to allowed roles. The `CheckTypeAccess` query validates at query time. This
  allows admins to restrict comparison types (e.g., only verified users can
  compare visa consultants).
- **System types are protected** — `is_system = true` prevents deletion of
  default comparison types via the API. The service layer checks this flag
  before allowing delete operations.
- **Grid assembly in handler** — the `GetComparisonGrid` handler pivots the
  flat query result into a JSON structure: `{ items: [...], dimensions: [...],
  values: { item_id: { dimension_id: value } } }`. This avoids complex SQL
  pivoting and keeps the query simple.
- **Read/write pool separation** — standard JapaConnect pattern applied.

## Known Issues and Deferrals

- **Comparison sharing** — generating shareable comparison links is not
  implemented. Deferred to a future section.
- **Comparison templates** — pre-built comparison templates for common use
  cases are seeded as system types but the admin UI for creating custom
  templates is a frontend concern.
- **Comparison voting** — "which is better?" community voting on comparisons
  is not implemented. Deferred to a future section.
- **Entity resolution** — the grid returns `entity_type`/`entity_id` references.
  The frontend must resolve these to display names and links. This is a
  frontend concern.

## Next Steps

Section 33 — Professional Profiles. This section adds professional identity
and credential verification for diaspora professionals.
