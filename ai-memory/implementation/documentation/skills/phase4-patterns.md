# Phase 4 Implementation Patterns

**Date:** 2026-06-26
**Phase:** 4
**Purpose:** Implementation patterns learned during Phase 4 (Sections 08, 24, 29, 30, 33), reusable in future sections.

## Pattern: Polymorphic Entity Reference (entity_type + entity_id)

This pattern appears in four of the five Phase 4 sections. A TEXT discriminator
(`entity_type`) paired with a UUID (`entity_id`) allows a single table to
reference any entity type without foreign key constraints to a specific table.

```sql
CREATE TABLE reviews (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    entity_type text NOT NULL,
    entity_id   uuid NOT NULL,
    UNIQUE (tenant_id, entity_type, entity_id, reviewer_user_id)
);
```

The service layer validates `entity_type` against a known set:

```go
var validEntityTypes = map[string]bool{
    "content":      true,
    "space":        true,
    "professional": true,
    "comparison":   true,
}

func ValidateEntityType(t string) error {
    if !validEntityTypes[t] {
        return fmt.Errorf("invalid entity type: %s", t)
    }
    return nil
}
```

This pattern is used for:
- **Reviews** (Section 29) -- `reviews.entity_type` / `entity_id`
- **Forum votes** (Section 08) -- `forum_votes.entity_type` / `entity_id`
- **Forum comments** (Section 08) -- `forum_comments.entity_type` / `entity_id`
- **Comparison items** (Section 30) -- `comparison_items.entity_type` / `entity_id`

**Trade-off:** No FK constraints means the application must handle referential
integrity. Deleted entities leave orphaned review/vote rows. The service layer
should check entity existence before creating references.

## Pattern: Denormalized Counters

Phase 4 sections use denormalized counters to avoid COUNT queries on list
endpoints. The counter is updated synchronously on insert/delete operations:

```go
// On answer creation:
func (s *Service) CreateAnswer(ctx context.Context, threadID, authorID pgtype.UUID, body string) (gen.ForumAnswer, error) {
    answer, err := s.store.insertForumAnswer(ctx, ...)
    if err != nil {
        return gen.ForumAnswer{}, fmt.Errorf("inserting answer: %w", err)
    }
    // Increment answer_count on the thread
    if err := s.store.incrementThreadAnswerCount(ctx, threadID); err != nil {
        slog.Warn("failed to increment answer count", "thread_id", threadID, "error", err)
    }
    return answer, nil
}
```

Counters used:
- **Forum threads** -- `answer_count`, `comment_count`, `view_count`
- **Forum answers** -- `vote_score` (net upvotes minus downvotes)
- **Blog series** -- `post_count`
- **Professional profiles** -- `profile_views`

**Trade-off:** Denormalized counters can drift under concurrent writes. The
benefit is O(1) reads on list endpoints. A periodic recalculation task
(Phase 6) can repair drift.

## Pattern: Aggregate Recalculation on Write

Review aggregates (Section 29) are recalculated synchronously on every
insert/update/delete. The recalculation queries actual counts from the source
table:

```go
func (s *Service) RecalculateAggregate(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID) error {
    count, avg, err := s.store.recalculateReviewAggregate(ctx, gen.RecalculateReviewAggregateParams{
        TenantID:   tenantID,
        EntityType: entityType,
        EntityID:   entityID,
    })
    if err != nil {
        return fmt.Errorf("recalculating aggregate: %w", err)
    }

    return s.store.upsertReviewAggregate(ctx, gen.UpsertReviewAggregateParams{
        TenantID:      tenantID,
        EntityType:    entityType,
        EntityID:      entityID,
        ReviewCount:   count,
        AverageRating: avg,
    })
}
```

The SQL uses a subquery:

```sql
-- name: RecalculateReviewAggregate :one
SELECT
    COALESCE(COUNT(*)::int, 0) AS review_count,
    COALESCE(AVG(rating)::numeric(3,2), 0.00) AS average_rating
FROM reviews
WHERE tenant_id = @tenant_id
  AND entity_type = @entity_type
  AND entity_id = @entity_id
  AND status = 'published';
```

**Trade-off:** Synchronous recalculation is simple and consistent but adds
write latency. For high-traffic scenarios, move to an Asynq background task.

## Pattern: Slug Generation with Uniqueness

Every section that creates user-facing entities generates slugs. The pattern
is consistent across all sections:

```go
func Slugify(title string) string {
    slug := strings.ToLower(title)
    slug = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(slug, "-")
    slug = strings.Trim(slug, "-")
    if len(slug) > 200 {
        slug = slug[:200]
    }
    return slug
}

func EnsureUniqueSlug(ctx context.Context, store *Store, tenantID pgtype.UUID, baseSlug string) (string, error) {
    slug := baseSlug
    for i := 0; i < MaxSlugAttempts; i++ {
        exists, err := store.slugExists(ctx, tenantID, slug)
        if err != nil {
            return "", fmt.Errorf("checking slug: %w", err)
        }
        if !exists {
            return slug, nil
        }
        slug = fmt.Sprintf("%s-%d", baseSlug, i+2)
    }
    return "", fmt.Errorf("could not generate unique slug after %d attempts", MaxSlugAttempts)
}
```

`MaxSlugAttempts = 100` prevents infinite loops. The slug uniqueness check
queries the relevant table (contents, forum_threads, professional_profiles,
etc.) scoped to the tenant.

## Pattern: Status Workflow Validation

Several sections implement status transitions with validation:

```go
var validTransitions = map[string][]string{
    "pending":           {"approved", "rejected", "changes_requested"},
    "changes_requested": {"pending", "rejected"},
    "approved":          {},
    "rejected":          {"pending"},
}

func ValidateStatusTransition(current, next string) error {
    allowed, ok := validTransitions[current]
    if !ok {
        return fmt.Errorf("unknown status: %s", current)
    }
    for _, s := range allowed {
        if s == next {
            return nil
        }
    }
    return fmt.Errorf("invalid transition from %s to %s", current, next)
}
```

Used in:
- **Blog reviews** (Section 24) -- review status workflow
- **Forum solved marking** (Section 08) -- solved/unsolved transitions
- **Professional verification** (Section 33) -- verified/unverified

## Pattern: Comparison Grid Assembly

The comparison system (Section 30) assembles a grid (items x dimensions) in
the handler layer rather than in SQL. The handler fetches items, dimensions,
and values separately, then pivots the flat values into a nested map:

```go
grid := make(map[string]map[string]any)
for _, v := range values {
    itemKey := v.ItemID.String()
    dimKey := v.DimensionID.String()
    if grid[itemKey] == nil {
        grid[itemKey] = make(map[string]any)
    }
    if v.TextValue != nil {
        grid[itemKey][dimKey] = *v.TextValue
    } else if v.NumberValue != nil {
        grid[itemKey][dimKey] = *v.NumberValue
    }
}
```

**Trade-off:** Simple SQL query plus Go pivoting is easier to maintain than a
SQL PIVOT. The number of items per comparison is typically small (2-5),
so the in-memory pivot is negligible.

## Pattern: RSS Feed Generation Without External Libraries

Section 24 generates RSS 2.0 feeds using only `encoding/xml`. The RSS struct
hierarchy maps directly to the RSS 2.0 spec:

```go
type RSSFeed struct {
    XMLName xml.Name   `xml:"rss"`
    Version string     `xml:"version,attr"`
    Channel *RSSChannel `xml:"channel"`
}

type RSSChannel struct {
    Title       string    `xml:"title"`
    Link        string    `xml:"link"`
    Description string    `xml:"description"`
    Items       []RSSItem `xml:"item"`
}

type RSSItem struct {
    Title       string `xml:"title"`
    Link        string `xml:"link"`
    Description string `xml:"description"`
    PubDate     string `xml:"pubDate"`
    Author      string `xml:"author"`
}
```

The feed is rendered via `xml.Marshal` and returned with `Content-Type:
application/rss+xml`. No external RSS library is needed for standard RSS 2.0.

## Pattern: View Counter Increment

Professional profiles (Section 33) use a dedicated increment query to avoid
read-modify-write races:

```sql
-- name: IncrementProfileViews :exec
UPDATE professional_profiles
SET profile_views = profile_views + 1
WHERE id = @id AND tenant_id = @tenant_id;
```

This is an atomic increment that does not require reading the current value
first. The counter is eventually consistent (not returned in the same request)
but avoids race conditions under concurrent views.

## Lesson: One-Review-Per-User Constraint

The unique constraint on `(tenant_id, entity_type, entity_id, reviewer_user_id)`
in the reviews table prevents duplicate reviews. The service layer catches the
constraint violation and returns a descriptive error:

```go
_, err := s.store.insertReview(ctx, params)
if err != nil {
    if strings.Contains(err.Error(), "unique constraint") {
        return nil, fmt.Errorf("you have already reviewed this %s", entityType)
    }
    return nil, fmt.Errorf("inserting review: %w", err)
}
```

This pattern (catching unique violations and translating to user-friendly
errors) applies to any one-per-user constraint: forum votes, poll votes,
bookmarks, follows, etc.

## Lesson: Read/Write Pool Separation Consistency

All five Phase 4 sections follow the identical store pattern:

```go
type Store struct {
    write *gen.Queries
    read  *gen.Queries
}

func NewStore(read, write *gen.Queries) *Store {
    return &Store{read: read, write: write}
}
```

The Service layer constructs the Store from the database pool:

```go
func NewService(database *db.DB) *Service {
    return &Service{
        store: NewStore(
            gen.New(database.Reader),
            gen.New(database.Writer),
        ),
        db: database,
    }
}
```

This is the canonical pattern. Every new section should follow it without
variation. The reader pool can be pointed at a read replica in production
without changing any application code.

## Lesson: Permission Key Naming Convention

All Phase 4 permission keys follow the `{domain}.{entity}.{action}` pattern:

- `forum.thread.create` -- domain: forum, entity: thread, action: create
- `blog.series.edit_own` -- domain: blog, entity: series, action: edit_own
- `reviews.moderate` -- domain: reviews, action: moderate (no entity, applies globally)
- `comparison.grid.view` -- domain: comparison, entity: grid, action: view

The `_own` suffix distinguishes self-service actions from `_any` admin actions.
This convention is consistent across all sections and should be maintained.
