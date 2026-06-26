# Phase 3 Follow Patterns

**Date:** 2026-06-26
**Phase:** 3
**Purpose:** Implementation patterns learned during Section 31 (Follow System), reusable in future sections.

## Pattern: Polymorphic Follow (Single Table, entity_type + entity_id)

The Follow System uses a single `follows` table to handle all entity types — users, Spaces, Tags, Organizations, and Professional Profiles — via a `text` discriminator column (`entity_type`) and a UUID `entity_id`. This avoids a table-per-entity explosion while keeping the schema extensible. The entity type is validated at the application layer, not via a database enum, so new entity types can be added without a migration:

```go
const (
    EntityTypeUser                = "user"
    EntityTypeSpace               = "space"
    EntityTypeTag                 = "tag"
    EntityTypeOrganization        = "organization"
    EntityTypeProfessionalProfile = "professional_profile"
)

func isValidEntityType(entityType string) bool {
    switch entityType {
    case EntityTypeUser, EntityTypeSpace, EntityTypeTag,
         EntityTypeOrganization, EntityTypeProfessionalProfile:
        return true
    default:
        return false
    }
}
```

The database CHECK constraint (`entity_type IN (...)`) enforces data integrity at rest, but the application-layer validation is the first gate. This two-layer validation pattern is intentional: the DB constraint is a safety net, not the primary validator, because adding new types requires both layers to stay in sync — and the application layer is the one that matters for API consumers.

The unique constraint on `(follower_id, entity_type, entity_id)` prevents duplicate follows without a separate dedup table:

```sql
CREATE TABLE follows (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    follower_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_type     text NOT NULL CHECK (entity_type IN ('user', 'space', 'tag', 'organization', 'professional_profile')),
    entity_id       uuid NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(follower_id, entity_type, entity_id)
);
```

`ON CONFLICT (follower_id, entity_type, entity_id) DO NOTHING` in the INSERT query silently skips duplicate follow attempts — the toggle logic lives in the store layer, which checks first and deletes on conflict.

## Pattern: Toggle Follow (Check-Then-Insert/Delete)

Toggle follow is the core operation. The store layer performs a check-then-act cycle: query whether the follow exists, then either delete (unfollow) or insert (follow). The return value is a boolean indicating the final state — `true` means the user is now following, `false` means they unfollowed:

```go
func (s *Store) toggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
    // First check if the follow already exists.
    existing, err := s.read.GetFollow(ctx, gen.GetFollowParams{
        FollowerID: followerID,
        EntityType: entityType,
        EntityID:   entityID,
    })
    if err == nil && existing.ID.Valid {
        // Already following — delete (unfollow).
        if err := s.write.DeleteFollowByFollowerEntity(ctx, gen.DeleteFollowByFollowerEntityParams{
            FollowerID: followerID,
            EntityType: entityType,
            EntityID:   entityID,
        }); err != nil {
            return false, fmt.Errorf("deleting follow: %w", err)
        }
        return false, nil
    }

    // Not following — insert.
    if err := s.write.InsertFollow(ctx, gen.InsertFollowParams{
        TenantID:   tenantID,
        FollowerID: followerID,
        EntityType: entityType,
        EntityID:   entityID,
    }); err != nil {
        return false, fmt.Errorf("inserting follow: %w", err)
    }
    return true, nil
}
```

Self-follow prevention is enforced at the service layer, before the store is called:

```go
func (s *Service) ToggleFollow(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityID pgtype.UUID) (bool, error) {
    if !isValidEntityType(entityType) {
        return false, fmt.Errorf("invalid entity type: %s", entityType)
    }
    if entityType == EntityTypeUser && followerID == entityID {
        return false, fmt.Errorf("cannot follow yourself")
    }
    // ... call store.toggleFollow
}
```

The self-follow check is entity-type-aware — only prevents self-following for the `user` type, not for spaces, tags, or other entities where the user's ID and the entity's ID could legitimately differ.

## Pattern: Service-Layer Follower Count Maintenance

Follower counts are denormalized on entity tables (`users.follower_count`, `spaces.follower_count`, `tags.follower_count`) and maintained in the service layer via raw SQL UPDATE. This is a deliberate choice over database triggers:

**Why not triggers:** Some entity tables (organizations, professional_profiles) do not exist yet when Section 31 ships. Triggers would require either creating stub tables or conditional trigger definitions. The service layer approach lets the code gracefully skip entities whose tables don't exist:

```go
func (s *Service) updateFollowerCount(ctx context.Context, entityType string, entityID pgtype.UUID, delta int32) error {
    var table string
    switch entityType {
    case EntityTypeUser:
        table = "users"
    case EntityTypeSpace:
        table = "spaces"
    case EntityTypeTag:
        table = "tags"
    default:
        // Organizations and professional_profiles don't have tables yet.
        return nil
    }

    query := fmt.Sprintf(
        "UPDATE %s SET follower_count = GREATEST(0, follower_count + $1) WHERE id = $2",
        table,
    )
    _, err := s.db.Writer.Exec(ctx, query, delta, entityID)
    if err != nil {
        return fmt.Errorf("updating follower count on %s: %w", table, err)
    }
    return nil
}
```

The `GREATEST(0, ...)` clause prevents negative counts from race conditions. The count update runs after the follow/unfollow operation succeeds, and a failure here is logged but does not fail the follow operation — the denormalized count is eventually consistent, not strongly consistent.

**Pattern for adding follower_count to new entity tables:** When a new entity table is created (e.g., `organizations`), add a `follower_count int NOT NULL DEFAULT 0` column and add a `case` branch in `updateFollowerCount`. No migration changes needed for the follows table itself.

The toggle handler calls `updateFollowerCount` with `delta = 1` for follow, `delta = -1` for unfollow:

```go
delta := int32(-1)
if followed {
    delta = 1
}
if err := s.updateFollowerCount(ctx, entityType, entityID, delta); err != nil {
    // Log but don't fail the follow operation.
    fmt.Printf("warning: failed to update follower count for %s/%s: %v\n", entityType, entityID, err)
}
```

## Pattern: Bulk Following Status Check (N+1 Prevention)

When listing entities (followers, spaces, tags), the UI needs to show whether the current user follows each one. The naive approach fires one `IsFollowing` query per entity — an N+1 disaster. The bulk check solves this with a single query that uses `EXISTS` as a correlated subquery in the SELECT clause:

```sql
-- name: GetBulkFollowingStatus :many
SELECT f.entity_type, f.entity_id, EXISTS (
    SELECT 1 FROM follows sub
    WHERE sub.follower_id = @follower_id
      AND sub.entity_type = f.entity_type
      AND sub.entity_id = f.entity_id
) AS is_following
FROM follows f
WHERE f.tenant_id = @tenant_id
  AND f.entity_type = @entity_type
  AND f.entity_id = ANY(@entity_ids::uuid[])
GROUP BY f.entity_type, f.entity_id;
```

The store layer converts the result into a `map[pgtype.UUID]bool` for O(1) lookup in the handler:

```go
func (s *Store) getBulkFollowingStatus(ctx context.Context, tenantID, followerID pgtype.UUID, entityType string, entityIDs []pgtype.UUID) (map[pgtype.UUID]bool, error) {
    if len(entityIDs) == 0 {
        return map[pgtype.UUID]bool{}, nil
    }
    rows, err := s.read.GetBulkFollowingStatus(ctx, gen.GetBulkFollowingStatusParams{
        FollowerID: followerID,
        TenantID:   tenantID,
        EntityType: entityType,
        EntityIds:  entityIDs,
    })
    if err != nil {
        return nil, fmt.Errorf("getting bulk following status: %w", err)
    }
    status := make(map[pgtype.UUID]bool, len(rows))
    for _, r := range rows {
        status[r.EntityID] = r.IsFollowing
    }
    return status, nil
}
```

The `len(entityIDs) == 0` guard is an early-exit that prevents an empty IN clause from producing unexpected results. The `ANY(@entity_ids::uuid[])` PostgreSQL array syntax is the idiomatic way to pass a list of UUIDs to a sqlc query.

## Pattern: Entity Name Resolution via CASE Subqueries

The `GetFollowingByUser` query needs to display the name of each followed entity, but entity names live in different tables — `users.username`, `spaces.name`, `tags.name`. Instead of joining all three tables, a single CASE subquery resolves the name based on entity type:

```sql
-- name: GetFollowingByUser :many
SELECT f.id, f.tenant_id, f.follower_id, f.entity_type, f.entity_id, f.created_at,
    CASE f.entity_type
        WHEN 'user' THEN (SELECT username FROM users WHERE id = f.entity_id)
        WHEN 'space' THEN (SELECT name FROM spaces WHERE id = f.entity_id)
        WHEN 'tag' THEN (SELECT name FROM tags WHERE id = f.entity_id)
        ELSE ''
    END AS entity_name
FROM follows f
WHERE f.tenant_id = @tenant_id AND f.follower_id = @follower_id
ORDER BY f.created_at DESC
LIMIT @page_limit OFFSET @page_offset;
```

The `ELSE ''` clause handles entity types whose tables don't exist yet — the query returns an empty string instead of failing. The service layer maps the result into a `FollowingInfo` struct with the resolved `EntityName`:

```go
type FollowingInfo struct {
    gen.Follow
    EntityName string `json:"entity_name"`
}
```

This pattern is preferable to three LEFT JOINs because:
1. The subquery is only evaluated for the matching entity type (not all three tables).
2. It avoids column name collisions between tables with different name column names (`username` vs `name`).
3. Adding a new entity type requires only adding a new CASE branch, not a new JOIN.

## Pattern: Tenant-Scoped Polymorphic Queries

Every query in the follows system includes a `tenant_id` filter, even though follows span multiple entity types. This is critical for multi-tenancy: a follow relationship is scoped to the tenant that created it, not globally visible. The pattern is consistent across all list and count queries:

```sql
-- Followers of an entity
WHERE f.tenant_id = @tenant_id AND f.entity_type = @entity_type AND f.entity_id = @entity_id

-- Following by a user
WHERE f.tenant_id = @tenant_id AND f.follower_id = @follower_id

-- Bulk status check
WHERE f.tenant_id = @tenant_id AND f.entity_type = @entity_type AND f.entity_id = ANY(@entity_ids::uuid[])
```

The tenant ID is resolved from the request context via tenant middleware, not from the URL path or query parameters. This prevents tenant ID spoofing — the middleware injects the tenant, and handlers read it from context:

```go
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
    t := tenant.FromContext(r.Context())
    if t == nil {
        writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
        return pgtype.UUID{}, false
    }
    return t.ID, true
}
```

The tenant middleware runs before all follow handlers, so the tenant ID is always available and trustworthy. This pattern applies to every new section that introduces polymorphic queries — the `tenant_id` filter is non-negotiable.

## Pattern: Read/Write Pool Separation in Store Layer

The Store struct holds two `gen.Queries` instances — one for the reader connection pool, one for the writer pool. Write operations (INSERT, DELETE, UPDATE) use `s.write`, read operations (SELECT) use `s.read`. Both pools connect to the same sqlc-generated queries but route to different PostgreSQL connection pools:

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
        store: &Store{
            read:  gen.New(database.Reader),
            write: gen.New(database.Writer),
        },
        db: database,
    }
}
```

This separation is the standard pattern for all sections in JapaConnect. The reader pool can be pointed at a read replica in production without changing any application code. The `db.Writer` pool is also used directly for the raw follower count UPDATE in `updateFollowerCount`, bypassing sqlc because the table name is dynamic.

## Pattern: Offset-Based Pagination with Clamped Limits

Every list query uses offset-based pagination with clamped page sizes. The `parsePageSize` and `parsePageOffset` helper functions extract query parameters, apply defaults, and enforce maximums:

```go
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
```

The store layer has a safety net — if the caller somehow passes an invalid limit, the store clamps it:

```go
func (s *Store) getFollowers(ctx context.Context, tenantID pgtype.UUID, entityType string, entityID pgtype.UUID, limit, offset int32) ([]gen.GetFollowersByEntityRow, error) {
    if limit <= 0 || limit > MaxPageSize {
        limit = DefaultPageSize
    }
    // ... query
}
```

This double-clamping (handler + store) prevents accidental large result sets from any code path that calls the store directly.

## Lesson: Entity Type Validation is Application-Layer, Not DB-Only

The database CHECK constraint ensures data integrity at rest, but it cannot produce user-facing error messages. When an API consumer sends an invalid `entity_type`, the CHECK constraint would produce a raw PostgreSQL error like `new row for relation "follows" violates check constraint`. The application-layer `isValidEntityType` check catches the error early and returns a meaningful JSON response:

```go
if !isValidEntityType(entityType) {
    return false, fmt.Errorf("invalid entity type: %s", entityType)
}
```

The handler converts this to a structured error:

```go
if err != nil {
    writeError(w, http.StatusBadRequest, "follow_error", err.Error())
    return
}
```

This pattern — application-layer validation with a DB constraint safety net — applies to all polymorphic tables in JapaConnect (follows, reactions, mentions, tags).

## Lesson: Follower Count Failures Are Non-Fatal

The follower count UPDATE runs after the follow/unfollow operation succeeds. If the count update fails (connection issue, table doesn't exist), the follow operation itself still succeeds. The warning is logged but does not propagate to the API consumer:

```go
if err := s.updateFollowerCount(ctx, entityType, entityID, delta); err != nil {
    fmt.Printf("warning: failed to update follower count for %s/%s: %v\n", entityType, entityID, err)
}
```

This is an intentional eventual-consistency tradeoff: the follower relationship is the source of truth, and the denormalized count is a cache. A reconciliation job (deferred to a future section) can rebuild counts from the follows table if needed.
