# Section 32 — Activity Feed / Timeline

**Status:** ✅ Locked (updated session 5 — multi-space fan-out, homepage routing clarified, sticky filter bar)

## Four Timelines

| Timeline | Label | Who | Source |
|---|---|---|---|
| `explore` | Explore | Guests (logged-out) | Trending/top content sitewide; pull-at-read |
| `primary` | Home / Primary | Logged-in users | Followed users, Spaces, Tags; hybrid push/pull |
| `space` | Space Feed | Logged-in, viewing a Space | Posts assigned to that specific Space |
| `profile` | User Timeline | Anyone | All public posts by a specific user |

**Homepage routing:** Authenticated users land on the `primary` timeline. Unauthenticated users land on the `explore` timeline. This is enforced at the SvelteKit root layout — no redirect; the correct layout is server-rendered directly.

## Sticky Sort Filter Bar

All four timelines expose a four-mode sort selector that is **sticky on scroll** — it stays pinned at the top of the viewport as the user scrolls through the feed. The active sort mode is persisted per-timeline in the user's session store (not server-side preferences — per-tab state is fine here).

```
[ Relevant ]  [ Latest ]  [ Popular ]  [ Rising ]   ← sticky on scroll, pinned below header
```

## Hybrid Fan-Out Strategy

```go
const FanOutThreshold = 5000 // members

// A post can be assigned to multiple spaces (content_spaces table, Section 14).
// Fan-out fires once per assigned space, plus once for the author's followers.

func FanOutPost(ctx context.Context, post *Content) error {
    // 1. Fan out to author's followers
    enqueueTask(ctx, "content:fan_out_author_followers", FanOutPayload{
        ContentID: post.ID,
        AuthorID:  post.AuthorUserID,
    })

    // 2. Fan out to each assigned space
    spaces, _ := queries.GetContentSpaces(ctx, post.ID) // from content_spaces
    for _, space := range spaces {
        if space.MemberCount <= FanOutThreshold {
            // Push: write a feeds row for every member now (Asynq default tier)
            enqueueTask(ctx, "content:fan_out_space", FanOutPayload{
                ContentID: post.ID,
                SpaceID:   space.ID,
                Strategy:  "push",
            })
        } else {
            // Pull: mark post in hot cache; members pull it on next timeline request
            enqueueTask(ctx, "content:fan_out_space", FanOutPayload{
                ContentID: post.ID,
                SpaceID:   space.ID,
                Strategy:  "pull_index",
            })
        }

        // 3. Post bubbling: fan out to parent primary spaces (up to spaces.bubble_depth levels)
        if space.Level == "custom" || space.Level == "city" {
            enqueueTask(ctx, "content:fan_out_bubble", FanOutPayload{
                ContentID: post.ID,
                SpaceID:   space.ID,
                Label:     "from " + space.Name,
            })
        }
    }
    return nil
}
```

```sql
-- feeds table (push side)
CREATE TABLE feeds (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  content_id   uuid NOT NULL REFERENCES contents(id) ON DELETE CASCADE,
  space_id     uuid REFERENCES spaces(id),   -- NULL = came from author follow, not space
  content_type text NOT NULL,
  score        numeric NOT NULL DEFAULT 0,   -- pre-computed for sort
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, content_id)
);
CREATE INDEX idx_feeds_user_created ON feeds (user_id, created_at DESC);
CREATE INDEX idx_feeds_user_score   ON feeds (user_id, score DESC);
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

- `content:fan_out_author_followers` (default tier) — pushes post to all followers of the author
- `content:fan_out_space` (default tier) — pushes post to members of a single assigned space
- `content:fan_out_bubble` (low tier) — bubbles post up to parent primary spaces with "from [Space]" label
- `maintenance:update_velocity_scores` (low tier, every 10 min) — recalculates velocity for posts < 7 days old
- `maintenance:prune_old_feeds` (low tier, weekly) — prunes feeds rows older than configured retention
- `ai:generate_content_embedding` (default tier) — generates and stores `relevance_embedding` after publish

## See Also

Section 31 (follows drives author fan-out), Section 05 (space hierarchy, bubbling), Section 13 (Meilisearch for Explore), Section 09 (onboarding follow minimums), Section 14 (content_spaces — multiple spaces per post). ADR-031, ADR-032, ADR-049.
