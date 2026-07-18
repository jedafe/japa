# Design — Personalized "For You" Feed

**Date:** 2026-07-18
**Surface:** JapaConnect Go backend (`github.com/jedafe/japa`) → consumed by Main App (`apps/main`)
**Status:** APPROVED (2026-07-18) — sort options on For-You confirmed; awaiting final spec review
**Roadmap context:** ROADMAP-MAIN Phase 04 (Home Timeline). Continuation of the main-app rebuild (ADR-058 shell).

---

## 0. How we got here (decisions already made)

| # | Decision | Choice |
|---|----------|--------|
| D1 | Home tab semantics — backend has no Following/For-You split today | Add a **real** personalized For-You feed (not fake tabs) |
| D2 | Following vs For You mapping | **Following** = existing `/feed` (fan-out); **For You** = NEW `/feed/for-you`; guest = existing `/explore` |
| D3 | Sequence | **Backend first**, then frontend rendering layer |
| D4 | Ranking strategy | **A — query-time weighted hybrid, performance-hardened** |
| D5 | Scale path | **B (precompute) documented, not built** — triggered later by metrics, same endpoint |

**Rule 16 clean:** every ranking signal is real backend data. No mocks, no fabricated ordering.

---

## 1. Scope & decomposition

Two sequenced sub-projects. **This spec is sub-project #1 only.**

- **#1 (this spec) — Backend:** `GET /api/v1/feed/for-you`, a personalized ranked content feed. Built under `jc-backend-develop` (Spec-Readiness + Contract-Delivery gates, catalog maintenance).
- **#2 (next spec) — Frontend:** `FeedCard` / `FeedList` / `FeedSortBar` + Home tab wiring:
  - **Following** tab → `GET /feed`
  - **For You** tab → `GET /feed/for-you`
  - guest home → `GET /explore`
  - Also fixes the live bugs the API-Readiness Gate found: `?limit=` is ignored (must be `?page_size=`/`?page_offset=`); "Popular" label must map to backend `top`; `created_at` is non-ISO (`"2026-07-12 08:03:55.888616 +0100 WAT"`) and needs safe parsing.

---

## 2. Endpoint & contract

- **Route:** `GET /api/v1/feed/for-you`
- **Auth:** `auth.RequireAuth` — personalized, so login required (401 otherwise, matching `/feed`).
- **Query params:**
  - `page_size` — default 20, clamped to `MaxPageSize` (existing `parsePageSize`).
  - `page_offset` — default 0 (existing `parsePageOffset`).
  - `sort` — **honored** (existing `parseSort`): `relevant` (default = personalized blend), `latest`, `top`, `rising`. All four reorder the **same personalized candidate pool** (§3/§4), so switching sort keeps you in *your* feed rather than falling through to global Explore.
- **Response:** `{ "data": FeedItem[] }` — the **exact same `FeedItem` struct** every other feed returns. No new response shape. This identical contract is what lets us swap A→B later with zero frontend change.

`FeedItem` fields (authoritative, from `internal/feed/service.go`):
`id, title, slug, content_type, author_id, space_id?, created_at, view_count, comment_count, vote_score, velocity_score, author_name?, author_avatar_url?, excerpt?, tags?[], reading_time_minutes?, cover_image_url?, bookmark_count?`

---

## 3. Ranking model

Per-candidate score, computed from real signals only:

```
score(item) =
    W_VELOCITY * norm(velocity_score)        // global quality (existing worker output)
  + W_AUTHOR   * (author ∈ followed_users)    // 0 or 1
  + W_SPACE    * (space ∈ followed_or_member)  // 0 or 1
  + W_TAG      * min(followed_tag_overlap, CAP)// # of the item's tags you follow, capped
  + W_RECENCY  * recency_decay(created_at)     // exponential decay, fresher scores higher
```

**Signals & their sources**
- `velocity_score` — `content_scores.velocity_score` (already computed by the velocity worker).
- Author affinity — `follows` where `entity_type='user'`, `follower_id = me`.
- Space affinity — `follows` where `entity_type='space'` **∪** `space_members` where `user_id = me`.
- Tag affinity — `follows` where `entity_type='tag'`, intersected with the item's `content_tags`.
- Recency — `contents.created_at`.

**Weights (v1):** named Go constants, e.g. `W_VELOCITY=1.0, W_AUTHOR=3.0, W_SPACE=2.5, W_TAG=1.5, W_RECENCY=2.0`, `TAG_OVERLAP_CAP=3`. Tunable later via `system_settings` with no schema change.

**Normalization (removes ambiguity):**
- `norm(velocity_score)` = **pool-relative**: `velocity_score / max(velocity_score across the fetched candidate pool)` (guard divide-by-zero → 0). Keeps the term in `[0,1]` so weights are directly comparable, and avoids depending on an absolute velocity scale that drifts over time.
- `recency_decay(created_at)` = `exp(-age_hours / HALF_LIFE_HOURS)` with `HALF_LIFE_HOURS ≈ 48` (constant, tunable). Also `[0,1]`.
- Affinity terms are already `0/1` (author, space) or `0..CAP` (tag); the tag term is divided by `TAG_OVERLAP_CAP` to land in `[0,1]` before weighting.

**Exclusions:** the requesting user's own posts; non-`published`; non-`public`.

**Known v1 limitation:** no "seen"/impression table exists, so no seen-dedupe. Documented; a future enhancement.

### 3.1 Sort behavior (all four honored)

The default (`relevant`) is the personalized blend above. The other three re-order the **same personalized candidate pool** (§4 stage 1) by a raw column — they do **not** become the global Explore feed, so personalization (the affinity-inclusive pool) is preserved across every sort:

| Frontend chip | `sort` param | Ordering applied in stage 2 (Go) |
|---|---|---|
| Relevant | `relevant` (default) | `computeForYouScore` blend, DESC |
| Latest | `latest` | `created_at` DESC |
| Popular | `top` | `vote_score` DESC |
| Rising | `rising` | `velocity_score` DESC |

Note the frontend label→param mapping: **"Popular" → `top`** (the API-Gate mismatch), handled in sub-project #2.

---

## 4. Architecture — "A-hardened" (bounded SQL + testable Go)

Refined by the repo's pure-function test convention: keep candidate retrieval bounded in SQL, keep the final blend in unit-testable Go.

```
Handler  GetForYouFeed  (internal/feed/handlers.go)
  └─ Service.GetForYouFeed(ctx, tenant, userID, sort, page_size, page_offset)
       ├─ 1. Valkey GET feed:foryou:{tenant}:{user}:{sort} → cached ranked []contentID?
       │        HIT  → slice[offset:offset+limit] → hydrate → return
       └─ MISS:
          ├─ 2. loadAffinity(user): 3 tiny indexed lookups →
          │        followed authorIDs, spaceIDs (followed ∪ member), tagIDs  (as arrays)
          ├─ 3. Store.getForYouCandidates(tenant, affinityArrays, ~30d window)
          │        ONE sqlc query, candidate pool = UNION of:
          │          (a) AFFINITY pool: recent content from followed authors OR
          │              followed/member spaces OR followed tags   (ORDER created_at, LIMIT ~300)
          │          (b) VELOCITY pool: recent content ORDER velocity_score DESC (LIMIT ~300)
          │        hydrated with per-row affinity flags + raw sort columns
          │        (is_author_followed, is_space_followed, followed_tag_overlap,
          │         created_at, vote_score, velocity_score)   — capped ≤ ~600 rows
          ├─ 4. rank in PURE Go over ≤600 rows by `sort`:               ← unit-tested
          │        relevant → computeForYouScore blend DESC
          │        latest   → created_at DESC
          │        top      → vote_score DESC
          │        rising   → velocity_score DESC
          ├─ 5. Valkey SET ranked []contentID under :{sort} key, TTL 90s
          └─ 6. slice page → hydrate FeedItem[] → return
```

**Why this shape**
- DB work happens **at most once per 90s per (user, sort)** (step 3) and the pool is **hard-capped (~600 rows)** — cost stays flat as total content grows.
- The pool is **affinity-inclusive**, so every sort reorders *your* content (not global) — this is what makes the sort chips honest (§3.1).
- Pagination is a **cache slice**, not a re-query. **Depth bound:** the ranked list is the capped pool (~600 IDs), so requests past that offset return an empty page (feed end) — an acceptable v1 ceiling for a personalized feed; the next 90s window refreshes candidates. Recorded as a known bound, not a bug.
- Affinity is **array membership** (`= ANY($ids)`), not correlated subqueries — 2–3 tiny indexed lookups per miss.
- Final ranking is a **pure Go function** → fits the existing `feed_test.go` pure/table-driven/`t.Parallel()` style; each sort mode is a unit-tested ordering.

**Layer changes (existing `internal/feed` module):**
- `internal/db/queries/feeds.sql` — new `GetForYouCandidates` query (+ affinity lookups if not reusing existing follow queries). `sqlc generate`.
- `store.go` — `getForYouCandidates(...)`.
- `service.go` — `GetForYouFeed(...)`, `loadAffinity(...)`, pure `computeForYouScore(...)`, cache read/write. **`NewService(database)` → `NewService(database, vkClient.Redis())`** (follows `config.NewSettings` / `rbac.New` precedent).
- `handlers.go` — `GetForYouFeed`.
- `server.go` — register `r.With(auth.RequireAuth(...)).Get("/feed/for-you", feedHandlers.GetForYouFeed)`; update the one `feed.NewService` call-site.

---

## 5. Indexes & migration

Paired up/down migration adding only indexes the plan actually needs (verify with EXPLAIN first; do not duplicate what a UNIQUE constraint already provides):
- `contents (tenant_id, status, visibility, created_at DESC)` — candidate window
- `content_scores (velocity_score DESC)` — stage-1 ordering
- `follows (follower_id, entity_type, entity_id)` — likely already covered by the existing `UNIQUE(follower_id, entity_type, entity_id)` → **verify, reuse**
- `space_members (user_id)`
- `content_tags (content_id)` and `(tag_id)`

---

## 6. Caching

- **Key:** `feed:foryou:{tenant_id}:{user_id}:{sort}` → JSON array of ranked content IDs. `sort` is part of the key since each mode yields a different order (≤4 keys per user).
- **TTL:** 90s.
- **Invalidation:** TTL-only in v1 (a new follow/post self-heals within 90s). Deliberate simplification.

---

## 7. Testing & gates

- **Pure unit tests** (mirror `feed_test.go`): `computeForYouScore` weight + decay behavior; weight constants; **each sort ordering** (`relevant`/`latest`/`top`/`rising`) produces the expected order over a fixed candidate slice; cache-key builder (incl. `:{sort}`); affinity-array assembly; param clamping.
- **`EXPLAIN ANALYZE` gate** on seed data before merge — assert index usage + bounded latency on the UNION candidate query.
- **Route catalog** updated (backend skill requirement).
- **Acceptance:** `GET /feed/for-you` 401 unauth / 200 authed returning ranked `FeedItem[]`; all four `sort` values return 200 with the correct ordering over the personalized pool; own posts excluded; cache hit on repeat within TTL (per-sort key); EXPLAIN shows index scans, no seq-scan on `contents`.

---

## 8. Scale path — B (precompute), documented not built

When instrumentation (query p95, cache hit-rate, candidate-window growth) crosses threshold: replace steps 2–4 with a worker that materializes a `for_you_feeds` table per **active** user (mirrors the existing fan-out `feeds` pattern). **Same endpoint, same `FeedItem[]` contract, zero frontend change.** Trade-off recorded: B optimizes reads but grows write/storage cost with users × content and introduces a staleness window — hence deferred until metrics justify it.

---

## 9. Out of scope (explicit)

- Sub-project #2 (frontend rendering layer) — separate spec.
- Interactive engagement (vote/react/bookmark mutations) on cards — separate unit.
- Seen/impression dedupe; semantic (vector) personalization using `content_scores.relevance_embedding` (no user-interest vector exists yet).
- Ads, WebSocket "N new posts", right-sidebar widgets.
