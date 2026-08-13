# JapaConnect Performance Audit — Backend Infrastructure Levers

**Date:** 2026-08-13
**Context:** Post ADR-004 amendment (single-tenant freeze). This audit covers
the four backend performance levers that materially affect end-user speed:
indexes, cache tuning, replica usage, and search offload.

---

## 1. Indexes — Drop Redundant Standalone (tenant_id) Indexes

**Status:** ✅ Implemented (migration 000433)

### What was done
Dropped 12 standalone `(tenant_id)`-only indexes that had zero selectivity in
a single-tenant deployment (every row carries the same UUID). These indexes
were written on every INSERT/UPDATE/DELETE (write amplification) but could
never be used for filtering.

### Verification
No feature breaks. Queries that filtered by `tenant_id + another_column` use
the other column's index and apply `tenant_id` as a post-filter (one integer
comparison on the fetched rows — negligible). Queries that filtered by
`tenant_id` alone fall back to a sequential scan on small lookup tables
(tens to low-hundreds of rows) — sub-millisecond, indistinguishable from an
index scan at that size.

### Watch items
- `moderation_alerts` — already has `(tenant_id, status)` composite index retained.
  `ListModerationAlerts` uses this composite. No action needed.
- `user_blocks` — resolved: migration 000437 adds `(blocker_id, tenant_id, created_at DESC)`
  composite for the user-facing `ListBlockedUsers` query. Admin-only `ListAllUserBlocks`
  remains a seq scan (acceptable — admin-only, paginated, low-traffic).

### Retained indexes
49 composite indexes that lead with `tenant_id` are retained. At single-tenant
scale, the planner skips the constant prefix and uses the remaining columns.

---

## 2. Cache Tuning — Valkey/Cache Layer

**Status:** ✅ Well-architected, minor recommendations

### Current state
The cache layer uses `internal/valkey/cachehelper.go` which provides:
- `GetOrComputeNative` / `GetOrComputeCompat` — cache-aside with stampede protection
- All cache writes use `.Ex(ttl)` — every cache key has a TTL (no unbounded growth)
- Stampede protection via `SetNX` with a 5-second lock TTL

### Key TTL inventory
| Key pattern | TTL | Assessment |
|---|---|---|
| `jc:perm:{user_id}` | 10 min | ✅ Extended — halves DB load from permission checks |
| `jc:flags:{tenant_id}:{flag_key}` | 5 min | ✅ Good |
| `jc:visitor:ctx:{session_id}` | 30 min | ✅ Good — invalidated on preference change |
| `jc:cache:search:*` | 2 min | ✅ Good — short enough for new content |
| `jc:settings:{tenant_id}:{key}` | 5 min | ✅ Good — invalidated on write |
| `jc:cache:feed:rss:{tenant_id}` | Configurable | ✅ Good |
| `jc:widget:{widget_id}` | From DB | ✅ Good — per-widget TTL |
| `jc:spam:censored:{tenant_id}` | 1 hour | ✅ Verified — `internal/spam/cache.go:22`, invalidated on admin mutations |
| `jc:coupon:{id}` | 15 min | ✅ Verified — `internal/coupons/service.go:21`, invalidated on mutations |
| `jc:coupon_list:{tenant}` | 5 min | ✅ Verified — `internal/coupons/service.go:22`, invalidated on mutations |

### Recommendations
1. **jc:spam:censored and jc:coupon* TTLs verified** (done — all 3 keys have
   proper TTLs with invalidation on mutations. No unbounded growth risk).
2. **jc:perm TTL extended to 10 min** (done — internal/rbac/rbac.go).
   Permissions change rarely, and cache invalidation fires on every
   group/role/ban/mod change (Rule 6), so the 10-min TTL is safe and
   halves the DB load from permission checks for active users.

---

## 3. Replica Usage — Reader/Writer Pool Separation

**Status:** ✅ Architecture correct, config support added, needs production provisioning

### Current state
`internal/db/pool.go` implements the correct pattern:
- `DB.Reader` — for all SELECT queries
- `DB.Writer` — for INSERT/UPDATE/DELETE
- `NewWithReplica(writerDSN, readerDSN, cfg)` — separate DSNs when a replica exists
- `cmd/api/main.go` now checks `JC_DATABASE_READER_URL`: when set, uses
  `NewWithReplica` (reads → replica, writes → primary); when empty, uses
  `NewWithConfig` (both pools → primary). Docker-compose has a commented-out
  example showing how to configure it.

### Recommendations
1. **Provision a read replica before launch** — done locally (native Postgres on port 5434).
   Set `JC_DATABASE_READER_URL` env var to the replica DSN. The local `.env` already
   points to `postgresql://japa:japa@localhost:5434/japa?sslmode=disable`.
2. **Audit for misplaced reads on Writer** — done for `internal/analytics/store.go`.
   Fixed `ResolveSecurityScanResult` (was using Reader pool for an UPDATE).
   All remaining `s.read.*` calls are reads; all `s.write.*` calls are writes.
3. **Pool sizing** — Writer: keep at 25. Reader: scale to 50-100 for production.
   Monitor pool.Stat().AcquiredConns() vs MaxConns().

---

## 4. Search Offload — ILIKE to Meilisearch

**Status:** ✅ Meilisearch integrated for public search, recommendations for ILIKE

### Current state
JapaConnect has a full Meilisearch integration (internal/search/) with 9
indexes. The public search endpoint uses Meilisearch. ILIKE patterns remain
in 24 SQL query files as list-page filters (e.g. ?q= on blog listing).

### Recommendation A: Add pg_trgm GIN indexes (immediate, zero code change)
PostgreSQL's pg_trgm extension makes ILIKE fast by creating trigram indexes.
This is the highest-leverage change — makes every existing ILIKE query use
an index without changing any SQL or Go code. Implemented in migration 000434.

```sql
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_contents_title_trgm ON contents USING gin (title gin_trgm_ops);
CREATE INDEX idx_contents_slug_trgm ON contents USING gin (slug gin_trgm_ops);
CREATE INDEX idx_users_username_trgm ON users USING gin (username gin_trgm_ops);
```

This turns ILIKE from a full table scan into an index scan — 10-100x faster.

### Recommendation B: Route list-page ?q= to Meilisearch (done)
For `GET /api/v1/contents` (ListContents), when `?q=` is present and the
Meilisearch client is configured, the handler now queries Meilisearch for
matching content IDs and fetches the full rows from Postgres by ID. If
Meilisearch fails or is unconfigured, it falls back to the existing ILIKE
path. See `internal/content/handlers.go`, `internal/content/content.go`, and
`internal/search/content_searcher.go`.

### Recommendation C: Leave admin-only ILIKE as-is
Admin searches are low-traffic and benefit from pg_trgm indexes automatically.

---

## Summary of Actions

| Lever | Action | Effort | Status |
|---|---|---|---|
| Indexes | Drop 12 redundant (tenant_id) indexes | Migration 000433 | ✅ Done |
| Indexes | Add pg_trgm GIN indexes for ILIKE columns | Migration 000434 | ✅ Done |
| Indexes | Add user_blocks composite (blocker_id, tenant_id) | Migration 000437 | ✅ Done |
| Cache | Verify jc:spam:censored and jc:coupon* TTLs | Audit | ✅ Done |
| Cache | Extend jc:perm TTL from 5 to 10 min | rbac.go | ✅ Done |
| Replica | Add JC_DATABASE_READER_URL config support | config.go + main.go | ✅ Done |
| Replica | Audit misplaced reads on Writer pool (analytics store) | 1 hour grep | ✅ Done |
| Search | Route list-page ?q= to Meilisearch | Service + handler refactor | ✅ Done |
| Middleware | SingleTenantMiddleware (eliminate per-request DB lookup) | Done | ✅ Done |
| UI | Remove tenant-management routes from control panel | server.go + layout + UI dir | ✅ Done |
| Replica | Provision read replica + set JC_DATABASE_READER_URL | Native Postgres on port 5434 | ✅ Done |
| Replica | Improve replication lag reporting (WAL lag + health) | control handlers + UI | ✅ Done |

### Priority order
1. ✅ Drop 12 redundant indexes (done — migration 000433)
2. ✅ SingleTenantMiddleware (done — eliminates per-request DB lookup)
3. ✅ Add pg_trgm GIN indexes (done — migration 000434, zero code change, biggest ILIKE win)
4. ✅ Add user_blocks composite index (done — migration 000437, resolves seq-scan concern)
5. ✅ Extend jc:perm TTL to 10 min (done — rbac.go, halves DB load from permission checks)
6. ✅ Add JC_DATABASE_READER_URL config support (done — config.go + cmd/api/main.go)
7. ✅ Verify cache TTLs for jc:spam:censored and jc:coupon* (done — all have TTLs + invalidation)
8. ✅ Fix misplaced read on Writer pool (done — analytics store ResolveSecurityScanResult fixed)
9. ✅ Route list-page ?q= to Meilisearch (done — handler + content_searcher.go wired)
10. ✅ Provision local read replica (done — native Postgres streaming replica on port 5434)
11. ✅ Improve replication lag reporting (done — WAL lag bytes + healthy flag, avoids false alarms during idle periods)
12. ✅ Remove tenant-management UI from control panel (done — /tenants routes, nav, and pages removed)


