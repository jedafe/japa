# Section 24 — Blog Extension: Implementation Complete

**Date:** 2026-06-26
**Phase:** 4
**Section:** 24
**Status:** Complete

## Overview

Section 24 extends the base content system (Section 14) with blog-specific
features: series (multi-part post collections), editorial review workflows,
featured post curation, and RSS feed generation. Blog posts use the existing
`contents` table with `content_type = 'blog_post'` and add metadata through the
`blog_series`, `blog_series_posts`, `content_reviews`, and `content_featured`
tables.

The editorial workflow allows editors to assign reviewers, track review status
(routing through pending/approved/rejected/changes_requested states), and
maintain a review history. Featured post curation enables manual and automatic
selection of posts for homepage display.

## Components Implemented

### Migration 000018 — Blog Extension

1 migration file (`migrations/000018_blog_extension.sql`, 127 lines); 4 tables
created:

**blog_series** — Multi-part post collections. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `author_user_id` — FK to `users` with `ON DELETE CASCADE`
- `title` — Series title (text, not null)
- `slug` — URL-safe slug (text, not null)
- `description` — Series description (text, nullable)
- `cover_image_url` — Cover image URL (text, nullable)
- `status` — Series status: `'draft'`, `'published'`, `'archived'` (default `'draft'`)
- `post_count` — Denormalized post count (integer, default `0`)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, slug)`

**blog_series_posts** — Many-to-many series/post ordering. Columns:
- `series_id` — FK to `blog_series` with `ON DELETE CASCADE`
- `content_id` — FK to `contents` with `ON DELETE CASCADE`
- `position` — Ordering position (integer, not null)
- `created_at` — Linking timestamp (default `now()`)
- Composite primary key on `(series_id, content_id)`

**content_reviews** — Editorial review records. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `content_id` — FK to `contents` with `ON DELETE CASCADE`
- `reviewer_user_id` — FK to `users` with `ON DELETE CASCADE`
- `status` — Review status: `'pending'`, `'approved'`, `'rejected'`, `'changes_requested'` (default `'pending'`)
- `notes` — Reviewer notes (text, nullable)
- `created_at` — Creation timestamp (default `now()`)
- `updated_at` — Update timestamp (default `now()`)
- Unique constraint on `(tenant_id, content_id, reviewer_user_id)` (one review per reviewer per content)

**content_featured** — Featured post curation. Columns:
- `id` — UUID primary key with `gen_random_uuid()`
- `tenant_id` — FK to `tenants` with `ON DELETE CASCADE`
- `content_id` — FK to `contents` with `ON DELETE CASCADE`
- `featured_by_user_id` — FK to `users` with `ON DELETE CASCADE`
- `position` — Display ordering (integer, default `0`)
- `starts_at` — Feature start timestamp (nullable, for time-limited featuring)
- `ends_at` — Feature end timestamp (nullable)
- `created_at` — Creation timestamp (default `now()`)
- Unique constraint on `(tenant_id, content_id)`

Indexes:
- `idx_blog_series_tenant` — `(tenant_id, created_at DESC)`
- `idx_blog_series_posts_series` — `(series_id, position)`
- `idx_content_reviews_tenant` — `(tenant_id, content_id, status)`
- `idx_content_featured_tenant` — `(tenant_id, position, starts_at, ends_at)`

### sqlc Queries (internal/db/queries/blog.sql)

22 queries in 1 SQL file (177 lines total):

**Series (8 queries):**
1. `InsertBlogSeries` — create new series
2. `GetBlogSeriesByID` — get series by ID
3. `GetBlogSeriesBySlug` — get series by slug
4. `ListBlogSeriesByTenant` — paginated series list
5. `UpdateBlogSeries` — update series metadata
6. `DeleteBlogSeries` — delete series
7. `AddPostToSeries` — link post to series at position
8. `RemovePostFromSeries` — unlink post from series

**Reviews (7 queries):**
9. `InsertContentReview` — create review assignment
10. `GetContentReviewByID` — get review by ID
11. `ListContentReviewsByContent` — list reviews for content
12. `UpdateContentReviewStatus` — update review status + notes
13. `DeleteContentReview` — remove review assignment
14. `GetPendingReviewCount` — count pending reviews for a reviewer
15. `ListReviewsByReviewer` — paginated reviews assigned to a user

**Featured (7 queries):**
16. `InsertContentFeatured` — feature a post
17. `GetContentFeaturedByID` — get featured record by ID
18. `ListFeaturedContent` — paginated featured posts (filters by active window)
19. `UpdateContentFeatured` — update position/dates
20. `RemoveContentFeatured` — unfeature a post
21. `IsContentFeatured` — check if content is currently featured
22. `ListExpiredFeatured` — find expired feature records (for cleanup task)

### Package: internal/blog/ (6 files)

**blog.go** (67 lines)

- Status constants: `StatusDraft`, `StatusPublished`, `StatusArchived`
- Review status constants: `ReviewPending`, `ReviewApproved`, `ReviewRejected`, `ReviewChangesRequested`
- Page size constants: `DefaultPageSize = 20`, `MaxPageSize = 100`
- `Service` struct with `*Store` and `*db.DB` dependency
- `NewService` constructor with reader/writer pool split

**store.go** (245 lines)

- `Store` struct with read/write `*gen.Queries` pool separation
- `NewStore` constructor
- 20 public methods wrapping sqlc-generated queries across series, reviews, and featured
- All errors wrapped with `fmt.Errorf("verb noun: %w", err)` for context

**service.go** (234 lines)

- Business logic for series, reviews, featured posts, and RSS generation
- `CreateSeries` — validates title, generates slug, inserts series
- `AssignReviewer` — validates reviewer exists, creates review record
- `UpdateReviewStatus` — validates status transitions, updates review
- `FeaturePost` — checks for expiry conflicts, inserts featured record
- `GetActiveFeatured` — filters by `starts_at`/`ends_at` window
- Input validation at service layer with descriptive error messages

**handlers.go** (489 lines)

- `Handlers` struct with `*Service` dependency
- `NewHandlers` constructor
- 16 HTTP handlers covering series, reviews, and featured posts
- Helpers: `requireTenant`, `requireUser`, `writeJSON`, `writeError`, `parsePageSize`

**rss.go** (78 lines)

- RSS feed generation for blog series and tenant-wide blog posts
- `GenerateSeriesRSS` — produces RSS 2.0 XML for a series with `<item>` elements
- `GenerateTenantRSS` — produces RSS 2.0 XML for all published blog posts
- Uses `encoding/xml` for XML marshaling with proper RSS 2.0 structure

**blog_test.go** (12 unit tests)

- Constants, RSS generation, status validation, slug generation, pagination helpers, JSON serialization

### Server Wiring (internal/server/server.go)

- `blog.NewService(database)` and `blog.NewHandlers(blogService)` created in `NewServer`
- 16 API routes registered under `/api/v1`, all within the tenant-scoped route group
- 12 new permission keys added to `bootstrapPermissions`

### Permission Keys

12 new permission keys bootstrapped at server startup:

| Key | Description |
|-----|-------------|
| `blog.series.create` | Create blog series |
| `blog.series.edit_own` | Edit own series |
| `blog.series.edit_any` | Edit any series |
| `blog.series.delete_own` | Delete own series |
| `blog.series.delete_any` | Delete any series |
| `blog.review.assign` | Assign reviewers |
| `blog.review.submit` | Submit review |
| `blog.review.moderate` | Moderate reviews (admin) |
| `blog.featured.manage` | Manage featured posts |
| `blog.featured.view` | View featured posts |
| `blog.rss.view` | View RSS feeds |
| `blog.editorial.view` | View editorial dashboard |

## Test Summary

| Package | Test Count | Result |
|---------|-----------|--------|
| internal/blog | 12 | PASS |

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/blog/ -count=1` | ok — 12 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## API Routes Added

16 routes registered under `/api/v1`, all within the tenant-scoped route group:

| # | Method | Path | Auth | Permission | Description |
|---|--------|------|------|------------|-------------|
| 1 | POST | `/blog/series` | Required | `blog.series.create` | Create blog series |
| 2 | GET | `/blog/series` | Required | — | List series |
| 3 | GET | `/blog/series/{id}` | Required | — | Get series by ID |
| 4 | PATCH | `/blog/series/{id}` | Required | `blog.series.edit_own` | Update series |
| 5 | DELETE | `/blog/series/{id}` | Required | `blog.series.delete_own` | Delete series |
| 6 | POST | `/blog/series/{id}/posts` | Required | `blog.series.edit_own` | Add post to series |
| 7 | DELETE | `/blog/series/{id}/posts/{post_id}` | Required | `blog.series.edit_own` | Remove post from series |
| 8 | POST | `/blog/reviews` | Required | `blog.review.assign` | Assign reviewer |
| 9 | GET | `/blog/reviews` | Required | `blog.editorial.view` | List reviews for content |
| 10 | PATCH | `/blog/reviews/{id}` | Required | `blog.review.submit` | Update review status |
| 11 | GET | `/blog/reviews/my` | Required | — | List my assigned reviews |
| 12 | POST | `/blog/featured` | Required | `blog.featured.manage` | Feature a post |
| 13 | GET | `/blog/featured` | Required | `blog.featured.view` | List featured posts |
| 14 | PATCH | `/blog/featured/{id}` | Required | `blog.featured.manage` | Update featured record |
| 15 | DELETE | `/blog/featured/{id}` | Required | `blog.featured.manage` | Unfeature a post |
| 16 | GET | `/blog/rss` | Public | — | RSS feed (tenant-wide) |

## Files Created

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000018_blog_extension.sql` | 127 | 4 tables for series, reviews, featured |
| 2 | `internal/db/queries/blog.sql` | 177 | 22 sqlc queries for blog entities |
| 3 | `internal/db/gen/blog.sql.go` | (generated) | Generated query methods and param structs |
| 4 | `internal/blog/blog.go` | 67 | Constants, Service struct |
| 5 | `internal/blog/store.go` | 245 | Store layer with read/write separation |
| 6 | `internal/blog/service.go` | 234 | Business logic for series, reviews, featured |
| 7 | `internal/blog/handlers.go` | 489 | 16 HTTP handlers |
| 8 | `internal/blog/rss.go` | 78 | RSS 2.0 feed generation |
| 9 | `internal/blog/blog_test.go` | 12 tests | Unit tests |

## Files Modified

| # | File | Change |
|---|------|--------|
| 1 | `internal/server/server.go` | Added `blog` import; created blog service and handlers; registered 16 routes with auth middleware; added 12 permission keys to `bootstrapPermissions` |

## Key Decisions

- **Series use the contents table** — blog series are metadata collections that
  reference `contents` rows via `blog_series_posts`. The series itself is not a
  content type; it is a grouping mechanism with its own table.
- **Editorial review is per-reviewer** — the unique constraint
  `(tenant_id, content_id, reviewer_user_id)` ensures one reviewer can only have
  one active review per content. Status transitions are validated in the service
  layer (e.g., cannot go from `rejected` to `pending`).
- **Featured posts have time windows** — `starts_at` and `ends_at` allow
  time-limited featuring (e.g., "feature this post for one week"). The
  `ListFeaturedContent` query filters by the current time. An Asynq cleanup task
  (Phase 6) will remove expired feature records.
- **RSS feeds are server-rendered** — no external RSS library. The `rss.go` file
  uses `encoding/xml` to produce standard RSS 2.0 XML. Feeds are available at
  `/api/v1/blog/rss` without authentication.
- **Read/write pool separation** — standard JapaConnect pattern applied to the
  blog store layer.

## Known Issues and Deferrals

- **RSS feed caching** — feeds are generated on every request. Caching with
  Valkey (Section 06) is deferred to Phase 6.
- **Featured post cleanup** — expired feature records are not automatically
  removed. An Asynq scheduled task for cleanup is deferred to Phase 6.
- **Series post ordering** — positions are managed manually. Drag-and-drop
  reordering in the admin UI is a frontend concern.
- **Review notifications** — notifying reviewers of assigned reviews is not
  wired to the Notification System (Section 35). Deferred to Phase 6.

## Next Steps

Section 29 — Universal Ratings & Reviews. This section adds a polymorphic
review system applicable to any entity type.
