# Design — For-You Feed Frontend (Rendering Layer)

**Date:** 2026-07-18
**Surface:** JapaConnect Main App (`apps/main`, SvelteKit 2 + Svelte 5 Runes, SSR, port 3000)
**Status:** APPROVED (2026-07-18; per-type card style added & approved 2026-07-19) — awaiting final spec review
**Roadmap context:** ROADMAP-MAIN Phase 04 (Home Timeline). **Sub-project #2** of the personalized feed work; sub-project #1 (backend `GET /feed/for-you`) is complete and merged.
**Predecessor spec:** `docs/superpowers/specs/2026-07-18-personalized-for-you-feed-design.md` (backend, §1 names this frontend layer as #2).
**Companion spec:** `docs/superpowers/specs/2026-07-19-per-type-feed-card-style-design.md` (per-type card style — backend + control panel; Layers 1 & 3 of the card-style feature, this spec being Layer 2).

---

## 0. Purpose & how we got here

The rebuilt Main App (ADR-058 shell) currently renders a **placeholder** Home timeline: `(app)/+page.svelte` shows tabs and sort chips whose selection is *visual only* — the comment at its top says "wiring each tab/sort to its own feed query is the next feed task." `(app)/+page.server.ts` fetches a single feed with the wrong param (`?limit=20`).

This spec turns that placeholder into a real, personalized, multi-tab feed and fixes the three live API-Gate bugs the backend spec flagged.

**Decisions already made (this brainstorm):**

| # | Decision | Choice |
|---|----------|--------|
| F1 | Tab/sort fetch strategy | **URL-driven SSR** — tab & sort are `?tab=`/`?sort=` search params; anchor-link navigation re-runs the server `load`. Token stays server-side; feed views are shareable. |
| F2 | Pagination | **Infinite scroll** — `IntersectionObserver` sentinel client-fetches the next `page_offset` and appends. |
| F3 | Card style | **Honor all 4** variants in one `FeedCard`, selected **per content type** via `feed.card_style` = `{ default, by_type }`. Container is a 2-col grid only when the whole page resolves to `grid`, else a stream (a `grid` item → full-width card). Backend map shape + admin UI: **companion spec** `2026-07-19-per-type-feed-card-style-design.md`. |
| F4 | Card richness | **Media-rich** — the `magazine` variant carries avatar, relative time, reading time, tags, cover image; other variants share the same data with lighter layout. |
| F5 | Container treatment | **Bordered rounded surfaces for all four** (`--color-surface`, 1px `--color-border`, `--radius-card`): `reddit`/`compact`/`magazine` are bordered feed surfaces with hairline row dividers; `grid` is individual 2-up cards. *(Supersedes an earlier edge-to-edge/placeholder preference after reviewing the rendered mockup.)* |
| F6 | Vote / bookmark controls | `reddit` shows a **visible up/down vote rail**; `compact` shows up-chevron + `vote_score`. The vote arrows are the community affordance but the mutation is wired in the later interactions unit (§7), so they are inert in v1. Bookmark/comment/view stay display-only counts. |
| F7 | Type badge | **Quiet neutral** — one surface-subtle badge for all content types; no per-type color. |

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

**Card style (per content type):** `GET /feed/card-style` (public) → `{ "data": { "default": CardStyle, "by_type": { [content_type]: CardStyle } } }`. The backend tolerates the legacy scalar value (`"reddit"`) and normalizes it to `{ default: "reddit", by_type: {} }`. Valid `CardStyle`: `reddit`, `compact`, `grid`, `magazine`. The map shape, migration, and admin UI live in the **companion spec** `2026-07-19-per-type-feed-card-style-design.md` (backend + control). The frontend resolves a style **per feed item**: `cardStyleFor(item.content_type)` = `by_type[content_type] ?? default`.

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
- **`cardStyle.ts`** — types `CardStyle = 'reddit' | 'compact' | 'grid' | 'magazine'` and `CardStyleConfig = { default: CardStyle; byType: Partial<Record<string, CardStyle>> }`. `normalizeConfig(raw): CardStyleConfig` accepts the endpoint's `{ default, by_type }` **or** a legacy scalar, coercing unknown values to `'reddit'`. `cardStyleFor(contentType, cfg): CardStyle` returns `cfg.byType[contentType] ?? cfg.default`. Pure, exhaustively unit-tested.
- **`FeedTabs` / `FeedSortBar`** — presentational. Given the current `tab`/`sort` and the active-item id, render anchor links whose `href` preserves the *other* param (switching sort keeps your tab and vice-versa). No imperative routing.
- **`FeedList`** — given `{ items, tab, sort, pageSize, cardStyleConfig }`. Seeds a local `items` copy from the prop; a `$effect` re-seeds it whenever the incoming SSR `items` identity changes (a new tab/sort produces a fresh SSR payload → replace, never append). Owns `loading` / `atEnd` and the `IntersectionObserver`. Also owns the **layout mode**: computes each item's style via `cardStyleFor`, and renders a 2-col grid **iff every item resolves to `grid`**, else a single-column stream (§3A).
- **`FeedCard`** — given `{ item, style, layout }` where `layout ∈ 'stream' | 'grid-cell'`. A single component; conditional card layout per `style`, sharing one data set and one metric row. A `grid`-styled item renders as a full-width image-top card in `stream` layout and as a grid cell in `grid-cell` layout.

**Why anchor links, not `onclick` + `goto`:** SvelteKit intercepts same-origin anchors for client-side navigation automatically. This yields URL-as-state, progressive enhancement (works with JS disabled), and `$navigating`-driven loading with zero imperative routing code. Infinite scroll is the single feature that genuinely requires JS; without JS the feed degrades gracefully to its first page.

---

## 3. Data flow

1. **`(app)/+layout.server.ts`** — add one graceful call:
   `cardStyleConfig = normalizeConfig(await apiGet('/feed/card-style'))`, degrading to `{ default: 'reddit', byType: {} }` on failure. Returned in layout data so the whole shell (and `FeedList`) can read it. Does **not** depend on `?tab`/`?sort`, so it will not re-run on tab/sort switches.

2. **`(app)/+page.server.ts`** — rewritten:
   - Read `tab` and `sort` from `url.searchParams`.
   - Guests are forced to `tab = 'explore'`; authed users default to `tab = 'for_you'` when absent/invalid.
   - `sort` defaults to `'relevant'` when absent/invalid, and is always sent explicitly.
   - Build the path with `feedPath(tab, sort, pageSize, 0)`; fetch page 0.
   - Return `{ items, tab, sort, authed, pageSize, loadError }`, preserving the existing `try/catch → loadError` graceful degradation.

3. **`(app)/+page.svelte`** — compose `FeedTabs` + `FeedSortBar` (active state derived from `page.url` / loader data) + `FeedList` (passed `items`, `tab`, `sort`, `pageSize`, and `cardStyleConfig` from layout data). The inline card markup currently in this file is removed in favor of `FeedList` → `FeedCard`.

4. **`FeedList.svelte`** — layout mode + infinite scroll:
   - **Layout mode** decided from the **initial SSR page**: `gridMode = items.length > 0 && items.every(i => cardStyleFor(i.content_type, cardStyleConfig) === 'grid')`. `gridMode` → 2-col grid container (cards render as `grid-cell`); otherwise → single-column stream (each card `stream`; a `grid`-resolved item is a full-width image-top card). Held across appends and **re-decided on tab/sort change** (fresh SSR page) — never flips mid-scroll.
   - Sentinel `<div>` after the last card; `IntersectionObserver` fires `loadMore()`.
   - `loadMore()` guards on `loading || atEnd`, then `api.get<FeedItem[]>(feedPath(tab, sort, pageSize, items.length))`, appends results, and sets `atEnd = (returned.length < pageSize)`.
   - Observer is created in an `$effect` and disconnected on teardown and on `atEnd`.

5. **`FeedCard.svelte`** — switches layout on `style` (`magazine` / `grid` / `compact` / else `reddit`). Full per-variant visual specification is **§3A** (locked).

---

## 3A. Card visual specification (locked 2026-07-19)

Interactive reference mockup (real tokens, theme-aware, light/dark toggle): artifact `e9dbc3d3-06aa-46e3-bb34-6c0840103611` (source `feed-card-variants.html`). Fonts there are approximated (Inter→system sans, Lora→Georgia); production uses the real faces. Cover images there are gradient placeholders for `cover_image_url`.

**Tokens (from `app.css` — consume, never redefine):** zinc neutral surfaces; `--color-primary` `#3b49df` (light) / `#5865f2` (dark); `--radius-card` 12px; Inter (`--font-sans`) everywhere **except** the magazine headline, which uses `--font-serif` (Lora). Theme is driven by `[data-theme]`, never Tailwind `dark:`.

**Shared elements (identical `FeedItem` data across all four):**
- **Type badge** — quiet neutral for *every* content type: `--color-surface-subtle` bg, `--color-text-subtle` text, 1px `--color-border`, uppercase ~10.5px / +0.05em tracking, radius 5px. No per-type color.
- **Metric row** — muted, ~12.5px, `tabular-nums`, gap 16px; lucide icons at 14px for `vote_score` (arrow-up), `comment_count` (message-square), `view_count` (eye), `bookmark_count` (bookmark). **Display-only in v1.**
- **Vote control** — `reddit`: a vertical **up/down rail** (arrow-up · `vote_score` · arrow-down) is **visible** in v1; `compact`: up-chevron + `vote_score`. The arrows render as the community-vote affordance, but the mutation is wired in the later interactions unit (§7), so they are inert in v1. `grid`/`magazine` show `vote_score` in the metric row (no rail).
- **Avatar** — `author_avatar_url` when present; else initials on a solid fill; circular.
- **Tags** — `--color-primary`, ~12px, `#tag` form, wrap.
- **Relative time** — from `time.ts`; **omitted entirely** when unparseable (never renders "Invalid Date").
- **Cover image** — `loading="lazy"`; rendered only when `cover_image_url` is present *and* the variant uses it.

**Container treatment (locked):**

| Variant | Container |
|---------|-----------|
| `reddit` (default) | **Bordered feed surface** (`--color-surface`, 1px `--color-border`, `--radius-card`) with hairline dividers between rows. |
| `compact` | Bordered feed surface with hairline dividers between one-line rows. |
| `grid` | Individual bordered rounded cards. **2-col grid only in `gridMode`** (every item on the page resolves to `grid`; 1-col below ~560px); in a mixed stream a `grid` item is a **full-width image-top card**. |
| `magazine` | Bordered feed surface, single column, hairline dividers between posts. |

**Per-item style resolution & layout mode (per-type card style):** each card's variant is resolved per item — `cardStyleFor(item.content_type, cardStyleConfig)` = `by_type[content_type] ?? default`. The feed's **container layout** is a single self-adjusting rule: it renders a **2-col grid iff every item on the page resolves to `grid`** (`gridMode`), otherwise a **single-column stream** in which a `grid`-resolved item renders as a full-width image-top card. This gives both behaviors with no context flag: a single-type all-`grid` listing becomes 2-col automatically, while a mixed For-You feed with e.g. `blog_post → grid` keeps blog posts as full-width cards aligned with the other rows, preserving rank order. Mode is fixed from the initial SSR page and re-decided on tab/sort change (never flips mid-scroll).

**Per-variant layout:**
- **`reddit`** — row = `[up/down vote rail ~30px] · [body] · [thumbnail 84px]`. Body: meta line (`badge · space · author · time`), ~15.5px/600 title, 1-line-clamped excerpt trailing `· Nm read`, foot row (tags left / metrics right). Thumbnail only when `cover_image_url`. Row hover → `surface-subtle`.
- **`compact`** — one line: `[▲ score ~44px] · [title, ellipsis, flex-1] · [space · time] · [comments · views]`. No excerpt, no image. `space · time` hides below ~560px.
- **`grid`** — cover (16:9), then body: badge, 2-line-clamped ~15px/600 title, meta (`avatar · author · time`) pinned to the card bottom, metric row. Hover → border `--color-muted`.
- **`magazine`** — header (`~34px avatar` + author / `space · time · Nm read`), 16:9 hero (rounded, bordered), badge, **serif headline ~25px/500 Lora with `text-wrap: balance`**, 2-line ~15px excerpt, foot (tags / metrics). The only variant that leaves Inter — the app's "this is a read" typographic signature.

**No-image behavior:** reddit/compact omit the thumbnail; grid/magazine render without the cover block (content moves up) — **no gray placeholder box** in production.

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
  - `cardStyle.ts` — `normalizeConfig` accepts `{ default, by_type }` **and** a legacy scalar; unknown values → `'reddit'`. `cardStyleFor` returns the per-type override or the default. `gridMode` predicate: true only when every item resolves to `grid`; false for any mixed set.
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

> **Cross-layer dependency:** this frontend layer consumes the `GET /feed/card-style` → `{ default, by_type }` contract defined in the companion spec (Layer 1). Build the backend contract first; the frontend can then be developed and tested against a hand-set `feed.card_style` map before the Control UI (Layer 3) exists.

1. Pure modules + unit tests: `endpoint.ts`, `time.ts`, `cardStyle.ts` (incl. `cardStyleFor` + `gridMode`).
2. `FeedItem` type expansion + i18n keys.
3. `+layout.server.ts` card-style config fetch.
4. `+page.server.ts` rewrite (tab/sort/params, remove `?limit=`).
5. `FeedCard.svelte` (4 variants × `stream`/`grid-cell` layout) + `FeedList.svelte` (layout mode + infinite scroll).
6. `FeedTabs.svelte` + `FeedSortBar.svelte` (anchor links).
7. `(app)/+page.svelte` composition (remove inline card markup).
8. API-Readiness Gate verification + `svelte-check`/lint green.
