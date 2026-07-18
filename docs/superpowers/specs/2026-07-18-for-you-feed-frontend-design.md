# Design — For-You Feed Frontend (Rendering Layer)

**Date:** 2026-07-18
**Surface:** JapaConnect Main App (`apps/main`, SvelteKit 2 + Svelte 5 Runes, SSR, port 3000)
**Status:** APPROVED (2026-07-18) — awaiting final spec review
**Roadmap context:** ROADMAP-MAIN Phase 04 (Home Timeline). **Sub-project #2** of the personalized feed work; sub-project #1 (backend `GET /feed/for-you`) is complete and merged.
**Predecessor spec:** `docs/superpowers/specs/2026-07-18-personalized-for-you-feed-design.md` (backend, §1 names this frontend layer as #2).

---

## 0. Purpose & how we got here

The rebuilt Main App (ADR-058 shell) currently renders a **placeholder** Home timeline: `(app)/+page.svelte` shows tabs and sort chips whose selection is *visual only* — the comment at its top says "wiring each tab/sort to its own feed query is the next feed task." `(app)/+page.server.ts` fetches a single feed with the wrong param (`?limit=20`).

This spec turns that placeholder into a real, personalized, multi-tab feed and fixes the three live API-Gate bugs the backend spec flagged.

**Decisions already made (this brainstorm):**

| # | Decision | Choice |
|---|----------|--------|
| F1 | Tab/sort fetch strategy | **URL-driven SSR** — tab & sort are `?tab=`/`?sort=` search params; anchor-link navigation re-runs the server `load`. Token stays server-side; feed views are shareable. |
| F2 | Pagination | **Infinite scroll** — `IntersectionObserver` sentinel client-fetches the next `page_offset` and appends. |
| F3 | Card style | **Honor all 4** `feed.card_style` variants (`reddit`/`compact`/`grid`/`magazine`) in one `FeedCard`, selected by the real backend setting. |
| F4 | Card richness | **Media-rich** — the `magazine` variant carries avatar, relative time, reading time, tags, cover image; other variants share the same data with lighter layout. |

**Rule 16 clean:** every rendered field and the card style come from real backend data. No fabricated ordering, no dead admin control.

---

## 1. Contract layer (verified against the live backend)

Verified in `internal/feed/handlers.go` + `internal/server/server.go` (routes) on 2026-07-18.

| Tab (UI) | Route | Auth | Notes |
|----------|-------|------|-------|
| Following | `GET /feed` | `RequireAuth` (401 otherwise) | fan-out primary timeline |
| For You | `GET /feed/for-you` | `RequireAuth` | personalized ranked (sub-project #1) |
| Explore (guest) | `GET /explore` | public | the documented guest fallback |

**Query params — identical across all three** (`parsePageSize` / `parsePageOffset` / `parseSort`):

- `page_size` — default 20, clamped to backend `MaxPageSize`.
- `page_offset` — default 0.
- `sort` — one of `relevant` | `latest` | `top` | `rising`.

**Sort chip label → `sort` value (the API-Gate mapping):**

| Chip label | `sort` value |
|------------|--------------|
| Relevant | `relevant` |
| Latest | `latest` |
| **Popular** | **`top`** |
| Rising | `rising` |

**Critical backend behavior:** `parseSort` **defaults to `latest`** when `sort` is absent or unrecognized. Therefore the frontend MUST always send an explicit `sort` value; it must never rely on omission to mean "relevant."

**Card style:** `GET /feed/card-style` (public) → `{ "data": { "card_style": string } }`, default `"reddit"`. The setting is tenant-scoped (`system_settings` key `feed.card_style`) and is exposed by a live admin selector in the Control app. Valid values: `reddit`, `compact`, `grid`, `magazine`.

**`created_at` format:** the backend emits a **non-ISO** timestamp string, e.g. `"2026-07-12 08:03:55.888616 +0100 WAT"`. `new Date(...)` on this returns `Invalid Date` in most engines. Safe parsing is required (see §4).

**Response shape:** every feed endpoint returns `{ "data": FeedItem[] }` — the identical `FeedItem` contract for all three tabs. One `FeedCard` renders all of them; only the fetch URL changes.

---

## 2. Component & module architecture

New files under `apps/main/src`:

```
lib/components/feed/
  FeedTabs.svelte      — tab bar; renders <a href="?tab=…&sort=…"> anchor links
  FeedSortBar.svelte   — sort chips; anchor links carrying the mapped sort value
  FeedList.svelte      — seeds items from SSR props; owns infinite-scroll append
  FeedCard.svelte      — a single card; switches layout on the cardStyle variant
lib/feed/
  endpoint.ts          — PURE: (tab, sort, pageSize, pageOffset) → backend path
  time.ts              — PURE: safe-parse the non-ISO created_at → relative label
  cardStyle.ts         — PURE: normalize/validate a card_style value (fallback 'reddit')
```

**Responsibilities & interfaces (deep-module discipline):**

- **`endpoint.ts`** — `feedPath(tab: FeedTab, sort: SortValue, pageSize: number, pageOffset: number): string`. The *only* place tab→route and chip→sort knowledge lives. Pure, exhaustively unit-tested. `FeedTab = 'following' | 'for_you' | 'explore'`. Returns a **bare path with no `/api/v1` prefix** (e.g. `/feed/for-you?sort=top&page_size=20&page_offset=40`) — both the server client (`apiList`, prepends `API_V1`) and the browser client (`api.get`, prepends `PUBLIC_JC_API_URL`) add their own base, so double-prefixing must be avoided.
- **`time.ts`** — `relativeTime(raw: string, now?: Date): string`. Parses ISO **and** the backend's space-separated `+0100 WAT` form; returns `""` on unparseable input (card then omits the timestamp rather than showing "Invalid Date").
- **`cardStyle.ts`** — `normalizeCardStyle(v: string | null | undefined): CardStyle`. Maps unknown values to `'reddit'`. `CardStyle = 'reddit' | 'compact' | 'grid' | 'magazine'`.
- **`FeedTabs` / `FeedSortBar`** — presentational. Given the current `tab`/`sort` and the active-item id, render anchor links whose `href` preserves the *other* param (switching sort keeps your tab and vice-versa). No imperative routing.
- **`FeedList`** — given `{ items, tab, sort, pageSize, cardStyle }`. Seeds a local `items` copy from the prop; a `$effect` re-seeds it whenever the incoming SSR `items` identity changes (a new tab/sort produces a fresh SSR payload → replace, never append). Owns `loading` / `atEnd` and the `IntersectionObserver`.
- **`FeedCard`** — given `{ item, style }`. A single component; conditional layout per `style`, sharing one data set and one metric row.

**Why anchor links, not `onclick` + `goto`:** SvelteKit intercepts same-origin anchors for client-side navigation automatically. This yields URL-as-state, progressive enhancement (works with JS disabled), and `$navigating`-driven loading with zero imperative routing code. Infinite scroll is the single feature that genuinely requires JS; without JS the feed degrades gracefully to its first page.

---

## 3. Data flow

1. **`(app)/+layout.server.ts`** — add one graceful call:
   `cardStyle = normalizeCardStyle((await apiGet('/feed/card-style')).card_style)`, degrading to `'reddit'` on failure. Returned in layout data so the whole shell (and `FeedList`) can read it. Does **not** depend on `?tab`/`?sort`, so it will not re-run on tab/sort switches.

2. **`(app)/+page.server.ts`** — rewritten:
   - Read `tab` and `sort` from `url.searchParams`.
   - Guests are forced to `tab = 'explore'`; authed users default to `tab = 'for_you'` when absent/invalid.
   - `sort` defaults to `'relevant'` when absent/invalid, and is always sent explicitly.
   - Build the path with `feedPath(tab, sort, pageSize, 0)`; fetch page 0.
   - Return `{ items, tab, sort, authed, pageSize, loadError }`, preserving the existing `try/catch → loadError` graceful degradation.

3. **`(app)/+page.svelte`** — compose `FeedTabs` + `FeedSortBar` (active state derived from `page.url` / loader data) + `FeedList` (passed `items`, `tab`, `sort`, `pageSize`, and `cardStyle` from layout data). The inline card markup currently in this file is removed in favor of `FeedList` → `FeedCard`.

4. **`FeedList.svelte`** — infinite scroll:
   - Sentinel `<div>` after the last card; `IntersectionObserver` fires `loadMore()`.
   - `loadMore()` guards on `loading || atEnd`, then `api.get<FeedItem[]>(feedPath(tab, sort, pageSize, items.length))`, appends results, and sets `atEnd = (returned.length < pageSize)`.
   - Observer is created in an `$effect` and disconnected on teardown and on `atEnd`.

5. **`FeedCard.svelte`** — `{#if style === 'magazine'}` large hero image + full meta; `{:else if style === 'grid'}` image-top compact card; `{:else if style === 'compact'}` dense single-line; `{:else}` `reddit` default (thumbnail + text). All variants share: type badge, title, author, relative time (when parseable), and the metric row (votes / comments / views / bookmarks). Cover image is lazy-loaded (`loading="lazy"`) and only rendered when `cover_image_url` is present and the variant uses it.

---

## 4. Type & i18n changes

**Expand `FeedItem`** (`lib/types/index.ts`) to declare fields the backend already returns but the type omits:

```ts
author_avatar_url?: string | null;
tags?: string[];
reading_time_minutes?: number;
cover_image_url?: string | null;
bookmark_count?: number;
velocity_score?: number;
```

(Additive only; existing fields unchanged. `space_name` already present.)

**i18n (`lib/i18n/en.json` + `fr.json`):** reuse existing `feed.tab.following`, `feed.tab.for_you`, `sort.relevant|latest|popular|rising`. Add keys as needed for: reading-time suffix, end-of-feed label, feed load-error title/body (may reuse existing `error.500_*`), and any new empty-state copy. Every new key added to both locales.

---

## 5. States & resilience

| Condition | UI |
|-----------|----|
| SSR page-0 fetch fails (`loadError`) | `EmptyState` (error variant); shell stays intact |
| Zero items | `EmptyState` (default copy) |
| Tab/sort switch in flight | `$navigating` drives `LoadingSkeleton` |
| Infinite-scroll fetch in flight | small bottom spinner |
| `atEnd` reached (short page / ~600 ceiling) | quiet stop — no error, observer retired |
| A card's `created_at` unparseable | timestamp omitted (never "Invalid Date") |
| A widget/card image 404s | broken-image avoided via graceful `onerror`/fallback |

The backend's ~600-item candidate ceiling (backend spec §4) gives infinite scroll a natural terminal state; `atEnd` prevents runaway fetching past it.

---

## 6. Testing & gates

- **Pure unit tests (vitest):**
  - `endpoint.ts` — every tab base path; every chip→sort mapping incl. **Popular→`top`**; explicit `relevant` is always emitted; `page_size`/`page_offset` present (never `limit`).
  - `time.ts` — the backend's `"… +0100 WAT"` string parses to a sensible relative label; ISO input parses; empty/garbage input returns `""`.
  - `cardStyle.ts` — each valid value passes through; unknown/null/undefined → `'reddit'`.
- **Component behavior (lightweight):** `FeedList` re-seeds (not appends) when the SSR `items` prop changes; `loadMore` respects the `loading`/`atEnd` guards.
- **API-Readiness Gate:** before merge, validate against the live backend — param names (`page_size`/`page_offset`/`sort`), `/feed/card-style` response shape, and the `created_at` format assumption. No mock-shaped-only verification.
- **Baseline green:** `svelte-check` and lint must pass (repo baseline is all-green; any failure is a new regression).
- **Rule 16:** confirm no fabricated ordering and that the admin `feed.card_style` selector demonstrably changes rendering.

---

## 7. Out of scope (v1, explicit)

- Vote / react / bookmark **mutations** from cards — counts are rendered read-only; interactions are a separate unit.
- Seen / impression dedupe (no backend table exists — noted in backend spec §3).
- "N new posts" live banner / WebSocket refresh.
- Right-sidebar widget changes.
- New card styles beyond the four the backend defines.
- Server-side rendering of pages 2+ (infinite scroll is client-only by design; first page is SSR).

---

## 8. Build sequence (for the implementation plan)

1. Pure modules + unit tests: `endpoint.ts`, `time.ts`, `cardStyle.ts`.
2. `FeedItem` type expansion + i18n keys.
3. `+layout.server.ts` card-style fetch.
4. `+page.server.ts` rewrite (tab/sort/params, remove `?limit=`).
5. `FeedCard.svelte` (4 variants) + `FeedList.svelte` (infinite scroll).
6. `FeedTabs.svelte` + `FeedSortBar.svelte` (anchor links).
7. `(app)/+page.svelte` composition (remove inline card markup).
8. API-Readiness Gate verification + `svelte-check`/lint green.
