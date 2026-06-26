# Section 29 — Universal Ratings & Reviews: Implementation Complete

**Date:** 2026-06-26
**Phase:** 4
**Section:** 29
**Status:** Complete

## Overview

Section 29 implements a universal, polymorphic ratings and reviews system for
JapaConnect. Any entity type (content, space, professional profile, comparison
item) can be rated and reviewed through a shared `reviews` table with
`entity_type`/`entity_id` discrimination. A `review_aggregates` table
denormalizes review counts and average ratings per entity, maintained via
service-layer recalculation.

The system supports 1-5 star ratings, review titles and bodies, status workflow
(published/flagged/removed), and a one-review-per-user-per-entity constraint.
Review aggregates are recalculated on every insert/update/delete to keep the
denormalized data consistent.

## Components Implemented

### Migration 000019 — Ratings & Reviews

1 migration file (`migrations/000019_ratings_reviews.sql`, 68 lines); 2 tables
created:

**reviews** — Polymorphic reviews. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `entity_type` — Discriminator: `'content'`, `'space'`, `'professional'`, `'comparison'` (text, not null)
- `entity_id` — UUID of the reviewed entity (not null)
- `reviewer_user_id` — FK to `users` with `ON DELETE CASCADE`
- `rating` — Star rating 1-5 (smallint, not null, CHECK constraint)
- `title` — Review title (text, nullable)
- `body` — Review body (text, nullable)
- `status` — Review status: `'published'`, `'flagged'`, `'removed'` (default `'published'`)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, entity_type, entity_id, reviewer_user_id)`

**review_aggregates** — Denormalized aggregates per entity. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `entity_type` — Discriminator (text, not null)
- `entity_id` — UUID of the entity (not null)
- `review_count` — Number of published reviews (integer, default `0`)
- `average_rating` — Average rating (numeric(3,2), default `0.00`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, entity_type, entity_id)`

Indexes:
- `idx_reviews_entity` — `(tenant_id, entity_type, entity_id, status, created_at DESC)`
- `idx_reviews_reviewer` — `(tenant_id, reviewer_user_id, created_at DESC)`
- `idx_review_aggregates_entity` — `(tenant_id, entity_type, entity_id)`

### sqlc Queries (internal/db/queries/reviews.sql)

11 queries in 1 SQL file (89 lines total):

**Reviews (8 queries):**
1. `InsertReview` — create new review
2. `GetReviewByID` — get review by ID
3. `GetReviewByUserAndEntity` — get user's review for an entity (unique constraint lookup)
4. `ListReviewsByEntity` — paginated reviews for an entity (filtered by status)
5. `ListReviewsByUser` — paginated reviews by a user
6. `UpdateReview` — update rating, title, body
7. `UpdateReviewStatus` — update status (moderation)
8. `DeleteReview` — hard delete a review

**Aggregates (3 queries):**
9. `UpsertReviewAggregate` — insert or update aggregate (ON CONFLICT)
10. `GetReviewAggregate` — get aggregate for an entity
11. `RecalculateReviewAggregate` — recalculate from actual reviews (COUNT, AVG)

### Package: internal/reviews/ (5 files)

**reviews.go** (54 lines)

- Entity type constants: `EntityTypeContent`, `EntityTypeSpace`, `EntityTypeProfessional`, `EntityTypeComparison`
- Status constants: `StatusPublished`, `StatusFlagged`, `StatusRemoved`
- Rating constants: `MinRating = 1`, `MaxRating = 5`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (189 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 10 public methods wrapping sqlc-generated queries
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**service.go** (223 lines)

- Business logic for reviews and aggregates
- `CreateReview` — validates rating range, checks one-review-per-user constraint, inserts review, recalculates aggregate
- `UpdateReview` — validates ownership or moderation permission, updates review, recalculates aggregate
- `DeleteReview` — deletes review, recalculates aggregate
- `RecalculateAggregate` — queries actual COUNT/AVG from reviews table, upserts aggregate
- Input validation at service layer with descriptive error messages

**handlers.go** (347 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 7 HTTP handlers covering reviews and aggregates
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`, `parsePageSize`

**reviews_test.go** (8 unit tests)

- Constants, entity types, status values, rating validation, JSON serialization, pagination helpers

### Server Wiring (internal/server/server.go)

- `reviews.NewService(database)` and `reviews.NewHandlers(reviewsService)` created in `NewServer`
- 7 API routes registered under `/api/v1`, all within the tenant-scoped route group
- 8 new permission keys added to `bootstrapPermissions`

### Permission Keys

8 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `reviews.create` | Create reviews |
| `reviews.edit_own` | Edit own reviews |
| `reviews.edit_any` | Edit any review (moderator) |
| `reviews.delete_own` | Delete own reviews |
| `reviews.delete_any` | Delete any review (moderator) |
| `reviews.flag` | Flag reviews for moderation |
| `reviews.moderate` | Moderate flagged reviews |
| `reviews.aggregate.view` | View review aggregates |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/reviews | 8 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/reviews/ -count=1` | ok — 8 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

7 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/reviews` | Required | `reviews.create` | Create a review |
| 2 | GET | `/reviews/{entity_type}/{entity_id}` | Required | — | List reviews for an entity |
| 3 | GET | `/reviews/{id}` | Required | — | Get review by ID |
| 4 | PATCH | `/reviews/{id}` | Required | `reviews.edit_own` | Update review |
| 5 | DELETE | `/reviews/{id}` | Required | `reviews.delete_own` | Delete review |
| 6 | GET | `/reviews/aggregate/{entity_type}/{entity_id}` | Required | `reviews.aggregate.view` | Get review aggregate |
| 7 | PATCH | `/reviews/{id}/status` | Required | `reviews.moderate` | Update review status (moderation) |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000019_ratings_reviews.sql` | 68 | 2 tables for reviews and aggregates |
| 2 | `internal/db/queries/reviews.sql` | 89 | 11 sqlc queries |
| 3 | `internal/db/gen/reviews.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/reviews/reviews.go` | 54 | Constants, entity types, Service struct |
| 5 | `internal/reviews/store.go` | 189 | Store layer with read/write separation |
| 6 | `internal/reviews/service.go` | 223 | Business logic for reviews and aggregates |
| 7 | `internal/reviews/handlers.go` | 347 | 7 HTTP handlers |
| 8 | `internal/reviews/reviews_test.go` | 8 tests | Unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `reviews` import; created reviews service and handlers; registered 7 routes with auth middleware; added 8 permission keys to `bootstrapPermissions` |

## Key Decisions

- **Polymorphic via entity_type/entity_id** — the same pattern used for
  reactions (Section 14) and notifications (Section 35). `entity_type` is TEXT
  (not FK) because the referenced table varies per entity. The service layer
  validates entity_type against the allowed set.
- **One review per user per entity** — enforced by the unique constraint on
  `(tenant_id, entity_type, entity_id, reviewer_user_id)`. The service layer
  checks this constraint and returns a descriptive error on duplicate.
- **Aggregate recalculation is synchronous** — `RecalculateAggregate` runs
  COUNT/AVG on every review insert/update/delete. For high-traffic scenarios,
  this could be moved to an Asynq task (Phase 6), but synchronous recalculation
  ensures consistency for the current scale.
- **Rating CHECK constraint** — `rating >= 1 AND rating <= 5` enforced at the
  database level. The service layer also validates before insertion.
- **Hard delete for reviews** — unlike soft delete in other sections, reviews
  are hard-deleted because they have no downstream references. The aggregate is
  recalculated after deletion.
- **Read/write pool separation** — standard JapaConnect pattern applied.

## Known Issues and Deferrals

- **Review helpfulness votes** — "was this review helpful?" voting is not
  implemented. Deferred to a future section.
- **Review images** — attaching images to reviews is not supported. Deferred to
  a future section.
- **Aggregate recalculation scaling** — synchronous recalculation may become a
  bottleneck at scale. Asynq-based deferred recalculation is deferred to Phase 6.
- **Review search** — searching reviews by content is not implemented. Full-text
  search via Meilisearch (Section 20) will cover this.

## Next Steps

Section 30 — Generic Comparison System. This section enables structured
side-by-side comparisons of any entity type.
