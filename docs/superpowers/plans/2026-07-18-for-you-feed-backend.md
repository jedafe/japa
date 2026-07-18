# Personalized "For You" Backend Feed — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. **Domain skill:** implement under `jc-backend-develop` (Spec-Readiness + Contract-Delivery gates, catalog maintenance).

**Goal:** Add `GET /api/v1/feed/for-you` — a personalized, ranked content feed powering the Home "For You" tab, with four sort modes, bounded query cost, and Valkey caching.

**Architecture:** Layered Go backend (Handlers → Services → Stores, sqlc/pgx). A two-stage bounded SQL query builds an *affinity ∪ velocity* candidate pool (~600 cap, 30-day window) with per-row affinity flags; a **pure-Go** function ranks it by the selected sort; the ranked result is cached in Valkey for 90s and paginated by slicing the cache. Same `FeedItem` response contract as every other feed.

**Tech Stack:** Go, chi router, pgx v5, sqlc, `github.com/valkey-io/valkey-go/valkeycompat`, golang-migrate, PostgreSQL 18.

**Spec:** `docs/superpowers/specs/2026-07-18-personalized-for-you-feed-design.md`

## Global Constraints

- **Rule 16:** every ranking signal is real backend data — no fabricated ordering, no mock rows.
- **Response contract is FROZEN:** the endpoint returns `{ "data": FeedItem[] }` using the **existing** `feed.FeedItem` struct (`internal/feed/service.go`). Do not add or rename `FeedItem` JSON fields.
- **Auth:** `GET /feed/for-you` requires auth (`auth.RequireAuth`) — 401 for guests.
- **Pagination params:** `page_size` (default 20, clamp `MaxPageSize`), `page_offset` (default 0) — reuse existing `parsePageSize` / `parsePageOffset`.
- **Sort param:** reuse existing `parseSort` → `relevant` (default) | `latest` | `top` | `rising`.
- **Weights (v1 constants):** `wVelocity=1.0, wAuthor=3.0, wSpace=2.5, wTag=1.5, wRecency=2.0, tagOverlapCap=3, halfLifeHours=48`.
- **Cache key:** `feed:foryou:{tenant_id}:{user_id}:{sort}`, TTL 90s, TTL-only invalidation.
- **Candidate bounds:** 30-day window; affinity LIMIT 300; velocity LIMIT 300; final candidate cap 600.
- **Gates before merge:** `make sqlc-gen` clean; `go build ./...` clean; `make lint` clean; `EXPLAIN ANALYZE` shows index scans (no seq-scan on `contents`); route catalog updated.
- **No AI attribution** in code comments.

---

### Task 1: Migration — supporting indexes

**Files:**
- Create: `migrations/000145_feed_for_you_indexes.up.sql`
- Create: `migrations/000145_feed_for_you_indexes.down.sql`

**Interfaces:**
- Consumes: existing tables `contents`, `content_scores`, `space_members`, `content_tags`.
- Produces: indexes that keep the candidate query off sequential scans. (No Go interface.)

- [ ] **Step 1: Write the up migration**

`migrations/000145_feed_for_you_indexes.up.sql`:
```sql
-- Supports the For-You candidate window (recent published/public content).
CREATE INDEX IF NOT EXISTS idx_contents_foryou_window
    ON contents (tenant_id, status, visibility, created_at DESC);

-- Supports the velocity candidate ordering.
CREATE INDEX IF NOT EXISTS idx_content_scores_velocity
    ON content_scores (velocity_score DESC);

-- Supports member-space affinity lookup by user.
CREATE INDEX IF NOT EXISTS idx_space_members_user
    ON space_members (tenant_id, user_id);

-- Supports tag-overlap EXISTS/JOIN by tag.
CREATE INDEX IF NOT EXISTS idx_content_tags_tag
    ON content_tags (tag_id);
```

Note: `follows (follower_id, entity_type, entity_id)` is already covered by the table's `UNIQUE(follower_id, entity_type, entity_id)` constraint — do **not** add a duplicate index.

- [ ] **Step 2: Write the down migration**

`migrations/000145_feed_for_you_indexes.down.sql`:
```sql
DROP INDEX IF EXISTS idx_content_tags_tag;
DROP INDEX IF EXISTS idx_space_members_user;
DROP INDEX IF EXISTS idx_content_scores_velocity;
DROP INDEX IF EXISTS idx_contents_foryou_window;
```

- [ ] **Step 3: Apply up, verify, roll back, re-apply**

Run (use the repo's migrate target/command; direct form shown):
```bash
migrate -database "$DATABASE_URL" -path migrations up 1
migrate -database "$DATABASE_URL" -path migrations down 1
migrate -database "$DATABASE_URL" -path migrations up 1
```
Expected: each command exits 0; `\d content_scores` in psql shows `idx_content_scores_velocity` after the final up.

- [ ] **Step 4: Commit**

```bash
git add migrations/000145_feed_for_you_indexes.up.sql migrations/000145_feed_for_you_indexes.down.sql
git commit -m "feat(feed): add indexes for For-You candidate query"
```

---

### Task 2: sqlc queries — affinity IDs + candidate pool

**Files:**
- Modify: `internal/db/queries/feeds.sql` (append new queries)
- Regenerate: `internal/db/gen/feeds.sql.go` (via `make sqlc-gen`)

**Interfaces:**
- Produces (generated Go, consumed by Task 4):
  - `gen.GetFollowedEntityIDsByType(ctx, gen.GetFollowedEntityIDsByTypeParams{TenantID, FollowerID, EntityType}) ([]pgtype.UUID, error)`
  - `gen.GetMemberSpaceIDs(ctx, gen.GetMemberSpaceIDsParams{TenantID, UserID}) ([]pgtype.UUID, error)`
  - `gen.GetForYouCandidates(ctx, gen.GetForYouCandidatesParams{...}) ([]gen.GetForYouCandidatesRow, error)` where the row has: `ID, Title, Slug, ContentType, AuthorID, SpaceID, CreatedAt, ViewCount, CommentCount, VoteScore, VelocityScore, BookmarkCount, AuthorName, AuthorAvatarUrl, Excerpt, ReadTimeMinutes, CoverStorageBucket, CoverStoragePath, Tags, IsAuthorFollowed, IsSpaceFollowed, FollowedTagOverlap`.

- [ ] **Step 1: Append the affinity-ID queries to `internal/db/queries/feeds.sql`**

```sql
-- name: GetFollowedEntityIDsByType :many
-- Lean ID-only lookup of everything a user follows of one entity type.
SELECT entity_id
FROM follows
WHERE tenant_id = @tenant_id
  AND follower_id = @follower_id
  AND entity_type = @entity_type;

-- name: GetMemberSpaceIDs :many
-- Space IDs the user is a member of (affinity signal, distinct from follows).
SELECT space_id
FROM space_members
WHERE tenant_id = @tenant_id
  AND user_id = @user_id;
```

- [ ] **Step 2: Append the candidate-pool query to `internal/db/queries/feeds.sql`**

```sql
-- name: GetForYouCandidates :many
-- Bounded personalized candidate pool = UNION of (a) recent content from
-- followed authors/spaces/tags and (b) top recent content by velocity.
-- Emits per-row affinity flags for Go-side ranking. Excludes own posts.
WITH affinity AS (
    SELECT c.id
    FROM contents c
    WHERE c.tenant_id = @tenant_id
      AND c.status = 'published'
      AND c.visibility = 'public'
      AND c.created_at >= @window_start
      AND c.author_id <> @exclude_author_id
      AND (
            c.author_id = ANY(@author_ids::uuid[])
         OR c.space_id  = ANY(@space_ids::uuid[])
         OR EXISTS (
              SELECT 1 FROM content_tags ct
              WHERE ct.content_id = c.id AND ct.tag_id = ANY(@tag_ids::uuid[])
         )
      )
    ORDER BY c.created_at DESC
    LIMIT @affinity_limit
),
velocity AS (
    SELECT c.id
    FROM contents c
    JOIN content_scores cs ON cs.content_id = c.id
    WHERE c.tenant_id = @tenant_id
      AND c.status = 'published'
      AND c.visibility = 'public'
      AND c.created_at >= @window_start
      AND c.author_id <> @exclude_author_id
    ORDER BY cs.velocity_score DESC
    LIMIT @velocity_limit
),
candidate_ids AS (
    SELECT id FROM affinity
    UNION
    SELECT id FROM velocity
)
SELECT c.id, c.title, c.slug, c.content_type, c.author_id, c.space_id,
       c.created_at, c.view_count, c.comment_count,
       cs.vote_score, cs.velocity_score, cs.bookmark_count,
       COALESCE(up.display_name, u.username, '') AS author_name,
       COALESCE(up.avatar_url, '') AS author_avatar_url,
       LEFT(c.body, 200) AS excerpt,
       c.read_time_minutes,
       m.storage_bucket AS cover_storage_bucket,
       m.storage_path   AS cover_storage_path,
       COALESCE(string_agg(t.name, ',' ORDER BY t.name), '') AS tags,
       (c.author_id = ANY(@author_ids::uuid[]))            AS is_author_followed,
       (c.space_id  = ANY(@space_ids::uuid[]))             AS is_space_followed,
       COUNT(DISTINCT t.id) FILTER (
           WHERE t.id = ANY(@tag_ids::uuid[])
       )::int                                              AS followed_tag_overlap
FROM contents c
JOIN candidate_ids ci ON ci.id = c.id
LEFT JOIN content_scores cs ON cs.content_id = c.id
LEFT JOIN users u  ON u.id = c.author_id
LEFT JOIN user_profiles up ON up.user_id = c.author_id
LEFT JOIN content_tags ct ON ct.content_id = c.id
LEFT JOIN tags t ON t.id = ct.tag_id
LEFT JOIN media m ON m.id = c.header_image_id AND m.deleted_at IS NULL
GROUP BY c.id, cs.vote_score, cs.velocity_score, cs.bookmark_count,
         u.username, up.display_name, up.avatar_url,
         m.storage_bucket, m.storage_path
LIMIT @candidate_cap;
```

- [ ] **Step 3: Regenerate sqlc**

Run: `make sqlc-gen`
Expected: exits 0; `internal/db/gen/feeds.sql.go` now contains `GetForYouCandidates`, `GetForYouCandidatesRow`, `GetForYouCandidatesParams`, `GetFollowedEntityIDsByType*`, `GetMemberSpaceIDs*`.

- [ ] **Step 4: Verify the generated types compile**

Run: `go build ./internal/db/gen/...`
Expected: PASS (no output).

- [ ] **Step 5: Verify the affinity-flag columns generated as `bool`/`int32`**

Run: `grep -nE "IsAuthorFollowed|IsSpaceFollowed|FollowedTagOverlap" internal/db/gen/feeds.sql.go`
Expected: `IsAuthorFollowed bool`, `IsSpaceFollowed bool`, `FollowedTagOverlap int32`. If sqlc emits `interface{}` for a flag, add an explicit cast in the SQL (`::bool`) and re-run Step 3.

- [ ] **Step 6: Commit**

```bash
git add internal/db/queries/feeds.sql internal/db/gen/feeds.sql.go
git commit -m "feat(feed): add For-You candidate + affinity sqlc queries"
```

---

### Task 3: Ranking core (pure Go, TDD)

**Files:**
- Create: `internal/feed/foryou.go`
- Test: `internal/feed/foryou_test.go`

**Interfaces:**
- Produces (consumed by Task 5):
  - `type Candidate struct { Item FeedItem; CreatedAt time.Time; VoteScore int32; VelocityScore float64; IsAuthorFollowed bool; IsSpaceFollowed bool; FollowedTagOverlap int }`
  - `func rankCandidates(cands []Candidate, sortMode string, now time.Time) []Candidate`
  - `func computeForYouScore(c Candidate, now time.Time, maxVelocity float64) float64`
  - Constants `wVelocity, wAuthor, wSpace, wTag, wRecency float64`, `tagOverlapCap int`, `halfLifeHours float64`.

- [ ] **Step 1: Write the failing test**

`internal/feed/foryou_test.go`:
```go
package feed

import (
	"testing"
	"time"
)

func TestComputeForYouScore_AffinityBoostsRank(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	base := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 10}

	followed := base
	followed.IsAuthorFollowed = true

	// Same velocity + recency; the followed-author candidate must score higher.
	if computeForYouScore(followed, now, 10) <= computeForYouScore(base, now, 10) {
		t.Fatal("followed-author candidate should outscore an identical non-followed one")
	}
}

func TestRecencyDecay_FresherWins(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	fresh := Candidate{CreatedAt: now.Add(-1 * time.Hour), VelocityScore: 5}
	old := Candidate{CreatedAt: now.Add(-240 * time.Hour), VelocityScore: 5}

	if computeForYouScore(fresh, now, 5) <= computeForYouScore(old, now, 5) {
		t.Fatal("fresher candidate should outscore an older one at equal velocity")
	}
}

func TestRankCandidates_SortModes(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	a := Candidate{Item: FeedItem{ID: "a"}, CreatedAt: now.Add(-1 * time.Hour), VoteScore: 1, VelocityScore: 9}
	b := Candidate{Item: FeedItem{ID: "b"}, CreatedAt: now.Add(-5 * time.Hour), VoteScore: 9, VelocityScore: 1}

	if got := rankCandidates([]Candidate{a, b}, SortLatest, now); got[0].Item.ID != "a" {
		t.Errorf("latest: want a first, got %s", got[0].Item.ID)
	}
	if got := rankCandidates([]Candidate{a, b}, SortTop, now); got[0].Item.ID != "b" {
		t.Errorf("top: want b first, got %s", got[0].Item.ID)
	}
	if got := rankCandidates([]Candidate{a, b}, SortRising, now); got[0].Item.ID != "a" {
		t.Errorf("rising: want a first, got %s", got[0].Item.ID)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/feed/ -run 'TestComputeForYouScore|TestRecencyDecay|TestRankCandidates' -v`
Expected: FAIL / build error — `computeForYouScore`, `rankCandidates`, `Candidate` undefined.

- [ ] **Step 3: Write the implementation**

`internal/feed/foryou.go`:
```go
package feed

import (
	"math"
	"sort"
	"time"
)

// For-You ranking weights (v1 constants; later tunable via system_settings).
const (
	wVelocity     = 1.0
	wAuthor       = 3.0
	wSpace        = 2.5
	wTag          = 1.5
	wRecency      = 2.0
	tagOverlapCap = 3
	halfLifeHours = 48.0
)

// Candidate is one scored row in the For-You candidate pool.
type Candidate struct {
	Item               FeedItem
	CreatedAt          time.Time
	VoteScore          int32
	VelocityScore      float64
	IsAuthorFollowed   bool
	IsSpaceFollowed    bool
	FollowedTagOverlap int
}

// recencyDecay returns an exponential decay in [0,1]; fresher content scores higher.
func recencyDecay(ageHours float64) float64 {
	if ageHours < 0 {
		ageHours = 0
	}

	return math.Exp(-ageHours / halfLifeHours)
}

// computeForYouScore blends the personalization signals into a single score.
// maxVelocity normalizes velocity relative to the fetched candidate pool.
func computeForYouScore(c Candidate, now time.Time, maxVelocity float64) float64 {
	velNorm := 0.0
	if maxVelocity > 0 {
		velNorm = c.VelocityScore / maxVelocity
	}

	author := 0.0
	if c.IsAuthorFollowed {
		author = 1.0
	}

	space := 0.0
	if c.IsSpaceFollowed {
		space = 1.0
	}

	overlap := c.FollowedTagOverlap
	if overlap > tagOverlapCap {
		overlap = tagOverlapCap
	}

	tag := float64(overlap) / float64(tagOverlapCap)
	rec := recencyDecay(now.Sub(c.CreatedAt).Hours())

	return wVelocity*velNorm + wAuthor*author + wSpace*space + wTag*tag + wRecency*rec
}

// rankCandidates orders the pool in place per sortMode and returns it.
func rankCandidates(cands []Candidate, sortMode string, now time.Time) []Candidate {
	maxVel := 0.0
	for i := range cands {
		if cands[i].VelocityScore > maxVel {
			maxVel = cands[i].VelocityScore
		}
	}

	sort.SliceStable(cands, func(i, j int) bool {
		switch sortMode {
		case SortLatest:
			return cands[i].CreatedAt.After(cands[j].CreatedAt)
		case SortTop:
			return cands[i].VoteScore > cands[j].VoteScore
		case SortRising:
			return cands[i].VelocityScore > cands[j].VelocityScore
		default: // SortRelevant
			return computeForYouScore(cands[i], now, maxVel) > computeForYouScore(cands[j], now, maxVel)
		}
	})

	return cands
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/feed/ -run 'TestComputeForYouScore|TestRecencyDecay|TestRankCandidates' -v`
Expected: PASS (all three).

- [ ] **Step 5: Commit**

```bash
git add internal/feed/foryou.go internal/feed/foryou_test.go
git commit -m "feat(feed): add For-You ranking core with pure-Go scoring"
```

---

### Task 4: Store methods

**Files:**
- Modify: `internal/feed/store.go` (append methods)

**Interfaces:**
- Consumes: `gen.GetForYouCandidates`, `gen.GetFollowedEntityIDsByType`, `gen.GetMemberSpaceIDs` (Task 2).
- Produces (consumed by Task 5):
  - `func (s *Store) getFollowedEntityIDs(ctx, tenantID, followerID pgtype.UUID, entityType string) ([]pgtype.UUID, error)`
  - `func (s *Store) getMemberSpaceIDs(ctx, tenantID, userID pgtype.UUID) ([]pgtype.UUID, error)`
  - `func (s *Store) getForYouCandidates(ctx, params gen.GetForYouCandidatesParams) ([]gen.GetForYouCandidatesRow, error)`

- [ ] **Step 1: Append the store methods to `internal/feed/store.go`**

```go
// getFollowedEntityIDs returns the IDs of all entities of one type the user follows.
func (s *Store) getFollowedEntityIDs(
	ctx context.Context, tenantID, followerID pgtype.UUID, entityType string,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetFollowedEntityIDsByType(ctx, gen.GetFollowedEntityIDsByTypeParams{
		TenantID:   tenantID,
		FollowerID: followerID,
		EntityType: entityType,
	})
	if err != nil {
		return nil, fmt.Errorf("getting followed %s ids: %w", entityType, err)
	}

	return ids, nil
}

// getMemberSpaceIDs returns the IDs of spaces the user is a member of.
func (s *Store) getMemberSpaceIDs(
	ctx context.Context, tenantID, userID pgtype.UUID,
) ([]pgtype.UUID, error) {
	ids, err := s.read.GetMemberSpaceIDs(ctx, gen.GetMemberSpaceIDsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		return nil, fmt.Errorf("getting member space ids: %w", err)
	}

	return ids, nil
}

// getForYouCandidates runs the bounded personalized candidate query.
func (s *Store) getForYouCandidates(
	ctx context.Context, params gen.GetForYouCandidatesParams,
) ([]gen.GetForYouCandidatesRow, error) {
	rows, err := s.read.GetForYouCandidates(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("getting for-you candidates: %w", err)
	}

	return rows, nil
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./internal/feed/...`
Expected: PASS (no output).

- [ ] **Step 3: Commit**

```bash
git add internal/feed/store.go
git commit -m "feat(feed): add store methods for For-You candidates and affinity"
```

---

### Task 5: Service assembly + cache

**Files:**
- Modify: `internal/feed/service.go` (add methods; change `NewService` signature)
- Modify: `internal/feed/foryou.go` (add cache-key + affinity helpers)
- Test: `internal/feed/foryou_test.go` (add cache-key + param tests)

**Interfaces:**
- Consumes: `rankCandidates`, `Candidate` (Task 3); `getForYouCandidates`, `getFollowedEntityIDs`, `getMemberSpaceIDs` (Task 4); `valkeycompat.Cmdable`.
- Produces (consumed by Task 6):
  - `func (s *Service) GetForYouFeed(ctx context.Context, tenantID, userID pgtype.UUID, sortMode string, limit, offset int32) ([]FeedItem, error)`
  - `func NewService(database *db.DB, vk valkeycompat.Cmdable) *Service` (**signature change**)
  - `func forYouCacheKey(tenantID, userID pgtype.UUID, sortMode string) string`

- [ ] **Step 1: Write the failing cache-key test**

Add to `internal/feed/foryou_test.go`:
```go
func TestForYouCacheKey_IncludesSort(t *testing.T) {
	t.Parallel()

	var tid, uid pgtype.UUID
	_ = tid.Scan("11111111-1111-1111-1111-111111111111")
	_ = uid.Scan("22222222-2222-2222-2222-222222222222")

	relevant := forYouCacheKey(tid, uid, SortRelevant)
	latest := forYouCacheKey(tid, uid, SortLatest)

	if relevant == latest {
		t.Fatal("cache key must differ per sort mode")
	}
	if want := "feed:foryou:11111111-1111-1111-1111-111111111111:22222222-2222-2222-2222-222222222222:relevant"; relevant != want {
		t.Errorf("key = %q, want %q", relevant, want)
	}
}
```
Add the import `"github.com/jackc/pgx/v5/pgtype"` to the test file if not present.

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/feed/ -run TestForYouCacheKey -v`
Expected: FAIL — `forYouCacheKey` undefined.

- [ ] **Step 3: Add the cache-key + affinity helpers to `internal/feed/foryou.go`**

Add these imports to `foryou.go`: `"fmt"`, `"github.com/jackc/pgx/v5/pgtype"`.
```go
// forYouCacheKey builds the per-user, per-sort Valkey key.
func forYouCacheKey(tenantID, userID pgtype.UUID, sortMode string) string {
	return fmt.Sprintf("feed:foryou:%s:%s:%s",
		uuidToString(tenantID), uuidToString(userID), sortMode)
}

// candidateFromRow maps a generated candidate row into a scored Candidate.
func candidateFromRow(r gen.GetForYouCandidatesRow) Candidate {
	return Candidate{
		Item: FeedItem{
			ID:                 uuidToString(r.ID),
			Title:              r.Title,
			Slug:               r.Slug,
			ContentType:        r.ContentType,
			AuthorID:           uuidToString(r.AuthorID),
			SpaceID:            nullableUUIDToString(r.SpaceID),
			CreatedAt:          r.CreatedAt.Time.String(),
			ViewCount:          r.ViewCount,
			CommentCount:       r.CommentCount,
			VoteScore:          r.VoteScore,
			VelocityScore:      pgNumericToFloat64(r.VelocityScore),
			AuthorName:         r.AuthorName,
			AuthorAvatarURL:    r.AuthorAvatarUrl,
			Excerpt:            r.Excerpt,
			Tags:               splitTags(r.Tags),
			ReadingTimeMinutes: int(r.ReadTimeMinutes),
			CoverImageURL:      coverImageURL(r.CoverStorageBucket, r.CoverStoragePath),
			BookmarkCount:      r.BookmarkCount,
		},
		CreatedAt:          r.CreatedAt.Time,
		VoteScore:          r.VoteScore,
		VelocityScore:      pgNumericToFloat64(r.VelocityScore),
		IsAuthorFollowed:   r.IsAuthorFollowed,
		IsSpaceFollowed:    r.IsSpaceFollowed,
		FollowedTagOverlap: int(r.FollowedTagOverlap),
	}
}
```
Add `"github.com/jedafe/japa/internal/db/gen"` to the `foryou.go` imports.

- [ ] **Step 4: Run the cache-key test to verify it passes**

Run: `go test ./internal/feed/ -run TestForYouCacheKey -v`
Expected: PASS.

- [ ] **Step 5: Change `NewService` to accept the Valkey client**

In `internal/feed/service.go`, add `vk valkeycompat.Cmdable` to the `Service` struct and constructor. Add import `"github.com/valkey-io/valkey-io/valkey-go/valkeycompat"` — **exact path:** `github.com/valkey-io/valkey-go/valkeycompat`. Also import `"time"` and `"github.com/jackc/pgx/v5/pgtype"` if not already present, and `"encoding/json"` (already imported).

Locate the existing struct/constructor (near the top of `service.go`) and update:
```go
type Service struct {
	store *Store
	vk    valkeycompat.Cmdable
}

func NewService(database *db.DB, vk valkeycompat.Cmdable) *Service {
	return &Service{
		store: NewStore(gen.New(database.Reader), gen.New(database.Writer)),
		vk:    vk,
	}
}
```
Note: match the **existing** `NewService` body for how the store is constructed (it may read `database.Reader`/`database.Writer` differently — preserve whatever the current constructor does, only adding the `vk` field). Import `"github.com/jedafe/japa/internal/db"` only if the current file already does.

- [ ] **Step 6: Add `GetForYouFeed`, `loadAffinity` to `internal/feed/service.go`**

```go
const (
	forYouWindowDays  = 30
	forYouAffinityCap = 300
	forYouVelocityCap = 300
	forYouCandidateCap = 600
	forYouCacheTTL    = 90 * time.Second
)

// loadAffinity fetches the user's followed authors, spaces (followed ∪ member),
// and followed tags as ID arrays for the candidate query.
func (s *Service) loadAffinity(
	ctx context.Context, tenantID, userID pgtype.UUID,
) (authorIDs, spaceIDs, tagIDs []pgtype.UUID, err error) {
	authorIDs, err = s.store.getFollowedEntityIDs(ctx, tenantID, userID, "user")
	if err != nil {
		return nil, nil, nil, err
	}

	followedSpaces, err := s.store.getFollowedEntityIDs(ctx, tenantID, userID, "space")
	if err != nil {
		return nil, nil, nil, err
	}

	memberSpaces, err := s.store.getMemberSpaceIDs(ctx, tenantID, userID)
	if err != nil {
		return nil, nil, nil, err
	}

	tagIDs, err = s.store.getFollowedEntityIDs(ctx, tenantID, userID, "tag")
	if err != nil {
		return nil, nil, nil, err
	}

	return authorIDs, dedupeUUIDs(append(followedSpaces, memberSpaces...)), tagIDs, nil
}

// GetForYouFeed returns the personalized, ranked feed for a user, cached 90s per sort.
func (s *Service) GetForYouFeed(
	ctx context.Context, tenantID, userID pgtype.UUID, sortMode string, limit, offset int32,
) ([]FeedItem, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	if offset < 0 {
		offset = 0
	}

	key := forYouCacheKey(tenantID, userID, sortMode)

	// 1. Cache hit → slice and return.
	if s.vk != nil {
		if data, err := s.vk.Get(ctx, key).Bytes(); err == nil && len(data) > 0 {
			var ranked []FeedItem
			if json.Unmarshal(data, &ranked) == nil {
				return pageSlice(ranked, offset, limit), nil
			}
		}
	}

	// 2. Miss → load affinity, fetch candidates.
	authorIDs, spaceIDs, tagIDs, err := s.loadAffinity(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	var windowStart pgtype.Timestamptz
	_ = windowStart.Scan(time.Now().Add(-forYouWindowDays * 24 * time.Hour))

	rows, err := s.store.getForYouCandidates(ctx, gen.GetForYouCandidatesParams{
		TenantID:        tenantID,
		WindowStart:     windowStart,
		ExcludeAuthorID: userID,
		AuthorIds:       authorIDs,
		SpaceIds:        spaceIDs,
		TagIds:          tagIDs,
		AffinityLimit:   forYouAffinityCap,
		VelocityLimit:   forYouVelocityCap,
		CandidateCap:    forYouCandidateCap,
	})
	if err != nil {
		return nil, err
	}

	// 3. Score + rank in pure Go.
	cands := make([]Candidate, len(rows))
	for i, r := range rows {
		cands[i] = candidateFromRow(r)
	}

	ranked := make([]FeedItem, 0, len(cands))
	for _, c := range rankCandidates(cands, sortMode, time.Now()) {
		ranked = append(ranked, c.Item)
	}

	// 4. Cache the ranked list (best-effort).
	if s.vk != nil {
		if data, err := json.Marshal(ranked); err == nil {
			_ = s.vk.Set(ctx, key, data, forYouCacheTTL).Err()
		}
	}

	return pageSlice(ranked, offset, limit), nil
}
```
Note the sqlc param field names (`AuthorIds`, `SpaceIds`, `TagIds`, `WindowStart`, `ExcludeAuthorID`, `AffinityLimit`, `VelocityLimit`, `CandidateCap`) — **confirm** them against the generated `gen.GetForYouCandidatesParams` after Task 2 and adjust casing if sqlc differs (e.g. `AuthorIDs`).

- [ ] **Step 7: Add the `pageSlice` and `dedupeUUIDs` helpers to `internal/feed/foryou.go`**

```go
// pageSlice returns a bounded page of items; out-of-range offsets yield empty.
func pageSlice(items []FeedItem, offset, limit int32) []FeedItem {
	if offset >= int32(len(items)) {
		return []FeedItem{}
	}

	end := offset + limit
	if end > int32(len(items)) {
		end = int32(len(items))
	}

	return items[offset:end]
}

// dedupeUUIDs removes duplicate UUIDs while preserving order.
func dedupeUUIDs(in []pgtype.UUID) []pgtype.UUID {
	seen := make(map[string]struct{}, len(in))
	out := make([]pgtype.UUID, 0, len(in))

	for _, id := range in {
		k := uuidToString(id)
		if _, ok := seen[k]; ok {
			continue
		}

		seen[k] = struct{}{}
		out = append(out, id)
	}

	return out
}
```

- [ ] **Step 8: Add pure tests for `pageSlice` and `dedupeUUIDs`**

Add to `internal/feed/foryou_test.go`:
```go
func TestPageSlice_Bounds(t *testing.T) {
	t.Parallel()

	items := []FeedItem{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	if got := pageSlice(items, 1, 1); len(got) != 1 || got[0].ID != "b" {
		t.Errorf("offset 1 limit 1: got %+v", got)
	}
	if got := pageSlice(items, 5, 10); len(got) != 0 {
		t.Errorf("offset past end must be empty, got %+v", got)
	}
	if got := pageSlice(items, 2, 10); len(got) != 1 || got[0].ID != "c" {
		t.Errorf("limit past end must clamp, got %+v", got)
	}
}
```

- [ ] **Step 9: Update the one existing `NewService` call-site so the build passes**

In `internal/server/server.go` (the line `feedService := feed.NewService(database)`), change to:
```go
feedService := feed.NewService(database, vkClient.Redis())
```

- [ ] **Step 10: Build + run all feed tests**

Run: `go build ./... && go test ./internal/feed/ -v`
Expected: build PASS; all feed tests PASS (existing + new).

- [ ] **Step 11: Commit**

```bash
git add internal/feed/service.go internal/feed/foryou.go internal/feed/foryou_test.go internal/server/server.go
git commit -m "feat(feed): assemble For-You service with affinity, ranking, and Valkey cache"
```

---

### Task 6: Handler, route, catalog, and end-to-end verification

**Files:**
- Modify: `internal/feed/handlers.go` (add handler)
- Modify: `internal/server/server.go` (register route)
- Modify: the route catalog file (see Step 4)

**Interfaces:**
- Consumes: `Service.GetForYouFeed` (Task 5); existing `parsePageSize`, `parsePageOffset`, `parseSort`, `requireTenant`, `optionalUser`, `writeJSON`, `writeError`.
- Produces: `GET /api/v1/feed/for-you` HTTP endpoint.

- [ ] **Step 1: Add the handler to `internal/feed/handlers.go`**

```go
// GetForYouFeed handles GET /api/v1/feed/for-you — personalized ranked feed.
func (h *Handlers) GetForYouFeed(w http.ResponseWriter, r *http.Request) {
	user := h.optionalUser(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	sortMode := parseSort(r)

	items, err := h.service.GetForYouFeed(r.Context(), tenantID, user.UserID, sortMode, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to get For-You feed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}
```

- [ ] **Step 2: Register the route in `internal/server/server.go`**

Directly after the existing `r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed", feedHandlers.GetPrimaryFeed)` line, add:
```go
r.With(auth.RequireAuth(kr, vkClient.Redis())).Get("/feed/for-you", feedHandlers.GetForYouFeed)
```

- [ ] **Step 3: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Update the route catalog**

Run: `grep -rl '"/feed"' --include=*.md --include=*.json . | grep -i catalog | head` to locate the catalog. Add a row/entry for `GET /api/v1/feed/for-you` mirroring the existing `/feed` entry (auth required, returns `FeedItem[]`). If the catalog is JEOS-managed (`.jeos/`), update the same file the `/feed` entry lives in.

- [ ] **Step 5: Lint**

Run: `make lint`
Expected: PASS (no new findings in `internal/feed`).

- [ ] **Step 6: End-to-end smoke — 401 for guest**

Run:
```bash
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/api/v1/feed/for-you
```
Expected: `401`.

- [ ] **Step 7: End-to-end smoke — 200 authed, all four sorts**

With a valid session token in `$TOKEN` and tenant host header:
```bash
for s in relevant latest top rising; do
  printf "sort=%s -> " "$s"
  curl -s -H "Authorization: Bearer $TOKEN" -H "Host: <tenant-host>" \
    "http://localhost:8080/api/v1/feed/for-you?sort=$s&page_size=5" \
    | head -c 120; echo
done
```
Expected: each returns `{"data":[...]}` with up to 5 items; the requesting user's own posts do not appear.

- [ ] **Step 8: EXPLAIN gate**

In psql, run `EXPLAIN ANALYZE` on the candidate query with representative array params. Expected: index scans on `contents` (`idx_contents_foryou_window`) and `content_scores` (`idx_content_scores_velocity`); **no** `Seq Scan on contents`. If a seq-scan appears, re-check the window/status/visibility predicates against the new index column order.

- [ ] **Step 9: Commit**

```bash
git add internal/feed/handlers.go internal/server/server.go
git commit -m "feat(feed): expose GET /feed/for-you route and handler"
```

---

## Self-Review

**Spec coverage:**
- §2 endpoint/contract → Tasks 5–6 (route, auth, params, `FeedItem` reuse). ✓
- §3 ranking model + §3.1 sorts → Task 3 (`computeForYouScore`, `rankCandidates` sort modes). ✓
- §4 architecture (bounded SQL + Go ranking + cache + `NewService` change) → Tasks 2, 4, 5. ✓
- §5 indexes/migration → Task 1. ✓
- §6 caching (key incl. sort, 90s, TTL-only) → Task 5 Steps 3, 6. ✓
- §7 testing + EXPLAIN + catalog → Tasks 3/5 (pure tests), Task 6 (EXPLAIN, catalog). ✓
- §8 B upgrade path → documented in spec, no task (deliberate). ✓
- §9 out of scope → nothing built here. ✓

**Placeholder scan:** No TBD/TODO; every code step shows complete code. Two explicit **verify-and-adjust** points (sqlc field casing in Task 2 Step 5 / Task 5 Step 6; catalog file location in Task 6 Step 4) are verification steps with concrete commands, not placeholders.

**Type consistency:** `Candidate`, `computeForYouScore(c, now, maxVelocity)`, `rankCandidates(cands, sortMode, now)`, `forYouCacheKey(tenant, user, sort)`, `pageSlice(items, offset, limit)`, `dedupeUUIDs`, `GetForYouFeed(ctx, tenant, user, sortMode, limit, offset)` are named identically across the tasks that define and consume them. `NewService(database, vk)` signature change is applied in Task 5 and its call-site fixed in the same task (Step 9) so the build never breaks across a task boundary.
