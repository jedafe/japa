# Phase 3 Feed Patterns

**Date:** 2026-06-26
**Phase:** 3
**Purpose:** Implementation patterns learned during Section 32 (Activity Feed / Timeline), reusable in future sections.

## Pattern: Hybrid Fan-Out Strategy (Push vs Pull Based on Space Size)

The Activity Feed uses a hybrid fan-out strategy to distribute posts to follower feeds. The decision point is the space member count relative to a configurable threshold:

```go
const FanOutThreshold = 5000

func (s *Service) FanOutPost(ctx context.Context, tenantID, contentID, spaceID pgtype.UUID, contentType string) error {
    // TODO: In Phase 6 (Asynq), this will be an async task that:
    // 1. Queries space member count
    // 2. If ≤ FanOutThreshold: fetch followers, insert feed rows
    // 3. If > FanOutThreshold: mark in hot cache for pull
    return nil
}
```

**Push mode** (≤ 5000 members): Write a `feeds` row for each follower. The primary feed reads from the `feeds` table. This is fast at read time but expensive at write time — acceptable for small spaces.

**Pull mode** (> 5000 members): Do not write feed rows. Instead, mark the content in a hot cache (deferred to Phase 6). Followers pull from `contents` + `content_scores` on their next request, filtered by followed spaces. This is cheap at write time but requires a more complex read query.

The threshold is a constant, not a database value, so it can be tuned without a migration. The choice is per-space, not per-tenant — a space with 3000 members uses push, while a space with 7000 members uses pull, even within the same tenant.

**When to reuse this pattern:** Any system that distributes content to a variable number of recipients — notification fan-out, email digest, real-time updates. The push/pull decision point based on recipient count is the core insight.

## Pattern: Velocity Score Computation (HN Formula)

Content ranking uses a velocity score inspired by Hacker News: `engagement / (hours_old + 2)^1.8`. The engagement metric combines four signals:

```go
func ComputeVelocity(voteScore, commentCount, viewCount, bookmarkCount int32, hoursOld float64) float64 {
    engagement := float64(voteScore + commentCount + viewCount + bookmarkCount)
    denominator := math.Pow(hoursOld+2, 1.8)
    if denominator == 0 {
        return 0
    }
    return engagement / denominator
}
```

Key properties of this formula:
- **Time decay**: The `+2` offset prevents division by zero for very new posts (0.1 hours old → denominator ≈ 2.1). The `1.8` exponent provides aggressive decay — a post at 24 hours old has denominator ≈ 26^1.8 ≈ 450, while a post at 1 hour old has denominator ≈ 3^1.8 ≈ 7.2.
- **Linear engagement scaling**: Doubling engagement doubles velocity. This means a post with 100 votes always ranks higher than a post with 50 votes, regardless of age.
- **Pure function**: `ComputeVelocity` has no side effects, making it trivially testable with table-driven tests.

The same formula is implemented in PostgreSQL for batch updates:

```sql
UPDATE content_scores cs
SET velocity_score = CASE
    WHEN EXTRACT(EPOCH FROM (now() - c.created_at)) / 3600 + 2 > 0
    THEN (cs.vote_score + cs.comment_count + cs.view_count + cs.bookmark_count)::numeric
         / (POWER(EXTRACT(EPOCH FROM (now() - c.created_at)) / 3600 + 2, 1.8))
    ELSE 0
END,
last_scored_at = now()
FROM contents c
WHERE cs.content_id = c.id
  AND c.tenant_id = $1
  AND c.status = 'published'
  AND c.created_at > now() - interval '7 days';
```

The PostgreSQL version uses `EXTRACT(EPOCH FROM ...)` to compute hours and `POWER()` for the exponent. The `CASE WHEN ... ELSE 0` guard handles edge cases where the denominator would be zero.

**When to reuse this pattern:** Any ranking system that needs time-decaying relevance — trending topics, "hot" lists, recommendation feeds. The HN formula is a well-understood baseline that can be tuned by adjusting the exponent.

## Pattern: Multi-Timeline Feed Query Pattern

A single `GetFeed` method routes to the correct timeline and sort combination via a switch statement. Each timeline has its own private method that handles the query:

```go
func (s *Service) GetFeed(ctx context.Context, tenantID pgtype.UUID, timeline string, userID *pgtype.UUID, spaceID *pgtype.UUID, authorID *pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
    // Clamp pagination limits.
    if limit <= 0 || limit > MaxPageSize {
        limit = DefaultPageSize
    }
    if offset < 0 {
        offset = 0
    }

    switch timeline {
    case "explore":
        return s.getExploreFeed(ctx, tenantID, sort, limit, offset)
    case "primary":
        if userID == nil {
            return nil, fmt.Errorf("primary feed requires user_id")
        }
        return s.getPrimaryFeedItems(ctx, tenantID, *userID, sort, limit, offset)
    case "space":
        if spaceID == nil {
            return nil, fmt.Errorf("space feed requires space_id")
        }
        return s.getSpaceFeedItems(ctx, tenantID, *spaceID, sort, limit, offset)
    case "profile":
        if authorID == nil {
            return nil, fmt.Errorf("profile feed requires author_id")
        }
        return s.getProfileFeedItems(ctx, tenantID, *authorID, sort, limit, offset)
    default:
        return nil, fmt.Errorf("unknown timeline: %s", timeline)
    }
}
```

Each private method then dispatches on sort:

```go
func (s *Service) getExploreFeed(ctx context.Context, tenantID pgtype.UUID, sort string, limit, offset int32) ([]FeedItem, error) {
    switch sort {
    case SortTop:
        rows, err := s.store.getTopFeed(ctx, tenantID, limit, offset)
        if err != nil {
            return nil, err
        }
        return topRowsToItems(rows), nil
    case SortRising:
        rows, err := s.store.getRisingFeed(ctx, tenantID, limit, offset)
        if err != nil {
            return nil, err
        }
        return risingRowsToItems(rows), nil
    default: // "latest" or "relevant" both use velocity_score DESC
        rows, err := s.store.getExploreFeed(ctx, tenantID, limit, offset)
        if err != nil {
            return nil, err
        }
        return exploreRowsToItems(rows), nil
    }
}
```

The nil-parameter validation (`if userID == nil`) is an early-exit guard clause — it prevents downstream panics from dereferencing nil pointers. The error message explicitly names the missing parameter for debugging.

**When to reuse this pattern:** Any API endpoint that serves multiple "views" of the same data — dashboard widgets, search results with different filters, notification feeds with different scopes. The switch-based dispatch keeps the routing logic in one place while allowing each view to have its own query.

## Pattern: Content Score Reuse (Existing Table from Section 14)

Rather than creating a new `feed_scores` table, the feed system JOINs the existing `content_scores` table from Section 14. This keeps the schema DRY — engagement metrics (votes, comments, views, bookmarks) and the velocity score are stored once and read by both the content system and the feed system:

```sql
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score
FROM contents c
JOIN content_scores cs ON cs.content_id = c.id
WHERE c.tenant_id = @tenant_id
  AND c.status = 'published'
  AND c.visibility = 'public'
ORDER BY cs.velocity_score DESC, cs.vote_score DESC
LIMIT @page_limit OFFSET @page_offset;
```

The `content_scores` table was created in migration 000010 (Section 14) with columns for `vote_score`, `comment_count`, `view_count`, `bookmark_count`, `velocity_score`, `relevance_embedding` (pgvector, for future use), and `last_scored_at`. The feed system reads `velocity_score` and `vote_score` for ranking, and `UpdateContentScore` writes to the same table when engagement metrics change.

The `content_scores` table is referenced by foreign key from the `contents` table (via `content_id`), so feed queries JOIN on `cs.content_id = c.id` — a simple primary key join that is always efficient.

**When to reuse this pattern:** Any time two systems need the same derived data. Instead of duplicating the data, create it once and have both systems read from the same table. The JOIN cost is negligible compared to the consistency benefits of a single source of truth.

## Pattern: Public vs Authenticated Endpoint Pattern

The feed system exposes four endpoints with two auth levels: explore is public (no auth), while primary, space, and profile require authentication. The handlers use `optionalUser` to distinguish between the two cases:

```go
func (h *Handlers) GetExploreFeed(w http.ResponseWriter, r *http.Request) {
    tenantID, ok := h.requireTenant(w, r)
    if !ok {
        return
    }
    // No auth check — public endpoint
    limit := parsePageSize(r)
    offset := parsePageOffset(r)
    sort := parseSort(r)
    items, err := h.service.GetFeed(r.Context(), tenantID, "explore", nil, nil, nil, sort, limit, offset)
    // ...
}

func (h *Handlers) GetPrimaryFeed(w http.ResponseWriter, r *http.Request) {
    user := h.optionalUser(r)
    if user == nil {
        writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
        return
    }
    // Auth required — use user.UserID
    tenantID, ok := h.requireTenant(w, r)
    if !ok {
        return
    }
    items, err := h.service.GetFeed(r.Context(), tenantID, "primary", &user.UserID, nil, nil, sort, limit, offset)
    // ...
}
```

The server wiring uses `auth.RequireAuth(kr)` middleware for authenticated routes and no middleware for public routes:

```go
r.Get("/feed/explore", feedHandlers.GetExploreFeed) // public — no auth required
r.With(auth.RequireAuth(kr)).Get("/feed/primary", feedHandlers.GetPrimaryFeed)
r.With(auth.RequireAuth(kr)).Get("/feed/space/{space_id}", feedHandlers.GetSpaceFeed)
r.With(auth.RequireAuth(kr)).Get("/feed/profile/{user_id}", feedHandlers.GetProfileFeed)
```

This pattern keeps the auth decision at the route level (declarative) rather than inside the handler (imperative). The explore feed is intentionally public for SEO and guest user acquisition.

**When to reuse this pattern:** Any system that needs both public and authenticated views of the same data — public profiles with private details, public posts with private drafts, public leaderboards with private stats. The key insight is that the auth decision is at the route level, not the handler level.

## Pattern: Asynq Task Definition Pattern (Placeholder Contract)

The feed system defines Asynq task names and payload structs now, even though the actual task wiring is deferred to Phase 6. This establishes a stable interface that Phase 6 can implement without changing the feed package:

```go
// Task names for Asynq queue.
const (
    TaskFanOutToFeeds        = "content:fan_out_to_feeds"
    TaskUpdateVelocityScores = "maintenance:update_velocity_scores"
)

// FanOutPayload is the payload for the fan_out_to_feeds task.
type FanOutPayload struct {
    ContentID   string `json:"content_id"`
    SpaceID     string `json:"space_id"`
    ContentType string `json:"content_type"`
}

// UpdateVelocityPayload is the payload for the update_velocity_scores task.
type UpdateVelocityPayload struct {
    TenantID string `json:"tenant_id"`
}
```

The `FanOutPost` service method is a no-op that returns nil:

```go
func (s *Service) FanOutPost(ctx context.Context, tenantID, contentID, spaceID pgtype.UUID, contentType string) error {
    // TODO: In Phase 6 (Asynq), this will be an async task that:
    // 1. Queries space member count
    // 2. If ≤ FanOutThreshold: fetch followers, insert feed rows
    // 3. If > FanOutThreshold: mark in hot cache for pull
    return nil
}
```

This placeholder pattern has three benefits:
1. **Interface stability** — the method signature is defined and tested now. Phase 6 only needs to fill in the implementation.
2. **Testability** — unit tests can call `FanOutPost` and verify it returns nil. Integration tests can mock the task enqueuer.
3. **Documentation** — the TODO comment describes exactly what Phase 6 will implement, so the next developer knows the contract.

**When to reuse this pattern:** Any time an async task is needed but the queue infrastructure is not yet ready. Define the task name, payload struct, and a no-op service method. This decouples the feature from the infrastructure and allows incremental delivery.

## Pattern: Row-to-Item Converter Functions

The feed system uses dedicated converter functions to map sqlc row types to the normalized `FeedItem` struct. Each timeline/sort combination has its own converter because the sqlc row types differ:

```go
func exploreRowsToItems(rows []gen.GetExploreFeedRow) []FeedItem {
    items := make([]FeedItem, len(rows))
    for i, r := range rows {
        items[i] = FeedItem{
            ID:            uuidToString(r.ID),
            Title:         r.Title,
            Slug:          r.Slug,
            ContentType:   r.ContentType,
            AuthorID:      uuidToString(r.AuthorID),
            SpaceID:       nullableUUIDToString(r.SpaceID),
            CreatedAt:     r.CreatedAt.Time.String(),
            ViewCount:     r.ViewCount,
            CommentCount:  r.CommentCount,
            VoteScore:     r.VoteScore,
            VelocityScore: pgNumericToFloat64(r.VelocityScore),
        }
    }
    return items
}
```

The converters handle type mapping: `pgtype.UUID` → `string`, nullable `pgtype.UUID` → `*string`, `pgtype.Timestamptz` → `string`, `pgtype.Numeric` → `float64`. This isolates the type conversion logic from the business logic in the service methods.

Empty input is handled gracefully — all converters return an empty slice for nil input:

```go
func TestExploreRowsToItemsEmpty(t *testing.T) {
    items := exploreRowsToItems(nil)
    if len(items) != 0 {
        t.Errorf("expected empty slice, got %d items", len(items))
    }
}
```

**When to reuse this pattern:** Any time you have multiple sqlc query result types that need to map to a single API response type. The converters keep the mapping logic testable and separate from the query logic.

## Lesson: Pagination Clamping is a Two-Layer Defense

Page size is clamped in both the handler (user-facing) and the store (safety net):

```go
// Handler layer
func parsePageSize(r *http.Request) int32 {
    s := r.URL.Query().Get("page_size")
    if s == "" {
        return DefaultPageSize
    }
    n, err := strconv.Atoi(s)
    if err != nil || n <= 0 {
        return DefaultPageSize
    }
    if n > MaxPageSize {
        return MaxPageSize
    }
    return int32(n)
}

// Store layer
func (s *Store) getExploreFeed(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.GetExploreFeedRow, error) {
    if limit <= 0 || limit > MaxPageSize {
        limit = DefaultPageSize
    }
    // ... query
}
```

The handler layer is the primary defense — it catches invalid input from HTTP requests. The store layer is a safety net — it catches invalid input from any code path that calls the store directly (e.g., internal batch jobs, tests). This double-clamping prevents accidental large result sets from any entry point.

## Lesson: Empty Feed Queries Return Empty Slices, Not Errors

When a user has no feed items (new account, no follows), the feed endpoints return an empty array, not an error. The sqlc `Query` method returns an empty slice for zero rows, and the converters handle this gracefully:

```go
func exploreRowsToItems(rows []gen.GetExploreFeedRow) []FeedItem {
    items := make([]FeedItem, len(rows)) // len(nil) == 0, so items is empty slice
    for i, r := range rows {             // loop doesn't execute
        // ...
    }
    return items // returns []FeedItem{}, not nil
}
```

The handler writes `{"data": []}` for empty feeds, which is a valid JSON response that the frontend can render as "No posts yet" without special-casing null. This is a deliberate UX decision — an empty array is semantically different from null (which would indicate an error).
