# Section 32 — Activity Feed / Timeline

**Status:** ✅ Locked

## Four Timelines

| Timeline | Who | Source |
|---|---|---|
| `explore` | Guests + logged-in | Trending/top content sitewide; pull-at-read |
| `primary` | Logged-in | Followed users, Spaces, Tags; hybrid push/pull |
| `space` | Logged-in, in a Space | Posts in that specific Space |
| `profile` | Anyone | All public posts by a specific user |

## Hybrid Fan-Out Strategy

```go
const FanOutThreshold = 5000 // members

func FanOutPost(ctx context.Context, post *Content) error {
    space, _ := getSpace(ctx, post.GeoSpaceID)
    if space.MemberCount <= FanOutThreshold {
        // Push: write a feeds row for every follower now (Asynq default tier)
        return enqueueFanOut(ctx, post.ID, "push")
    }
    // Pull: mark post in hot cache; followers pull it on next timeline request
    return enqueueFanOut(ctx, post.ID, "pull_index")
}

// feeds table (push side)
// CREATE TABLE feeds (
//   id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
//   tenant_id    uuid NOT NULL REFERENCES tenants(id),
//   user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
//   content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
//   space_id     uuid REFERENCES spaces(id),
//   content_type text NOT NULL,
//   score        numeric NOT NULL DEFAULT 0,   -- pre-computed for sort
//   created_at   timestamptz NOT NULL DEFAULT now(),
//   UNIQUE(user_id, content_id)
// );
// CREATE INDEX idx_feeds_user_created ON feeds (user_id, created_at DESC);
// CREATE INDEX idx_feeds_user_score   ON feeds (user_id, score DESC);
```

## Content Scoring (content_scores table)

```sql
CREATE TABLE content_scores (
  content_id          uuid PRIMARY KEY REFERENCES contents(id) ON DELETE CASCADE,
  vote_score          int NOT NULL DEFAULT 0,
  comment_count       int NOT NULL DEFAULT 0,
  view_count          int NOT NULL DEFAULT 0,
  bookmark_count      int NOT NULL DEFAULT 0,
  velocity_score      numeric NOT NULL DEFAULT 0,  -- Rising sort
  relevance_embedding vector(1536),                -- Relevant sort (pgvector)
  last_scored_at      timestamptz NOT NULL DEFAULT now()
);
```

## Four Sort Modes

```go
type FeedSort string
const (
  SortLatest   FeedSort = "latest"   // keyset cursor on created_at DESC
  SortTop      FeedSort = "top"      // vote_score DESC, 7-day rolling window
  SortRising   FeedSort = "rising"   // Hacker News formula
  SortRelevant FeedSort = "relevant" // pgvector cosine similarity
)

// Rising (velocity) formula — runs every 10 min via maintenance:update_velocity_scores task
func ComputeVelocityScore(engagement int, ageHours float64) float64 {
  gravity := 1.8
  return float64(engagement) / math.Pow(ageHours+2, gravity)
}

// Relevant sort — cosine similarity between post embedding and user interest embedding
// SELECT c.*, cs.relevance_embedding <=> $user_embedding AS dist
// FROM feeds f JOIN contents c ON c.id = f.content_id
// JOIN content_scores cs ON cs.content_id = c.id
// WHERE f.user_id = $user_id
// ORDER BY dist LIMIT 30;
```

## Onboarding Feed Bootstrap

New users have an empty `feeds` table. The onboarding wizard (Section 09) requires following a minimum number of users, Spaces, and Tags before the wizard completes — this populates feeds immediately. After that, the Explore timeline is the fallback until the primary feed has at least 20 items.

## Asynq Tasks

- `content:fan_out_to_feeds` (default tier) — pushes post to follower feeds
- `maintenance:update_velocity_scores` (low tier, every 10 min) — recalculates velocity for all posts < 7 days old
- `ai:generate_content_embedding` (default tier) — generates and stores `relevance_embedding` after publish

## See Also

Section 31 (follows drives fan-out), Section 13 (Meilisearch for Explore), Section 09 (onboarding follow minimums). ADR-031.
