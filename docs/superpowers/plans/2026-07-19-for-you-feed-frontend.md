# For-You Feed Frontend (Layer 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Follow the `jc-main-develop` conventions (SvelteKit 2 + Svelte 5 Runes + Tailwind v4, tokens via CSS vars, never Tailwind `dark:`).

**Goal:** Turn the placeholder Home timeline into a real multi-tab, multi-sort, infinitely-scrolling feed whose card style is resolved per content type, and fix the three API-Gate bugs.

**Architecture:** Tabs and sort are URL search params rendered as `<a href>` links (SvelteKit intercepts them → server `load` re-runs, token stays server-side, `$navigating` drives loading). Three pure modules isolate the bug-prone logic (`endpoint.ts` param mapping, `time.ts` non-ISO date, `cardStyle.ts` per-type resolution + grid-mode). `FeedList` seeds from SSR, appends via `IntersectionObserver`, and picks a 2-col-grid vs single-column-stream layout from the resolved styles. `FeedCard` renders one item in its resolved style.

**Tech Stack:** SvelteKit 2.69, Svelte 5.56 (runes), Tailwind v4, TypeScript, Vitest (jsdom), lucide-svelte.

## Global Constraints

- Feed routes (bare paths, no `/api/v1` prefix — the clients add their base): Following `/feed`, For You `/feed/for-you`, guest `/explore`. All three take `page_size`, `page_offset`, `sort`.
- Sort chip label → `sort` value: Relevant→`relevant`, Latest→`latest`, **Popular→`top`**, Rising→`rising`. The backend `parseSort` defaults to `latest`, so the frontend MUST always send an explicit `sort`.
- `CardStyle ∈ { reddit, compact, grid, magazine }`; config is `{ default, byType }` from `GET /feed/card-style` (Layer 1). Resolve per item: `cardStyleFor(content_type) = byType[content_type] ?? default`.
- Layout mode: render a 2-col grid **iff every item on the page resolves to `grid`**, else a single-column stream where a `grid` item is a full-width image-top card. Mode is fixed from the initial SSR page.
- Container treatment: `reddit`/`compact`/`magazine` in bordered surfaces with hairline dividers; `grid` cells are individual bordered cards. Type badge is quiet neutral. Vote is display in stream (reddit shows up/down arrows, inert in v1 — mutations are out of scope).
- `created_at` is non-ISO (`"2026-07-12 08:03:55.888616 +0100 WAT"`); parse safely, and omit the timestamp (never "Invalid Date") when unparseable.
- Tokens: use CSS vars (`var(--color-*)`, `var(--radius-card)`) via Tailwind arbitrary values, matching the existing placeholder. Theme is `[data-theme]`, never Tailwind `dark:`.
- Tests: `pnpm test` / `npx vitest run` (jsdom); pure modules are fully unit-tested. Gate: `npm run check` (svelte-check) + `npm run build` stay green.
- Visual authority: spec §3A + the mockup artifact `e9dbc3d3-06aa-46e3-bb34-6c0840103611`.
- Spec: `docs/superpowers/specs/2026-07-18-for-you-feed-frontend-design.md`. Depends on Layer 1 (`GET /feed/card-style` returning `{default, by_type}`); can be developed against a hand-set setting before Layer 3.

---

### Task 1: `endpoint.ts` — feed path + param resolution (pure)

**Files:**
- Create: `apps/main/src/lib/feed/endpoint.ts`
- Test: `apps/main/src/lib/feed/endpoint.test.ts`

**Interfaces:**
- Produces:
  - `type FeedTab = 'following' | 'for_you' | 'explore'`
  - `type SortValue = 'relevant' | 'latest' | 'top' | 'rising'`
  - `const PAGE_SIZE = 20`
  - `function feedPath(tab: FeedTab, sort: SortValue, pageSize: number, pageOffset: number): string`
  - `function resolveTab(raw: string | null, authed: boolean): FeedTab`
  - `function resolveSort(raw: string | null): SortValue`

- [ ] **Step 1: Write the failing test**

Create `apps/main/src/lib/feed/endpoint.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from './endpoint';

describe('feedPath', () => {
	it('maps tabs to bare backend paths (no /api/v1)', () => {
		expect(feedPath('following', 'relevant', 20, 0)).toBe('/feed?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('for_you', 'relevant', 20, 0)).toBe('/feed/for-you?sort=relevant&page_size=20&page_offset=0');
		expect(feedPath('explore', 'relevant', 20, 0)).toBe('/explore?sort=relevant&page_size=20&page_offset=0');
	});

	it('uses page_size/page_offset, never limit', () => {
		const p = feedPath('for_you', 'top', 20, 40);
		expect(p).toContain('page_size=20');
		expect(p).toContain('page_offset=40');
		expect(p).not.toContain('limit=');
	});

	it('carries the raw sort value including top for Popular', () => {
		expect(feedPath('for_you', 'top', 20, 0)).toContain('sort=top');
	});
});

describe('resolveTab', () => {
	it('forces guests to explore', () => {
		expect(resolveTab('for_you', false)).toBe('explore');
		expect(resolveTab(null, false)).toBe('explore');
	});
	it('defaults authed users to for_you and accepts valid tabs', () => {
		expect(resolveTab(null, true)).toBe('for_you');
		expect(resolveTab('following', true)).toBe('following');
		expect(resolveTab('for_you', true)).toBe('for_you');
		expect(resolveTab('explore', true)).toBe('for_you'); // authed can't select explore
		expect(resolveTab('garbage', true)).toBe('for_you');
	});
});

describe('resolveSort', () => {
	it('accepts the four valid values', () => {
		for (const s of ['relevant', 'latest', 'top', 'rising'] as const) {
			expect(resolveSort(s)).toBe(s);
		}
	});
	it('defaults invalid/absent to relevant (never latest)', () => {
		expect(resolveSort(null)).toBe('relevant');
		expect(resolveSort('popular')).toBe('relevant'); // "popular" is a label, not a value
		expect(resolveSort('')).toBe('relevant');
	});
});

it('PAGE_SIZE is 20', () => {
	expect(PAGE_SIZE).toBe(20);
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && npx vitest run src/lib/feed/endpoint.test.ts`
Expected: FAIL — cannot resolve `./endpoint`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/main/src/lib/feed/endpoint.ts`:

```ts
/** Which feed a Home tab maps to. */
export type FeedTab = 'following' | 'for_you' | 'explore';

/** Backend `sort` values (note: chip label "Popular" → value "top"). */
export type SortValue = 'relevant' | 'latest' | 'top' | 'rising';

export const PAGE_SIZE = 20;

const TAB_PATH: Record<FeedTab, string> = {
	following: '/feed',
	for_you: '/feed/for-you',
	explore: '/explore'
};

const SORTS: SortValue[] = ['relevant', 'latest', 'top', 'rising'];

/**
 * Build the bare backend path for a feed request. Returns a path WITHOUT the
 * /api/v1 prefix — the server client (apiList) and browser client (api.get)
 * each prepend their own base, so we must not double-prefix.
 */
export function feedPath(tab: FeedTab, sort: SortValue, pageSize: number, pageOffset: number): string {
	return `${TAB_PATH[tab]}?sort=${sort}&page_size=${pageSize}&page_offset=${pageOffset}`;
}

/** Resolve the active tab. Guests are always explore; authed users default to for_you. */
export function resolveTab(raw: string | null, authed: boolean): FeedTab {
	if (!authed) return 'explore';
	if (raw === 'following' || raw === 'for_you') return raw;
	return 'for_you';
}

/** Resolve the active sort; always yields an explicit value (default relevant). */
export function resolveSort(raw: string | null): SortValue {
	return (SORTS as string[]).includes(raw ?? '') ? (raw as SortValue) : 'relevant';
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && npx vitest run src/lib/feed/endpoint.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/feed/endpoint.ts apps/main/src/lib/feed/endpoint.test.ts
git commit -m "feat(main): feed endpoint path + param resolver (pure)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: `time.ts` — safe relative-time parsing (pure)

**Files:**
- Create: `apps/main/src/lib/feed/time.ts`
- Test: `apps/main/src/lib/feed/time.test.ts`

**Interfaces:**
- Produces: `function relativeTime(raw: string, now?: Date): string` — `''` when unparseable.

- [ ] **Step 1: Write the failing test**

Create `apps/main/src/lib/feed/time.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { relativeTime } from './time';

describe('relativeTime', () => {
	const now = new Date('2026-07-12T09:03:55Z');

	it('parses the backend non-ISO "+0100 WAT" format', () => {
		// 08:03:55 +0100 == 07:03:55Z, so 2h before 09:03:55Z
		expect(relativeTime('2026-07-12 08:03:55.888616 +0100 WAT', now)).toBe('2h');
	});

	it('parses ISO input', () => {
		expect(relativeTime('2026-07-12T08:33:55Z', now)).toBe('30m');
	});

	it('returns "now" for very recent times', () => {
		expect(relativeTime('2026-07-12T09:03:30Z', now)).toBe('now');
	});

	it('formats days and weeks', () => {
		expect(relativeTime('2026-07-09T09:03:55Z', now)).toBe('3d');
		expect(relativeTime('2026-06-25T09:03:55Z', now)).toBe('2w');
	});

	it('returns empty string for unparseable input', () => {
		expect(relativeTime('', now)).toBe('');
		expect(relativeTime('not a date', now)).toBe('');
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && npx vitest run src/lib/feed/time.test.ts`
Expected: FAIL — cannot resolve `./time`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/main/src/lib/feed/time.ts`:

```ts
/**
 * Parse a timestamp that may be ISO or the backend's non-ISO form,
 * e.g. "2026-07-12 08:03:55.888616 +0100 WAT". Returns null when unparseable.
 */
function parseTimestamp(raw: string): Date | null {
	if (!raw) return null;

	const iso = new Date(raw);
	if (!Number.isNaN(iso.getTime())) return iso;

	const m = raw.match(/^(\d{4}-\d{2}-\d{2})[ T](\d{2}:\d{2}:\d{2})(\.\d+)?\s*([+-]\d{2})(\d{2})/);
	if (!m) return null;

	const [, date, time, frac = '', offH, offM] = m;
	const built = new Date(`${date}T${time}${frac}${offH}:${offM}`);
	return Number.isNaN(built.getTime()) ? null : built;
}

/** Compact relative time ("now", "5m", "2h", "3d", "2w") or a short date; '' if unparseable. */
export function relativeTime(raw: string, now: Date = new Date()): string {
	const d = parseTimestamp(raw);
	if (!d) return '';

	const secs = Math.floor((now.getTime() - d.getTime()) / 1000);
	if (secs < 45) return 'now';
	const mins = Math.floor(secs / 60);
	if (mins < 60) return `${mins}m`;
	const hrs = Math.floor(mins / 60);
	if (hrs < 24) return `${hrs}h`;
	const days = Math.floor(hrs / 24);
	if (days < 7) return `${days}d`;
	const weeks = Math.floor(days / 7);
	if (weeks < 5) return `${weeks}w`;
	return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && npx vitest run src/lib/feed/time.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/feed/time.ts apps/main/src/lib/feed/time.test.ts
git commit -m "feat(main): safe relative-time parser for non-ISO created_at

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: `cardStyle.ts` — per-type resolution + grid mode (pure)

**Files:**
- Create: `apps/main/src/lib/feed/cardStyle.ts`
- Test: `apps/main/src/lib/feed/cardStyle.test.ts`

**Interfaces:**
- Produces:
  - `type CardStyle = 'reddit' | 'compact' | 'grid' | 'magazine'`
  - `interface CardStyleConfig { default: CardStyle; byType: Partial<Record<string, CardStyle>> }`
  - `function normalizeConfig(raw: unknown): CardStyleConfig`
  - `function cardStyleFor(contentType: string, cfg: CardStyleConfig): CardStyle`
  - `function gridMode(contentTypes: string[], cfg: CardStyleConfig): boolean`

- [ ] **Step 1: Write the failing test**

Create `apps/main/src/lib/feed/cardStyle.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { normalizeConfig, cardStyleFor, gridMode } from './cardStyle';

describe('normalizeConfig', () => {
	it('accepts the {default, by_type} object', () => {
		expect(normalizeConfig({ default: 'reddit', by_type: { blog_post: 'grid' } })).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('accepts a legacy scalar', () => {
		expect(normalizeConfig('magazine')).toEqual({ default: 'magazine', byType: {} });
	});
	it('coerces invalid default and drops invalid overrides', () => {
		expect(normalizeConfig({ default: 'bogus', by_type: { blog_post: 'grid', x: 'nope' } })).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('falls back for null/garbage', () => {
		expect(normalizeConfig(null)).toEqual({ default: 'reddit', byType: {} });
		expect(normalizeConfig(42)).toEqual({ default: 'reddit', byType: {} });
	});
});

describe('cardStyleFor', () => {
	const cfg = { default: 'reddit', byType: { blog_post: 'grid' } } as const;
	it('returns the per-type override', () => {
		expect(cardStyleFor('blog_post', cfg)).toBe('grid');
	});
	it('falls back to default for unmapped types', () => {
		expect(cardStyleFor('discussion', cfg)).toBe('reddit');
	});
});

describe('gridMode', () => {
	it('is true only when every type resolves to grid', () => {
		const allGrid = { default: 'grid', byType: {} };
		expect(gridMode(['blog_post', 'discussion'], allGrid)).toBe(true);
	});
	it('is false for a mixed set', () => {
		const cfg = { default: 'reddit', byType: { blog_post: 'grid' } };
		expect(gridMode(['blog_post', 'discussion'], cfg)).toBe(false);
	});
	it('is false for an empty page', () => {
		expect(gridMode([], { default: 'grid', byType: {} })).toBe(false);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && npx vitest run src/lib/feed/cardStyle.test.ts`
Expected: FAIL — cannot resolve `./cardStyle`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/main/src/lib/feed/cardStyle.ts`:

```ts
export type CardStyle = 'reddit' | 'compact' | 'grid' | 'magazine';

export interface CardStyleConfig {
	default: CardStyle;
	byType: Partial<Record<string, CardStyle>>;
}

const STYLES: CardStyle[] = ['reddit', 'compact', 'grid', 'magazine'];

function isStyle(v: unknown): v is CardStyle {
	return typeof v === 'string' && (STYLES as string[]).includes(v);
}

/** Normalize the /feed/card-style payload (object, legacy scalar, or junk) into a config. */
export function normalizeConfig(raw: unknown): CardStyleConfig {
	const fallback: CardStyleConfig = { default: 'reddit', byType: {} };
	if (!raw) return fallback;
	if (typeof raw === 'string') return { default: isStyle(raw) ? raw : 'reddit', byType: {} };
	if (typeof raw !== 'object') return fallback;

	const r = raw as { default?: unknown; by_type?: unknown };
	const byType: Partial<Record<string, CardStyle>> = {};
	if (r.by_type && typeof r.by_type === 'object') {
		for (const [k, v] of Object.entries(r.by_type as Record<string, unknown>)) {
			if (isStyle(v)) byType[k] = v;
		}
	}
	return { default: isStyle(r.default) ? r.default : 'reddit', byType };
}

/** The style for a given content type: its override, else the default. */
export function cardStyleFor(contentType: string, cfg: CardStyleConfig): CardStyle {
	return cfg.byType[contentType] ?? cfg.default;
}

/** True only when the page is non-empty and every content type resolves to grid. */
export function gridMode(contentTypes: string[], cfg: CardStyleConfig): boolean {
	return contentTypes.length > 0 && contentTypes.every((ct) => cardStyleFor(ct, cfg) === 'grid');
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && npx vitest run src/lib/feed/cardStyle.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/feed/cardStyle.ts apps/main/src/lib/feed/cardStyle.test.ts
git commit -m "feat(main): per-type card-style resolver + grid-mode (pure)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Expand `FeedItem` type + add i18n keys

**Files:**
- Modify: `apps/main/src/lib/types/index.ts` (the `FeedItem` interface, ~line 113)
- Modify: `apps/main/src/lib/i18n/en.json`
- Modify: `apps/main/src/lib/i18n/fr.json`

**Interfaces:**
- Produces: `FeedItem` with `author_avatar_url?`, `tags?`, `reading_time_minutes?`, `cover_image_url?`, `bookmark_count?`, `velocity_score?`; i18n keys `feed.tab.explore`, `feed.end`, `feed.min_read`.

- [ ] **Step 1: Expand the FeedItem interface**

In `apps/main/src/lib/types/index.ts`, replace the `FeedItem` interface body's closing fields so it reads:

```ts
export interface FeedItem {
	id: string;
	title: string;
	slug: string;
	content_type: ContentType;
	author_id: string;
	author_name?: string;
	author_avatar_url?: string | null;
	space_id?: string | null;
	space_name?: string | null;
	excerpt?: string;
	created_at: string;
	view_count?: number;
	comment_count?: number;
	vote_score?: number;
	velocity_score?: number;
	bookmark_count?: number;
	reading_time_minutes?: number;
	cover_image_url?: string | null;
	tags?: string[];
}
```

- [ ] **Step 2: Add the English i18n keys**

In `apps/main/src/lib/i18n/en.json`, add after the `"feed.tab.for_you"` line:

```json
	"feed.tab.explore": "Explore",
	"feed.end": "You're all caught up",
	"feed.min_read": "min read",
```

- [ ] **Step 3: Add the French i18n keys**

In `apps/main/src/lib/i18n/fr.json`, add the same keys with French values:

```json
	"feed.tab.explore": "Explorer",
	"feed.end": "Vous êtes à jour",
	"feed.min_read": "min de lecture",
```

- [ ] **Step 4: Verify types + JSON compile**

Run: `cd apps/main && npm run check`
Expected: no new svelte-check/type errors from these files (unused-key warnings are fine).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/types/index.ts apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(main): expand FeedItem fields + feed i18n keys

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Fetch the card-style config in the shell loader

**Files:**
- Modify: `apps/main/src/routes/(app)/+layout.server.ts`

**Interfaces:**
- Consumes: `apiGet` (`$lib/server/api`), `normalizeConfig` (Task 3).
- Produces: `data.cardStyleConfig: CardStyleConfig` for every `(app)` route.

- [ ] **Step 1: Add the graceful fetch**

Edit `apps/main/src/routes/(app)/+layout.server.ts`. Add imports:

```ts
import { normalizeConfig, type CardStyleConfig } from '$lib/feed/cardStyle';
```

Before the `return`, add:

```ts
	let cardStyleConfig: CardStyleConfig;
	try {
		cardStyleConfig = normalizeConfig(await apiGet<unknown>(ctx, '/feed/card-style'));
	} catch {
		cardStyleConfig = { default: 'reddit', byType: {} };
	}
```

Add `cardStyleConfig` to the returned object.

- [ ] **Step 2: Verify**

Run: `cd apps/main && npm run check`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/routes/(app)/+layout.server.ts
git commit -m "feat(main): load per-type card-style config in shell loader

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 6: Rewrite the Home page loader (tab/sort/params, fix ?limit=)

**Files:**
- Modify: `apps/main/src/routes/(app)/+page.server.ts`

**Interfaces:**
- Consumes: `apiList` (`$lib/server/api`), `feedPath`/`resolveTab`/`resolveSort`/`PAGE_SIZE` (Task 1).
- Produces: `data = { items, tab, sort, authed, pageSize, loadError }`.

- [ ] **Step 1: Replace the loader**

Replace the entire contents of `apps/main/src/routes/(app)/+page.server.ts` with:

```ts
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem } from '$lib/types';

/**
 * Home timeline. Tab + sort come from the URL (?tab=&sort=), so switching them
 * is a client-side navigation that re-runs this loader. Guests are pinned to
 * the public explore feed; authed users default to For You.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tab = resolveTab(event.url.searchParams.get('tab'), authed);
	const sort = resolveSort(event.url.searchParams.get('sort'));

	try {
		const { items } = await apiList<FeedItem>(ctx, feedPath(tab, sort, PAGE_SIZE, 0));
		return { items, tab, sort, authed, pageSize: PAGE_SIZE, loadError: false };
	} catch {
		return { items: [] as FeedItem[], tab, sort, authed, pageSize: PAGE_SIZE, loadError: true };
	}
};
```

- [ ] **Step 2: Verify**

Run: `cd apps/main && npm run check`
Expected: no new errors. (`+page.svelte` still references the old shape; it is replaced in Task 10 — a temporary type error there is acceptable until then, but this loader itself must type-check.)

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/routes/(app)/+page.server.ts
git commit -m "feat(main): URL-driven Home loader; page_size/offset + explicit sort

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 7: `FeedCard.svelte` — one item, resolved style × layout

**Files:**
- Create: `apps/main/src/lib/components/feed/FeedCard.svelte`

**Interfaces:**
- Consumes: `relativeTime` (Task 2), `CardStyle` (Task 3), `FeedItem` (Task 4), lucide icons.
- Produces: `<FeedCard item={FeedItem} style={CardStyle} layout={'stream' | 'grid-cell'} />`.

**Notes:** Shared pieces (avatar, badge, tags, metric row, cover) are Svelte snippets so the five render paths (grid-cell; stream reddit/compact/magazine/grid) don't duplicate them. Href: `blog_post` → `/blog/{slug}`, else `/content/{slug}`. Reference: spec §3A + mockup `e9dbc3d3`.

- [ ] **Step 1: Create the component**

Create `apps/main/src/lib/components/feed/FeedCard.svelte`:

```svelte
<script lang="ts">
	import { ArrowUp, ArrowDown, MessageSquare, Eye, Bookmark } from 'lucide-svelte';
	import { relativeTime } from '$lib/feed/time';
	import { t } from '$lib/i18n';
	import type { CardStyle } from '$lib/feed/cardStyle';
	import type { FeedItem } from '$lib/types';

	interface Props {
		item: FeedItem;
		style: CardStyle;
		layout: 'stream' | 'grid-cell';
	}
	let { item, style, layout }: Props = $props();

	const typeLabels: Record<string, string> = {
		blog_post: 'Blog',
		discussion: 'Discussion',
		question: 'Question',
		comparison: 'Comparison',
		page: 'Page'
	};
	const typeLabel = (ct: string) => typeLabels[ct] ?? ct.replace(/_/g, ' ');
	const href = (i: FeedItem) => (i.content_type === 'blog_post' ? `/blog/${i.slug}` : `/content/${i.slug}`);
	const time = $derived(relativeTime(item.created_at));
	const initials = $derived((item.author_name ?? '?').slice(0, 2).toUpperCase());
</script>

{#snippet avatar(size: number)}
	{#if item.author_avatar_url}
		<img src={item.author_avatar_url} alt="" loading="lazy" class="shrink-0 rounded-full object-cover" style="width:{size}px;height:{size}px" />
	{:else}
		<span class="grid shrink-0 place-items-center rounded-full bg-[var(--color-primary)] font-semibold text-[var(--color-primary-contrast)]" style="width:{size}px;height:{size}px;font-size:{Math.round(size * 0.4)}px">{initials}</span>
	{/if}
{/snippet}

{#snippet badge()}
	<span class="inline-block rounded border border-[var(--color-border)] bg-[var(--color-surface-subtle)] px-1.5 py-0.5 text-[10.5px] font-semibold uppercase tracking-wide text-[var(--color-text-subtle)]">{typeLabel(item.content_type)}</span>
{/snippet}

{#snippet tags()}
	{#if item.tags?.length}
		<div class="flex flex-wrap gap-1.5">
			{#each item.tags.slice(0, 3) as tag (tag)}
				<span class="text-xs font-medium text-[var(--color-primary)]">#{tag}</span>
			{/each}
		</div>
	{/if}
{/snippet}

{#snippet metrics()}
	<div class="flex items-center gap-4 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
		<span class="flex items-center gap-1"><ArrowUp size={14} /> {item.vote_score ?? 0}</span>
		<span class="flex items-center gap-1"><MessageSquare size={14} /> {item.comment_count ?? 0}</span>
		<span class="flex items-center gap-1"><Eye size={14} /> {item.view_count ?? 0}</span>
		{#if item.bookmark_count != null}<span class="flex items-center gap-1"><Bookmark size={14} /> {item.bookmark_count}</span>{/if}
	</div>
{/snippet}

{#snippet metaLine()}
	<div class="flex flex-wrap items-center gap-1.5 text-xs text-[var(--color-muted)]">
		{@render badge()}
		{#if item.space_name}<span class="font-semibold text-[var(--color-text-subtle)]">{item.space_name}</span>{/if}
		{#if item.author_name}<span>·</span><span>{item.author_name}</span>{/if}
		{#if time}<span>·</span><span>{time}</span>{/if}
	</div>
{/snippet}

{#if layout === 'grid-cell' || style === 'grid'}
	<!-- grid card (2-col cell) OR full-width grid item in a stream -->
	<a href={href(item)} class="flex flex-col overflow-hidden {layout === 'grid-cell' ? 'rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]' : ''} hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		{#if item.cover_image_url}
			<img src={item.cover_image_url} alt="" loading="lazy" class="aspect-video w-full object-cover" />
		{/if}
		<div class="flex flex-1 flex-col gap-2 p-3">
			{@render badge()}
			<h2 class="line-clamp-2 text-[15px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.title}</h2>
			<div class="mt-auto flex items-center gap-2 text-xs text-[var(--color-muted)]">
				{@render avatar(20)}
				<span>{item.author_name ?? ''}{#if time} · {time}{/if}</span>
			</div>
			{@render metrics()}
		</div>
	</a>
{:else if style === 'compact'}
	<a href={href(item)} class="flex items-center gap-3 px-4 py-2 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		<span class="flex w-11 shrink-0 items-center gap-1 text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]"><ArrowUp size={13} class="text-[var(--color-muted)]" /> {item.vote_score ?? 0}</span>
		<span class="min-w-0 flex-1 truncate text-sm font-medium text-[var(--color-text)]">{item.title}</span>
		{#if item.space_name}<span class="hidden shrink-0 text-xs text-[var(--color-muted)] sm:inline">{item.space_name}{#if time} · {time}{/if}</span>{/if}
		<span class="flex shrink-0 items-center gap-3 text-xs text-[var(--color-muted)] [font-variant-numeric:tabular-nums]">
			<span class="flex items-center gap-1"><MessageSquare size={13} /> {item.comment_count ?? 0}</span>
			<span class="flex items-center gap-1"><Eye size={13} /> {item.view_count ?? 0}</span>
		</span>
	</a>
{:else if style === 'magazine'}
	<a href={href(item)} class="block px-5 py-5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_35%,transparent)]">
		<div class="mb-3.5 flex items-center gap-2.5">
			{@render avatar(34)}
			<span class="flex flex-col leading-tight">
				<span class="text-[13.5px] font-semibold text-[var(--color-text)]">{item.author_name ?? ''}</span>
				<span class="text-xs text-[var(--color-muted)]">{#if item.space_name}{item.space_name} · {/if}{time}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</span>
			</span>
		</div>
		{#if item.cover_image_url}
			<img src={item.cover_image_url} alt="" loading="lazy" class="mb-3.5 aspect-video w-full rounded-[var(--radius-card)] border border-[var(--color-border)] object-cover" />
		{/if}
		{@render badge()}
		<h2 class="mt-2 mb-2 text-balance font-serif text-[25px] font-medium leading-tight tracking-tight text-[var(--color-text)]">{item.title}</h2>
		{#if item.excerpt}<p class="mb-3.5 line-clamp-2 text-[15px] leading-relaxed text-[var(--color-text-subtle)]">{item.excerpt}</p>{/if}
		<div class="flex flex-wrap items-center justify-between gap-3.5">{@render tags()}{@render metrics()}</div>
	</a>
{:else}
	<!-- reddit (default): vote rail + body + thumbnail -->
	<a href={href(item)} class="flex gap-3 px-4 py-3.5 hover:bg-[color-mix(in_srgb,var(--color-surface-subtle)_45%,transparent)]">
		<span class="flex w-7 shrink-0 flex-col items-center gap-0.5 pt-0.5 text-[var(--color-muted)]">
			<ArrowUp size={18} />
			<span class="text-xs font-semibold text-[var(--color-text-subtle)] [font-variant-numeric:tabular-nums]">{item.vote_score ?? 0}</span>
			<ArrowDown size={18} />
		</span>
		<span class="min-w-0 flex-1">
			{@render metaLine()}
			<h2 class="mb-0.5 mt-1 text-[15.5px] font-semibold leading-snug tracking-tight text-[var(--color-text)]">{item.title}</h2>
			{#if item.excerpt}<p class="mb-2 line-clamp-1 text-[13.5px] text-[var(--color-text-subtle)]">{item.excerpt}{#if item.reading_time_minutes} · {item.reading_time_minutes} {t('feed.min_read', 'min read')}{/if}</p>{/if}
			<div class="flex items-center justify-between gap-3">{@render tags()}{@render metrics()}</div>
		</span>
		{#if item.cover_image_url}
			<img src={item.cover_image_url} alt="" loading="lazy" class="h-[84px] w-[84px] shrink-0 rounded-[var(--radius)] object-cover" />
		{/if}
	</a>
{/if}
```

- [ ] **Step 2: Verify types + build**

Run: `cd apps/main && npm run check`
Expected: no errors in `FeedCard.svelte`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/feed/FeedCard.svelte
git commit -m "feat(main): FeedCard with four styles x stream/grid-cell layouts

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 8: `FeedList.svelte` — layout mode + infinite scroll

**Files:**
- Create: `apps/main/src/lib/components/feed/FeedList.svelte`

**Interfaces:**
- Consumes: `feedPath`/`FeedTab`/`SortValue` (Task 1), `cardStyleFor`/`gridMode`/`CardStyleConfig` (Task 3), `FeedItem` (Task 4), `FeedCard` (Task 7), browser `api` (`$lib/api/client`), `EmptyState`/`LoadingSkeleton` (`$lib/components/ui`), `navigating` (`$app/state`).
- Produces: `<FeedList items tab sort pageSize config loadError />`.

- [ ] **Step 1: Create the component**

Create `apps/main/src/lib/components/feed/FeedList.svelte`:

```svelte
<script lang="ts">
	import { untrack } from 'svelte';
	import { navigating } from '$app/state';
	import { TriangleAlert } from 'lucide-svelte';
	import { api } from '$lib/api/client';
	import { feedPath, type FeedTab, type SortValue } from '$lib/feed/endpoint';
	import { cardStyleFor, gridMode, type CardStyleConfig } from '$lib/feed/cardStyle';
	import { EmptyState, LoadingSkeleton } from '$lib/components/ui';
	import { t } from '$lib/i18n';
	import type { FeedItem } from '$lib/types';
	import FeedCard from './FeedCard.svelte';

	interface Props {
		items: FeedItem[];
		tab: FeedTab;
		sort: SortValue;
		pageSize: number;
		config: CardStyleConfig;
		loadError: boolean;
	}
	let { items: initial, tab, sort, pageSize, config, loadError }: Props = $props();

	let items = $state<FeedItem[]>(untrack(() => initial));
	let atEnd = $state(false);
	let loading = $state(false);
	let sentinel = $state<HTMLElement | null>(null);

	// Re-seed on a new SSR payload (tab/sort change) — replace, never append.
	$effect(() => {
		const seed = initial;
		untrack(() => {
			items = seed;
			atEnd = seed.length < pageSize;
			loading = false;
		});
	});

	// Layout mode is fixed from the initial SSR page (never flips mid-scroll).
	const gridLayout = $derived(gridMode(initial.map((i) => i.content_type), config));

	$effect(() => {
		const el = sentinel;
		if (!el || atEnd) return;
		const obs = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting) loadMore();
			},
			{ rootMargin: '600px' }
		);
		obs.observe(el);
		return () => obs.disconnect();
	});

	async function loadMore() {
		if (loading || atEnd) return;
		loading = true;
		const res = await api.get<FeedItem[]>(feedPath(tab, sort, pageSize, items.length));
		if (res.error) {
			loading = false;
			return;
		}
		const batch = res.data ?? [];
		items = [...items, ...batch];
		atEnd = batch.length < pageSize;
		loading = false;
	}
</script>

{#if loadError}
	<EmptyState icon={TriangleAlert} title={t('error.500_title')} body={t('error.500_body')} />
{:else if items.length === 0}
	<EmptyState title={t('empty.default_title')} body={t('empty.default_body')} />
{:else}
	{#if navigating.to}
		<LoadingSkeleton variant="card" count={3} class="p-2" />
	{/if}

	{#if gridLayout}
		<div class="grid grid-cols-1 gap-3.5 p-3 sm:grid-cols-2">
			{#each items as item (item.id)}
				<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="grid-cell" />
			{/each}
		</div>
	{:else}
		<div
			class="divide-y divide-[var(--color-border)] overflow-hidden rounded-[var(--radius-card)] border border-[var(--color-border)] bg-[var(--color-surface)]"
		>
			{#each items as item (item.id)}
				<FeedCard {item} style={cardStyleFor(item.content_type, config)} layout="stream" />
			{/each}
		</div>
	{/if}

	{#if !atEnd}
		<div bind:this={sentinel} class="h-8" aria-hidden="true"></div>
		{#if loading}<LoadingSkeleton variant="lines" count={2} class="p-4" />{/if}
	{:else}
		<p class="py-6 text-center text-xs text-[var(--color-muted)]">{t('feed.end', "You're all caught up")}</p>
	{/if}
{/if}
```

- [ ] **Step 2: Verify**

Run: `cd apps/main && npm run check`
Expected: no errors in `FeedList.svelte`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/feed/FeedList.svelte
git commit -m "feat(main): FeedList with grid/stream layout mode + infinite scroll

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 9: `FeedTabs.svelte` + `FeedSortBar.svelte` — anchor-link controls

**Files:**
- Create: `apps/main/src/lib/components/feed/FeedTabs.svelte`
- Create: `apps/main/src/lib/components/feed/FeedSortBar.svelte`

**Interfaces:**
- Consumes: `FeedTab`/`SortValue` (Task 1), `t` (`$lib/i18n`).
- Produces: `<FeedTabs tab sort authed />`, `<FeedSortBar tab sort />`.

- [ ] **Step 1: Create FeedTabs**

Create `apps/main/src/lib/components/feed/FeedTabs.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import type { FeedTab, SortValue } from '$lib/feed/endpoint';

	interface Props {
		tab: FeedTab;
		sort: SortValue;
		authed: boolean;
	}
	let { tab, sort, authed }: Props = $props();

	const tabs = $derived(
		authed
			? [
					{ value: 'for_you' as const, label: t('feed.tab.for_you') },
					{ value: 'following' as const, label: t('feed.tab.following') }
				]
			: [{ value: 'explore' as const, label: t('feed.tab.explore', 'Explore') }]
	);
</script>

<div
	class="sticky top-0 z-10 flex gap-1 border-b border-[var(--color-border)] bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-3 backdrop-blur"
	style="min-height: var(--nav-height)"
>
	{#each tabs as tabItem (tabItem.value)}
		<a
			href="?tab={tabItem.value}&sort={sort}"
			data-sveltekit-noscroll
			aria-current={tab === tabItem.value ? 'page' : undefined}
			class="relative flex items-center px-4 text-sm font-medium transition-colors {tab === tabItem.value
				? 'text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:text-[var(--color-text)]'}"
		>
			{tabItem.label}
			{#if tab === tabItem.value}
				<span class="absolute inset-x-3 -bottom-px h-0.5 rounded-full bg-[var(--color-primary)]"></span>
			{/if}
		</a>
	{/each}
</div>
```

- [ ] **Step 2: Create FeedSortBar**

Create `apps/main/src/lib/components/feed/FeedSortBar.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import type { FeedTab, SortValue } from '$lib/feed/endpoint';

	interface Props {
		tab: FeedTab;
		sort: SortValue;
	}
	let { tab, sort }: Props = $props();

	// Note the label→value mapping: "Popular" → top.
	const chips: { value: SortValue; label: string }[] = [
		{ value: 'relevant', label: t('sort.relevant') },
		{ value: 'latest', label: t('sort.latest') },
		{ value: 'top', label: t('sort.popular') },
		{ value: 'rising', label: t('sort.rising') }
	];
</script>

<div
	class="sticky z-10 flex items-center gap-2 bg-[color-mix(in_srgb,var(--color-bg)_90%,transparent)] px-4 py-2 backdrop-blur"
	style="top: var(--nav-height)"
>
	{#each chips as chip (chip.value)}
		<a
			href="?tab={tab}&sort={chip.value}"
			data-sveltekit-noscroll
			aria-current={sort === chip.value ? 'true' : undefined}
			class="rounded-full px-3 py-1 text-xs font-medium transition-colors {sort === chip.value
				? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]'
				: 'text-[var(--color-muted)] hover:bg-[var(--color-surface-subtle)]'}"
		>
			{chip.label}
		</a>
	{/each}
</div>
```

- [ ] **Step 3: Verify**

Run: `cd apps/main && npm run check`
Expected: no errors in the two new components.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/components/feed/FeedTabs.svelte apps/main/src/lib/components/feed/FeedSortBar.svelte
git commit -m "feat(main): FeedTabs + FeedSortBar as URL anchor-link controls

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 10: Compose the Home page

**Files:**
- Modify: `apps/main/src/routes/(app)/+page.svelte` (replace entirely)

**Interfaces:**
- Consumes: `FeedTabs`/`FeedSortBar`/`FeedList` (Tasks 8-9), layout `cardStyleConfig` via `page.data`, page `data` (Task 6).

- [ ] **Step 1: Replace the page**

Replace the entire contents of `apps/main/src/routes/(app)/+page.svelte` with:

```svelte
<script lang="ts">
	import { page } from '$app/state';
	import FeedTabs from '$lib/components/feed/FeedTabs.svelte';
	import FeedSortBar from '$lib/components/feed/FeedSortBar.svelte';
	import FeedList from '$lib/components/feed/FeedList.svelte';
	import type { CardStyleConfig } from '$lib/feed/cardStyle';
	import type { PageData } from './$types';

	let { data }: { data: PageData } = $props();

	// cardStyleConfig comes from the (app) layout loader (shared shell data).
	const config = $derived((page.data.cardStyleConfig as CardStyleConfig) ?? { default: 'reddit', byType: {} });
</script>

<div class="mx-auto w-full max-w-2xl">
	<FeedTabs tab={data.tab} sort={data.sort} authed={data.authed} />
	<FeedSortBar tab={data.tab} sort={data.sort} />
	<div class="p-3">
		<FeedList
			items={data.items}
			tab={data.tab}
			sort={data.sort}
			pageSize={data.pageSize}
			{config}
			loadError={data.loadError}
		/>
	</div>
</div>
```

- [ ] **Step 2: Verify the whole app type-checks and builds**

Run: `cd apps/main && npm run check && npm run build`
Expected: svelte-check reports 0 errors; build succeeds.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/routes/(app)/+page.svelte
git commit -m "feat(main): compose Home feed (tabs + sort + list); drop placeholder markup

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 11: Full-suite verification + API-Readiness Gate

**Files:** none (verification only).

- [ ] **Step 1: Run the unit suite**

Run: `cd apps/main && npx vitest run`
Expected: all `endpoint`/`time`/`cardStyle` tests PASS.

- [ ] **Step 2: Type + build gate**

Run: `cd apps/main && npm run check && npm run build`
Expected: 0 svelte-check errors; build succeeds.

- [ ] **Step 3: API-Readiness Gate (against a running backend)**

With the backend + `apps/main` dev server running and an authed session, confirm end-to-end (curl or browser dev-tools Network):
- `/feed/for-you?sort=relevant&page_size=20&page_offset=0` returns `{ data: FeedItem[] }` (200 authed, 401 unauth).
- Switching a sort chip issues a new request with the right `sort` (`Popular` → `sort=top`) and `page_size`/`page_offset` (never `limit`).
- Scrolling to the bottom fires a `page_offset=<n>` request and appends.
- `GET /feed/card-style` returns `{ data: { default, by_type } }`; setting `by_type.blog_post = "grid"` (via SQL) makes blog posts render as full-width image cards in the mixed feed, and a hand-made all-blog page renders 2-col.
- A card with the backend's non-ISO `created_at` shows a relative time (e.g. "2h"), not "Invalid Date".

- [ ] **Step 4: Commit any fixes**

If the gate surfaced fixes, commit them:

```bash
git add -A
git commit -m "fix(main): API-Readiness Gate corrections for feed frontend

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Acceptance (Layer 2 done)

- Home renders For You / Following (authed) or Explore (guest); tabs and sort are shareable URL params; switching them re-runs the loader with a loading state.
- All requests use `page_size`/`page_offset`/explicit `sort`; `Popular` maps to `top`; no `?limit=`.
- Infinite scroll appends and stops cleanly at the feed end.
- Card style is resolved per content type; a mixed feed with `blog_post → grid` shows blog posts as full-width image cards; an all-grid page is 2-col.
- Non-ISO timestamps render safely.
- `npx vitest run`, `npm run check`, and `npm run build` are all green.
