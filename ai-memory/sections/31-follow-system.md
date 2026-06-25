# Section 31 — Follow System

**Status:** ✅ Locked

## Decided

Universal polymorphic follow system — users can follow other users, Spaces, Tags, Organizations, and Professional Profiles. A single `follows` table handles all entity types rather than three separate tables, keeping the fan-out logic unified.

```sql
CREATE TABLE follows (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  follower_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  entity_type     text NOT NULL,  -- 'user'|'space'|'tag'|'organization'|'professional_profile'
  entity_id       uuid NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(follower_id, entity_type, entity_id)
);
CREATE INDEX idx_follows_entity ON follows (entity_type, entity_id);
CREATE INDEX idx_follows_follower ON follows (follower_id, entity_type);

-- Denormalized follower counts (incremented/decremented via DB trigger)
-- follower_count on users, spaces, tags, organizations, professional_profiles
```

## Follow / Unfollow Handler

```go
func ToggleFollow(ctx context.Context, followerID uuid.UUID, entityType string, entityID uuid.UUID) (bool, error) {
    existing, err := queries.GetFollow(ctx, db.GetFollowParams{
        FollowerID: followerID, EntityType: entityType, EntityID: entityID,
    })
    if err == nil {
        // unfollow
        if err := queries.DeleteFollow(ctx, existing.ID); err != nil {
            return false, err
        }
        decrementFollowerCount(ctx, entityType, entityID)
        return false, nil
    }
    // follow
    if err := queries.InsertFollow(ctx, db.InsertFollowParams{
        FollowerID: followerID, EntityType: entityType, EntityID: entityID,
    }); err != nil {
        return false, err
    }
    incrementFollowerCount(ctx, entityType, entityID)
    // If following a space: auto-join if joining_perm == 'open'
    if entityType == "space" {
        tryAutoJoin(ctx, followerID, entityID)
    }
    return true, nil
}
```

## Mutual Follow (Connections)

Users can mutually follow each other. A "connection" is implicit: when `follows` has both `(A→B)` and `(B→A)`. No separate `connections` table. A helper query checks mutual status:

```sql
-- Is user B also following user A?
SELECT EXISTS (
  SELECT 1 FROM follows
  WHERE follower_id = $mutual_check_user_id
    AND entity_type = 'user'
    AND entity_id = $original_follower_id
) AS is_mutual;
```

DM request gating (Section 10) uses this query to determine whether a new conversation needs to go to "message requests."

## Follow Counts & Discovery

```go
// GET /api/v1/users/:id/followers  — paginated, cursor-based
// GET /api/v1/users/:id/following  — paginated, cursor-based
// GET /api/v1/spaces/:id/followers — paginated
// GET /api/v1/tags/:slug/followers — paginated
```

All list endpoints expose a `is_following` boolean for the authenticated user, resolved in a single bulk query rather than N+1 per item.

## Feed Integration

The Activity Feed (Section 32) uses `follows` as its primary interest signal: every new post by a followed user, in a followed Space, or tagged with a followed Tag gets fanned out to the follower's `feeds` row.

## See Also

Section 32 (feed fan-out from follows), Section 10 (DM request gating on mutual follow), Section 23 (discovery pages with follow/unfollow buttons). ADR-031.
