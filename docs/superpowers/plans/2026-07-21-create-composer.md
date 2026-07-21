# /create Composer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Also load the project skill **jc-main-develop** (shell/design rules), and the **tiptap** and **svelte** skills for Task groups 4–5.

**Goal:** Build the full `/create` authoring surface (ADR-058 Exception #2) for blog + custom post types + comparisons, with a debounced-autosave state machine, four TipTap triggers, and a complete Settings/SEO panel.

**Architecture:** One `CreateShell` owns the Exception-#2 layout, a `ComposerState` autosave state machine, and the Save/Publish bar. A pure `contentTypeRegistry` maps each content type to `{ formKind, bodyEditor, endpoints, rules }`; blog + CPTs share the TipTap `PostEditor` on the ungated `POST /content` path, comparisons use a distinct `ComparisonBuilder` on `POST /comparisons`. Custom fields ride inline in the `content.field_values` JSONB (atomic with the draft); tags and series are deferred linkages that flush after the draft first exists.

**Tech Stack:** SvelteKit 2 (adapter-node, Bun), Svelte 5 strict Runes, Bits UI (dot-notation), Tailwind v4 (CSS custom properties), TipTap (ProseMirror), Vitest 4 + @testing-library/svelte 5 + jsdom.

**Spec:** `docs/superpowers/specs/2026-07-21-create-composer-design.md` (read it before starting).

## Global Constraints

- **App root:** all paths below are under `apps/main/` unless noted. Backend is `http://localhost:8080`, all API paths under `/api/v1`.
- **Svelte 5 strict Runes only:** `$state`, `$derived`, `$props`, `$effect`. FORBIDDEN: `export let`, `$:`, `on:click` (use `onclick`), `createEventDispatcher`, `localStorage`/`sessionStorage` for any state.
- **Bits UI** dot-notation for every interactive primitive: `Select.*`, `Tabs.*`, `Popover.*`, `Dialog.*`.
- **Tailwind v4:** utilities via `class`; colors ONLY via `var(--color-*)` / `var(--chart-*)`; NEVER hardcode hex; dark mode via `[data-theme]`, never Tailwind `dark:`.
- **i18n:** every user-facing string via `t('key')` (`import { t } from '$lib/i18n'`); every new key added to BOTH `src/lib/i18n/en.json` AND `src/lib/i18n/fr.json`.
- **Rule 16:** all data from `/api/v1`; `load()` never returns static data. Server reads use `$lib/server/api` (`apiContext`, `apiGet`, `apiList`); client mutations use `$lib/api/client` (`api.post/patch/delete`, returns `{ data, error, status }`).
- **TypeScript:** zero `any`. Add shared types to `src/lib/types/index.ts`.
- **No AI attribution** in code comments or commit messages (beyond the repo's standard `Co-Authored-By` trailer).
- **Content-type constants (verified):** `blog_post`, `article`, `guide` (standard form) + `comparison` (builder). Status default `draft`; visibility default `public`.
- **Testing baseline:** all five frontend suites are green; any new failure is a regression. `bun run test` + `bun run check` must pass before the final task closes.
- **Commit discipline:** commit after each task on `main` directly (repo convention: a cron auto-commits the tree; `git add` only the task's files, never `-A`).

---

## File Structure

```
src/lib/types/index.ts ................ (MODIFY) add composer domain + transport types
src/lib/api/client.ts ................. (existing) client mutation wrapper — reused
src/lib/api/create.ts ................. (CREATE) typed client wrappers for all composer calls + classifyError
src/lib/components/create/
  contentTypeRegistry.ts .............. (CREATE) pure type→editor/endpoint/rules map
  composerState.svelte.ts ............. (CREATE) ComposerState autosave state machine + linkage reconciler
  slug.ts ............................. (CREATE) pure slug suggestion helper
  CreateShell.svelte .................. (CREATE) Exception-#2 layout host; owns ComposerState
  SaveBar.svelte ...................... (CREATE) sticky status pill + Save draft + Publish
  editors/
    PostEditor.svelte ................. (CREATE) TipTap host (blog/CPT body)
    ComparisonBuilder.svelte ......... (CREATE) items × dimensions grid editor
  fields/
    CoverUploader.svelte ............. (CREATE) 2-step media upload
    TitleField.svelte ................ (CREATE) autosizing title → slug suggestion
    TagInput.svelte .................. (CREATE) tag chips + autocomplete
    SpaceSelector.svelte ............. (CREATE) required primary-space picker
  panel/
    CreateRightPanel.svelte .......... (CREATE) [Guide|Settings] Bits UI Tabs host
    CreateGuidePanel.svelte .......... (CREATE) focus-reactive help
    CreateSettingsPanel.svelte ....... (CREATE) visibility/schedule/canonical/series/custom-fields/SEO
    SeriesSelector.svelte ............ (CREATE) existing-or-create-new series
    CustomFieldGroup.svelte .......... (CREATE) renders one custom-field group → field_values
src/lib/tiptap/
  search.ts ........................... (CREATE) pure query-shaping utils for @ # [[ triggers
  slashMenu.ts ........................ (CREATE) slash-command block list (config)
  extensions.ts ....................... (CREATE) buildExtensions() assembling TipTap + suggestion plugins
src/routes/(app)/create/
  +page.ts ............................ (CREATE) exception-layout flag (client)
  +page.server.ts ..................... (REPLACE stub) loader: types∩flags, spaces, field-groups, series, draft
  +page.svelte ........................ (REPLACE stub) hosts CreateShell
src/routes/(app)/+layout.svelte ....... (MODIFY) collapse nav + hide RightSidebar on exception routes
src/lib/i18n/en.json, fr.json ......... (MODIFY) add create.* keys
```

---

## Phase 0 — Domain types & pure logic

### Task 1: Composer domain & transport types

**Files:**
- Modify: `src/lib/types/index.ts` (append a new section)

**Interfaces:**
- Produces: `ContentTypeMeta`, `MeSpace`, `CustomFieldDef`, `CustomFieldGroup`, `BlogSeries`, `ContentRecord`, `ComposerStatus`, `SaveErrorClass`, `Visibility`, `MediaAsset`, `SearchHit`.

- [ ] **Step 1: Add the types** (no test — pure type declarations; validated by `bun run check` and downstream tasks)

Append to `src/lib/types/index.ts`:

```ts
/* ------------------------------------------------------------------ *
 * Composer (/create) — Section 14/24/30
 * ------------------------------------------------------------------ */

export type Visibility = 'public' | 'unlisted' | 'private';

/** GET /content-types item. */
export interface ContentTypeMeta {
	value: string;
	label: string;
}

/** GET /me/spaces item (the caller's joined spaces). */
export interface MeSpace {
	id: string;
	slug: string;
	name: string;
	is_primary?: boolean;
}

/** One custom-field definition (from GET /custom-field-groups/{id}). */
export interface CustomFieldDef {
	id: string;
	key: string;
	label: string;
	field_type: string; // text|textarea|rich_text|number|boolean|date|select|multiselect|rating|url|...
	is_required: boolean;
	options?: string[] | null;
	help_text?: string | null;
}

/** A custom-field group scoped to an entity type (the composer keeps entity_type === "content"). */
export interface CustomFieldGroup {
	id: string;
	name: string;
	entity_type: string;
	fields: CustomFieldDef[];
}

/** GET /blog/series item. */
export interface BlogSeries {
	id: string;
	slug: string;
	title: string;
}

/** Content record returned by POST /content and PATCH /content/{id}. */
export interface ContentRecord {
	id: string;
	slug: string;
	content_type: string;
	title: string;
	body?: string;
	body_html?: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string | null;
	status: string;
	visibility: string;
	publish_at?: string | null;
	space_id?: string | null;
}

/** Uploaded media (POST /media/upload → confirm). */
export interface MediaAsset {
	id: string;
	url: string;
}

/** One autocomplete result for @ / [[ / # triggers. */
export interface SearchHit {
	id: string;
	label: string; // display name / title / tag name
	slug?: string;
	handle?: string;
	avatar_url?: string | null;
}

/** Autosave/publish lifecycle state. */
export type ComposerStatus =
	| 'pristine'
	| 'creating'
	| 'saved'
	| 'dirty'
	| 'saving'
	| 'published'
	| 'scheduled'
	| 'error';

/** How a failed save should be handled. */
export type SaveErrorClass = 'validation' | 'server' | 'network' | 'conflict';
```

- [ ] **Step 2: Verify types compile**

Run: `cd apps/main && bun run check`
Expected: PASS (no new errors introduced by this file).

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/types/index.ts
git commit -m "feat(main): composer domain + transport types"
```

---

### Task 2: Slug suggestion helper (pure, TDD)

**Files:**
- Create: `src/lib/components/create/slug.ts`
- Test: `src/lib/components/create/slug.test.ts`

**Interfaces:**
- Produces: `suggestSlug(title: string): string`

- [ ] **Step 1: Write the failing test**

`src/lib/components/create/slug.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { suggestSlug } from './slug';

describe('suggestSlug', () => {
	it('lowercases and hyphenates', () => {
		expect(suggestSlug('Hello World')).toBe('hello-world');
	});
	it('strips punctuation and collapses separators', () => {
		expect(suggestSlug('  Study in the UK: A Guide!! ')).toBe('study-in-the-uk-a-guide');
	});
	it('keeps digits and drops diacritics-adjacent symbols', () => {
		expect(suggestSlug('Top 10 — Universities')).toBe('top-10-universities');
	});
	it('returns empty string for empty/symbol-only input', () => {
		expect(suggestSlug('   ')).toBe('');
		expect(suggestSlug('!!!')).toBe('');
	});
	it('truncates to 80 chars without a trailing hyphen', () => {
		const long = 'a'.repeat(100);
		const out = suggestSlug(long);
		expect(out.length).toBeLessThanOrEqual(80);
		expect(out.endsWith('-')).toBe(false);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test slug`
Expected: FAIL ("suggestSlug is not a function" / cannot find module).

- [ ] **Step 3: Write minimal implementation**

`src/lib/components/create/slug.ts`:

```ts
/** Derive a URL slug from a human title. Pure; safe to call on every keystroke. */
export function suggestSlug(title: string): string {
	const base = title
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-') // any run of non-alphanumerics → single hyphen
		.replace(/^-+|-+$/g, ''); // trim leading/trailing hyphens
	if (base.length <= 80) return base;
	return base.slice(0, 80).replace(/-+$/g, '');
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test slug`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/create/slug.ts apps/main/src/lib/components/create/slug.test.ts
git commit -m "feat(main): pure slug suggestion helper"
```

---

### Task 3: Content-type registry (pure, TDD)

**Files:**
- Create: `src/lib/components/create/contentTypeRegistry.ts`
- Test: `src/lib/components/create/contentTypeRegistry.test.ts`

**Interfaces:**
- Consumes: `ContentTypeMeta` (Task 1).
- Produces:
  - `type FormKind = 'standard' | 'comparison'`
  - `interface TypeEntry { value: string; label: string; formKind: FormKind; supportsSeries: boolean; customFieldEntityType: 'content'; featureFlag: string | null }`
  - `const COMPOSER_TYPES: Record<string, TypeEntry>` (keys: blog_post, article, guide, comparison)
  - `resolveType(value: string): TypeEntry` (unknown → blog_post entry)
  - `availableTypes(types: ContentTypeMeta[], flags: Record<string, boolean>): TypeEntry[]`

- [ ] **Step 1: Write the failing test**

`src/lib/components/create/contentTypeRegistry.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { resolveType, availableTypes, COMPOSER_TYPES } from './contentTypeRegistry';

describe('resolveType', () => {
	it('maps blog_post/article/guide to the standard form', () => {
		for (const v of ['blog_post', 'article', 'guide']) {
			expect(resolveType(v).formKind).toBe('standard');
		}
	});
	it('maps comparison to the comparison form and its feature flag', () => {
		expect(resolveType('comparison').formKind).toBe('comparison');
		expect(resolveType('comparison').featureFlag).toBe('feature.comparison');
	});
	it('falls back to blog_post for unknown types', () => {
		expect(resolveType('nonsense')).toEqual(COMPOSER_TYPES.blog_post);
	});
	it('only standard blog-family types support series', () => {
		expect(resolveType('blog_post').supportsSeries).toBe(true);
		expect(resolveType('comparison').supportsSeries).toBe(false);
	});
});

describe('availableTypes', () => {
	const all = [
		{ value: 'discussion', label: 'Discussion' },
		{ value: 'blog_post', label: 'Blog Post' },
		{ value: 'page', label: 'Page' },
		{ value: 'comparison', label: 'Comparison' },
		{ value: 'question', label: 'Question' },
		{ value: 'article', label: 'Article' },
		{ value: 'guide', label: 'Guide' }
	];
	it('keeps only composer types, excluding discussion/question/page', () => {
		const vals = availableTypes(all, { 'feature.blog': true, 'feature.comparison': true }).map(
			(t) => t.value
		);
		expect(vals).toEqual(['blog_post', 'article', 'guide', 'comparison']);
	});
	it('hides comparison when feature.comparison is off', () => {
		const vals = availableTypes(all, { 'feature.blog': true, 'feature.comparison': false }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('comparison');
	});
	it('hides blog_post when feature.blog is off (respecting intent)', () => {
		const vals = availableTypes(all, { 'feature.blog': false, 'feature.comparison': true }).map(
			(t) => t.value
		);
		expect(vals).not.toContain('blog_post');
	});
	it('treats a missing flag as enabled for non-gated types (article/guide always show)', () => {
		const vals = availableTypes(all, {}).map((t) => t.value);
		expect(vals).toContain('article');
		expect(vals).toContain('guide');
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test contentTypeRegistry`
Expected: FAIL (module not found).

- [ ] **Step 3: Write minimal implementation**

`src/lib/components/create/contentTypeRegistry.ts`:

```ts
import type { ContentTypeMeta } from '$lib/types';

export type FormKind = 'standard' | 'comparison';

export interface TypeEntry {
	value: string;
	label: string;
	formKind: FormKind;
	supportsSeries: boolean;
	customFieldEntityType: 'content';
	/** Flag that must be ON for this type to appear; null = always available. */
	featureFlag: string | null;
}

export const COMPOSER_TYPES: Record<string, TypeEntry> = {
	blog_post: {
		value: 'blog_post',
		label: 'Blog post',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: 'feature.blog'
	},
	article: {
		value: 'article',
		label: 'Article',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	guide: {
		value: 'guide',
		label: 'Guide',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	comparison: {
		value: 'comparison',
		label: 'Comparison',
		formKind: 'comparison',
		supportsSeries: false,
		customFieldEntityType: 'content',
		featureFlag: 'feature.comparison'
	}
};

/** The order the selector presents composer types. */
const COMPOSER_ORDER = ['blog_post', 'article', 'guide', 'comparison'];

export function resolveType(value: string): TypeEntry {
	return COMPOSER_TYPES[value] ?? COMPOSER_TYPES.blog_post;
}

/**
 * Intersect the backend's content types with composer-supported types and the
 * tenant's feature flags. A gated type is shown only when its flag is truthy;
 * an ungated type (featureFlag === null) is always shown.
 */
export function availableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>
): TypeEntry[] {
	const offered = new Set(types.map((t) => t.value));
	return COMPOSER_ORDER.filter((v) => offered.has(v))
		.map((v) => COMPOSER_TYPES[v])
		.filter((entry) => entry.featureFlag === null || flags[entry.featureFlag] === true);
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test contentTypeRegistry`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/create/contentTypeRegistry.ts apps/main/src/lib/components/create/contentTypeRegistry.test.ts
git commit -m "feat(main): content-type registry (pure, feature-flag aware)"
```

---

### Task 4: Trigger search query-shaping utils (pure, TDD)

**Files:**
- Create: `src/lib/tiptap/search.ts`
- Test: `src/lib/tiptap/search.test.ts`

**Interfaces:**
- Consumes: `SearchHit` (Task 1).
- Produces:
  - `searchPath(kind: 'users' | 'content' | 'tags', query: string): string | null` (null when below min length)
  - `mapUserHits(raw: unknown): SearchHit[]`, `mapContentHits(raw: unknown): SearchHit[]`, `mapTagHits(raw: unknown): SearchHit[]`

- [ ] **Step 1: Write the failing test**

`src/lib/tiptap/search.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { searchPath, mapUserHits, mapContentHits, mapTagHits } from './search';

describe('searchPath', () => {
	it('returns null below the 2-char minimum', () => {
		expect(searchPath('users', 'a')).toBeNull();
		expect(searchPath('users', '')).toBeNull();
	});
	it('builds an encoded /search path for users and content', () => {
		expect(searchPath('users', 'jane d')).toBe('/search?type=users&q=jane%20d');
		expect(searchPath('content', 'uk unis')).toBe('/search?type=content&q=uk%20unis');
	});
	it('builds a /tags path for tag search', () => {
		expect(searchPath('tags', 'visa')).toBe('/tags?q=visa');
	});
});

describe('hit mappers', () => {
	it('maps user hits to {id,label,handle,avatar}', () => {
		const raw = [{ id: 'u1', display_name: 'Jane D', username: 'jane', avatar_url: 'x' }];
		expect(mapUserHits(raw)).toEqual([
			{ id: 'u1', label: 'Jane D', handle: 'jane', avatar_url: 'x' }
		]);
	});
	it('maps content hits to {id,label,slug}', () => {
		const raw = [{ id: 'c1', title: 'UK Unis', slug: 'uk-unis' }];
		expect(mapContentHits(raw)).toEqual([{ id: 'c1', label: 'UK Unis', slug: 'uk-unis' }]);
	});
	it('maps tag hits to {id,label,slug}', () => {
		const raw = [{ id: 't1', name: 'visa', slug: 'visa' }];
		expect(mapTagHits(raw)).toEqual([{ id: 't1', label: 'visa', slug: 'visa' }]);
	});
	it('returns [] for non-array input', () => {
		expect(mapUserHits(null)).toEqual([]);
		expect(mapContentHits({})).toEqual([]);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test tiptap/search`
Expected: FAIL (module not found).

- [ ] **Step 3: Write minimal implementation**

`src/lib/tiptap/search.ts`:

```ts
import type { SearchHit } from '$lib/types';

const MIN_QUERY = 2;

export function searchPath(kind: 'users' | 'content' | 'tags', query: string): string | null {
	const q = query.trim();
	if (q.length < MIN_QUERY) return null;
	const enc = encodeURIComponent(q);
	if (kind === 'tags') return `/tags?q=${enc}`;
	return `/search?type=${kind}&q=${enc}`;
}

function asArray(raw: unknown): Record<string, unknown>[] {
	return Array.isArray(raw) ? (raw as Record<string, unknown>[]) : [];
}

export function mapUserHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.display_name ?? r.username ?? ''),
		handle: r.username ? String(r.username) : undefined,
		avatar_url: (r.avatar_url as string | null | undefined) ?? null
	}));
}

export function mapContentHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.title ?? ''),
		slug: r.slug ? String(r.slug) : undefined
	}));
}

export function mapTagHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.name ?? ''),
		slug: r.slug ? String(r.slug) : undefined
	}));
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test tiptap/search`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/tiptap/search.ts apps/main/src/lib/tiptap/search.test.ts
git commit -m "feat(main): pure trigger search path + hit mappers"
```

---

### Task 5: API client wrappers + error classifier (TDD)

**Files:**
- Create: `src/lib/api/create.ts`
- Test: `src/lib/api/create.test.ts`

**Interfaces:**
- Consumes: `api` from `$lib/api/client`; `ContentRecord`, `MediaAsset`, `SaveErrorClass` (Task 1); `searchPath`, hit mappers (Task 4).
- Produces (all return the underlying `ClientResult<T>` unless noted):
  - `classifyError(status: number): SaveErrorClass`
  - `createContent(input: CreateContentInput)`, `patchContent(id, patch: Partial<CreateContentInput>)`
  - `publishContent(id)`, `scheduleContent(id, publishAt: string)`
  - `attachTag(id, tagId)`, `detachTag(id, tagId)`
  - `listSeries()`, `createSeries(input)`, `linkSeries(seriesId, contentId, position)`, `unlinkSeries(seriesId, contentId)`
  - `uploadMedia(file)`, `confirmMedia(id)`
  - `searchHits(kind, query)` → `Promise<SearchHit[]>`
  - `createComparison(input)`, `addComparisonItem`, `addComparisonDimension`, `setDimensionValues`, `patchComparison`
  - `type CreateContentInput = { content_type; title; slug?; body?; body_html?; field_values?; canonical_url?; status?; visibility?; publish_at?; space_id?; geo_space_id? }`

- [ ] **Step 1: Write the failing test** (classifier + one representative wrapper; the rest are thin passthroughs verified by check + integration)

`src/lib/api/create.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/api/client', () => ({
	api: { get: vi.fn(), post: vi.fn(), patch: vi.fn(), delete: vi.fn() }
}));

import { api } from '$lib/api/client';
import { classifyError, createContent, searchHits } from './create';

describe('classifyError', () => {
	it('classifies by status band', () => {
		expect(classifyError(0)).toBe('network');
		expect(classifyError(409)).toBe('conflict');
		expect(classifyError(422)).toBe('validation');
		expect(classifyError(400)).toBe('validation');
		expect(classifyError(500)).toBe('server');
		expect(classifyError(503)).toBe('server');
	});
});

describe('createContent', () => {
	beforeEach(() => vi.clearAllMocks());
	it('POSTs /content with the input body', async () => {
		(api.post as any).mockResolvedValue({ data: { id: 'c1', slug: 's' }, error: null, status: 201 });
		const res = await createContent({ content_type: 'blog_post', title: 'Hi' });
		expect(api.post).toHaveBeenCalledWith('/content', { content_type: 'blog_post', title: 'Hi' });
		expect(res.data).toEqual({ id: 'c1', slug: 's' });
	});
});

describe('searchHits', () => {
	beforeEach(() => vi.clearAllMocks());
	it('returns [] without calling the API below min length', async () => {
		const hits = await searchHits('users', 'a');
		expect(hits).toEqual([]);
		expect(api.get).not.toHaveBeenCalled();
	});
	it('maps user results on a valid query', async () => {
		(api.get as any).mockResolvedValue({
			data: [{ id: 'u1', display_name: 'Jane', username: 'jane' }],
			error: null,
			status: 200
		});
		const hits = await searchHits('users', 'jane');
		expect(api.get).toHaveBeenCalledWith('/search?type=users&q=jane');
		expect(hits[0]).toMatchObject({ id: 'u1', label: 'Jane', handle: 'jane' });
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test api/create`
Expected: FAIL (module not found).

- [ ] **Step 3: Write minimal implementation**

`src/lib/api/create.ts`:

```ts
import { api } from '$lib/api/client';
import type { ContentRecord, MediaAsset, SaveErrorClass, SearchHit, BlogSeries } from '$lib/types';
import { searchPath, mapUserHits, mapContentHits, mapTagHits } from '$lib/tiptap/search';

export interface CreateContentInput {
	content_type: string;
	title: string;
	slug?: string;
	body?: string;
	body_html?: string;
	field_values?: Record<string, unknown>;
	canonical_url?: string | null;
	status?: string;
	visibility?: string;
	publish_at?: string | null;
	space_id?: string | null;
	geo_space_id?: string | null;
}

/** Map an HTTP status to a save-failure class (drives retry vs surface behavior). */
export function classifyError(status: number): SaveErrorClass {
	if (status === 0) return 'network';
	if (status === 409) return 'conflict';
	if (status >= 500) return 'server';
	return 'validation'; // any other 4xx
}

/* ---- content lifecycle (standard form) ---- */
export const createContent = (input: CreateContentInput) =>
	api.post<ContentRecord>('/content', input);
export const patchContent = (id: string, patch: Partial<CreateContentInput>) =>
	api.patch<ContentRecord>(`/content/${id}`, patch);
export const publishContent = (id: string) => api.post<ContentRecord>(`/content/${id}/publish`);
export const scheduleContent = (id: string, publish_at: string) =>
	api.post<ContentRecord>(`/content/${id}/schedule`, { publish_at });

/* ---- tags (deferred linkage) ---- */
export const attachTag = (id: string, tagId: string) =>
	api.post(`/content/${id}/tags`, { tag_id: tagId });
export const detachTag = (id: string, tagId: string) => api.delete(`/content/${id}/tags/${tagId}`);

/* ---- series (deferred linkage; standard form only) ---- */
export const listSeries = () => api.get<BlogSeries[]>('/blog/series');
export const createSeries = (input: { title: string; slug: string; description?: string }) =>
	api.post<BlogSeries>('/blog/series', input);
export const linkSeries = (seriesId: string, contentId: string, position: number) =>
	api.post(`/blog/series/${seriesId}/posts`, { content_id: contentId, position });
export const unlinkSeries = (seriesId: string, contentId: string) =>
	api.delete(`/blog/series/${seriesId}/posts/${contentId}`);

/* ---- media (2-step) ---- */
export const uploadMedia = (file: File) => {
	const form = new FormData();
	form.append('file', file);
	// FormData sets its own content-type boundary; omit the JSON header.
	return api.post<MediaAsset & { upload_url?: string }>('/media/upload', undefined);
	// NOTE: implementer wires the multipart body per the media handler (see Task 12).
};
export const confirmMedia = (id: string) => api.post<MediaAsset>(`/media/confirm/${id}`);

/* ---- trigger search ---- */
export async function searchHits(
	kind: 'users' | 'content' | 'tags',
	query: string
): Promise<SearchHit[]> {
	const path = searchPath(kind, query);
	if (!path) return [];
	const res = await api.get<unknown>(path);
	if (res.error || res.data == null) return [];
	if (kind === 'users') return mapUserHits(res.data);
	if (kind === 'content') return mapContentHits(res.data);
	return mapTagHits(res.data);
}

/* ---- comparisons ---- */
export const createComparison = (input: { title: string; slug?: string; visibility?: string }) =>
	api.post<{ id: string; slug: string }>('/comparisons', input);
export const patchComparison = (id: string, patch: Record<string, unknown>) =>
	api.patch(`/comparisons/${id}`, patch);
export const addComparisonItem = (id: string, input: { label: string; position: number }) =>
	api.post(`/comparisons/${id}/items`, input);
export const addComparisonDimension = (id: string, input: { label: string; position: number }) =>
	api.post(`/comparisons/${id}/dimensions`, input);
export const setDimensionValues = (dimensionId: string, values: Record<string, unknown>) =>
	api.patch(`/comparisons/dimensions/${dimensionId}/values`, values);
```

> Implementer note: the exact `POST /media/upload` multipart contract and the tag-attach body key (`tag_id` vs `tag_slug`) must be confirmed against `internal/media` and `internal/content` handlers when Task 11/12 run; adjust the two flagged wrappers then. The classifier and content-lifecycle wrappers are the tested contract.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test api/create`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/api/create.ts apps/main/src/lib/api/create.test.ts
git commit -m "feat(main): composer API client wrappers + error classifier"
```

---

## Phase 1 — The autosave state machine (crown jewel)

### Task 6: `ComposerState` — lifecycle, single-flight, deferred linkage, publish gate (TDD)

**Files:**
- Create: `src/lib/components/create/composerState.svelte.ts`
- Test: `src/lib/components/create/composerState.svelte.test.ts`

**Interfaces:**
- Consumes: `ComposerStatus`, `SaveErrorClass` (Task 1); `CreateContentInput` (Task 5).
- Produces:
  - `interface ComposerDeps { create; patch; publish; schedule; attachTag; detachTag; linkSeries; unlinkSeries; classify }`
  - `interface ComposerOptions { getSnapshot; canPublish; getPublishAt; debounceMs?; maxRetries? }`
  - `class ComposerState` with reactive fields `status`, `contentId`, `lastError`, `savedAt`, `desiredTags`, `desiredSeries`, and methods `markDirty()`, `flush()`, `publish()`, `setTags(next)`, `setSeries(id)`, `dispose()`.

> **Why `.svelte.ts`:** the `.svelte.ts` extension makes Vitest compile Svelte 5 runes, so `$state` fields on this class work in tests without mounting a component. Testability was the reason the machine is a standalone module (spec §7).

- [ ] **Step 1: Write the failing test**

`src/lib/components/create/composerState.svelte.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ComposerState, type ComposerDeps, type ComposerOptions } from './composerState.svelte';

const ok = (data: unknown = {}, status = 200) => ({ data, error: null, status });
const fail = (status: number, message = 'x') => ({ data: null, error: { code: 'e', message, status }, status });

function makeDeps(over: Partial<ComposerDeps> = {}): ComposerDeps {
	return {
		create: vi.fn().mockResolvedValue(ok({ id: 'c1', slug: 's' }, 201)),
		patch: vi.fn().mockResolvedValue(ok({ id: 'c1' })),
		publish: vi.fn().mockResolvedValue(ok()),
		schedule: vi.fn().mockResolvedValue(ok()),
		attachTag: vi.fn().mockResolvedValue(ok()),
		detachTag: vi.fn().mockResolvedValue(ok()),
		linkSeries: vi.fn().mockResolvedValue(ok()),
		unlinkSeries: vi.fn().mockResolvedValue(ok()),
		classify: (s: number) => (s === 0 ? 'network' : s === 409 ? 'conflict' : s >= 500 ? 'server' : 'validation'),
		...over
	};
}

function makeState(deps: ComposerDeps, opts: Partial<ComposerOptions> = {}) {
	const options: ComposerOptions = {
		getSnapshot: () => ({ content_type: 'blog_post', title: 'Hi', body_html: '<p>x</p>' }),
		canPublish: () => true,
		getPublishAt: () => null,
		debounceMs: 1_000_000, // effectively disable auto-fire; tests call flush() directly
		maxRetries: 2,
		...opts
	};
	return new ComposerState(deps, options);
}

describe('ComposerState lifecycle', () => {
	let deps: ComposerDeps;
	beforeEach(() => (deps = makeDeps()));

	it('starts pristine', () => {
		expect(makeState(deps).status).toBe('pristine');
	});

	it('first flush creates content and lands in saved', async () => {
		const s = makeState(deps);
		s.markDirty();
		expect(s.status).toBe('dirty');
		await s.flush();
		expect(deps.create).toHaveBeenCalledTimes(1);
		expect(s.contentId).toBe('c1');
		expect(s.status).toBe('saved');
	});

	it('subsequent flush patches, never re-creates', async () => {
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		s.markDirty();
		await s.flush();
		expect(deps.create).toHaveBeenCalledTimes(1);
		expect(deps.patch).toHaveBeenCalledTimes(1);
	});

	it('single-flight: overlapping flushes create once', async () => {
		let resolve!: (v: unknown) => void;
		deps.create = vi.fn().mockReturnValue(new Promise((r) => (resolve = r)));
		const s = makeState(deps);
		s.markDirty();
		const a = s.flush();
		const b = s.flush(); // must not start a second create
		resolve(ok({ id: 'c1', slug: 's' }, 201));
		await Promise.all([a, b]);
		expect(deps.create).toHaveBeenCalledTimes(1);
	});
});

describe('ComposerState deferred linkage', () => {
	let deps: ComposerDeps;
	beforeEach(() => (deps = makeDeps()));

	it('holds tags chosen while pristine, flushes them after create', async () => {
		const s = makeState(deps);
		s.setTags(['t1', 't2']); // no content yet
		expect(deps.attachTag).not.toHaveBeenCalled();
		s.markDirty();
		await s.flush();
		expect(deps.attachTag).toHaveBeenCalledWith('c1', 't1');
		expect(deps.attachTag).toHaveBeenCalledWith('c1', 't2');
	});

	it('reconciles a series change after the draft exists (unlink old, link new)', async () => {
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		s.setSeries('sA');
		await Promise.resolve();
		s.setSeries('sB');
		await Promise.resolve();
		expect(deps.linkSeries).toHaveBeenCalledWith('sA', 'c1', 0);
		expect(deps.unlinkSeries).toHaveBeenCalledWith('sA', 'c1');
		expect(deps.linkSeries).toHaveBeenCalledWith('sB', 'c1', 0);
	});
});

describe('ComposerState publish gate + failure classes', () => {
	it('refuses to publish when canPublish() is false', async () => {
		const deps = makeDeps();
		const s = makeState(deps, { canPublish: () => false });
		s.markDirty();
		await s.flush();
		const okPublished = await s.publish();
		expect(okPublished).toBe(false);
		expect(deps.publish).not.toHaveBeenCalled();
	});

	it('publishes when valid; schedules when a publish_at is set', async () => {
		const deps = makeDeps();
		const s = makeState(deps, { getPublishAt: () => '2099-01-01T00:00:00Z' });
		s.markDirty();
		await s.flush();
		await s.publish();
		expect(deps.schedule).toHaveBeenCalledWith('c1', '2099-01-01T00:00:00Z');
		expect(deps.publish).not.toHaveBeenCalled();
		expect(s.status).toBe('scheduled');
	});

	it('a 422 create failure is validation → error, no retry', async () => {
		const deps = makeDeps({ create: vi.fn().mockResolvedValue(fail(422, 'bad title')) });
		const s = makeState(deps);
		s.markDirty();
		await s.flush();
		expect(s.status).toBe('error');
		expect(s.lastError?.cls).toBe('validation');
		expect(deps.create).toHaveBeenCalledTimes(1);
	});

	it('a 500 create failure is server → keeps buffer, schedules a retry', async () => {
		vi.useFakeTimers();
		const deps = makeDeps({ create: vi.fn().mockResolvedValue(fail(503)) });
		const s = makeState(deps, { debounceMs: 10 });
		s.markDirty();
		await s.flush();
		expect(s.lastError?.cls).toBe('server');
		expect(s.status).toBe('dirty'); // queued for retry, not error
		deps.create = vi.fn().mockResolvedValue(ok({ id: 'c1', slug: 's' }, 201));
		await vi.advanceTimersByTimeAsync(20);
		expect(s.contentId).toBe('c1');
		vi.useRealTimers();
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test composerState`
Expected: FAIL (module not found).

- [ ] **Step 3: Write the implementation**

`src/lib/components/create/composerState.svelte.ts`:

```ts
import type { ComposerStatus, SaveErrorClass } from '$lib/types';
import type { CreateContentInput } from '$lib/api/create';

type Res<T = unknown> = { data: T | null; error: { message: string; status: number } | null; status: number };

export interface ComposerDeps {
	create: (input: CreateContentInput) => Promise<Res<{ id: string; slug: string }>>;
	patch: (id: string, patch: Partial<CreateContentInput>) => Promise<Res>;
	publish: (id: string) => Promise<Res>;
	schedule: (id: string, at: string) => Promise<Res>;
	attachTag: (id: string, tagId: string) => Promise<Res>;
	detachTag: (id: string, tagId: string) => Promise<Res>;
	linkSeries: (seriesId: string, contentId: string, position: number) => Promise<Res>;
	unlinkSeries: (seriesId: string, contentId: string) => Promise<Res>;
	classify: (status: number) => SaveErrorClass;
}

export interface ComposerOptions {
	getSnapshot: () => CreateContentInput;
	canPublish: () => boolean;
	getPublishAt: () => string | null;
	debounceMs?: number;
	maxRetries?: number;
}

export class ComposerState {
	status = $state<ComposerStatus>('pristine');
	contentId = $state<string | null>(null);
	lastError = $state<{ cls: SaveErrorClass; message: string } | null>(null);
	savedAt = $state<number | null>(null);
	desiredTags = $state<string[]>([]);
	desiredSeries = $state<string | null>(null);

	#deps: ComposerDeps;
	#opts: Required<ComposerOptions>;
	#rev = 0;
	#savedRev = 0;
	#inFlight = false;
	#retries = 0;
	#timer: ReturnType<typeof setTimeout> | null = null;
	#committedTags: string[] = [];
	#committedSeries: string | null = null;

	constructor(deps: ComposerDeps, opts: ComposerOptions) {
		this.#deps = deps;
		this.#opts = { debounceMs: 2000, maxRetries: 3, ...opts };
	}

	/** Call on any body/field edit. Bumps the revision and schedules a debounced save. */
	markDirty(): void {
		this.#rev++;
		if (this.status !== 'creating' && this.status !== 'saving') this.status = 'dirty';
		this.#schedule(this.#opts.debounceMs);
	}

	setTags(next: string[]): void {
		this.desiredTags = next;
		if (this.contentId) void this.#reconcileTags();
	}

	setSeries(id: string | null): void {
		this.desiredSeries = id;
		if (this.contentId) void this.#reconcileSeries();
	}

	/** Persist the current snapshot. Single-flight; reschedules if edits arrive mid-flight. */
	async flush(): Promise<void> {
		if (this.#inFlight) return;
		if (this.contentId && this.#rev === this.#savedRev) return; // nothing new
		this.#inFlight = true;
		this.lastError = null;
		const rev = this.#rev;
		const body = this.#opts.getSnapshot();
		try {
			if (!this.contentId) {
				this.status = 'creating';
				const res = await this.#deps.create({ ...body, status: 'draft' });
				if (res.error || !res.data) return this.#fail(res.status, res.error?.message);
				this.contentId = res.data.id;
				this.#markSaved(rev);
				await this.#reconcileTags();
				await this.#reconcileSeries();
			} else {
				this.status = 'saving';
				const res = await this.#deps.patch(this.contentId, body);
				if (res.error) return this.#fail(res.status, res.error.message);
				this.#markSaved(rev);
			}
		} finally {
			this.#inFlight = false;
			if (this.status === 'saved' && this.#rev !== this.#savedRev) this.#schedule(0);
		}
	}

	/** Validate, flush the latest draft, then publish or schedule. Returns success. */
	async publish(): Promise<boolean> {
		if (!this.#opts.canPublish()) return false;
		await this.flush();
		if (!this.contentId || this.status === 'error') return false;
		const at = this.#opts.getPublishAt();
		const res = at
			? await this.#deps.schedule(this.contentId, at)
			: await this.#deps.publish(this.contentId);
		if (res.error) {
			this.#fail(res.status, res.error.message);
			return false;
		}
		this.status = at ? 'scheduled' : 'published';
		return true;
	}

	dispose(): void {
		if (this.#timer) clearTimeout(this.#timer);
	}

	#markSaved(rev: number): void {
		this.#savedRev = rev;
		this.status = 'saved';
		this.savedAt = Date.now();
		this.#retries = 0;
	}

	#fail(status: number, message = ''): void {
		const cls = this.#deps.classify(status);
		this.lastError = { cls, message };
		if ((cls === 'network' || cls === 'server') && this.#retries < this.#opts.maxRetries) {
			this.#retries++;
			this.status = 'dirty';
			this.#schedule(this.#backoff());
		} else {
			this.status = 'error';
		}
	}

	#backoff(): number {
		return Math.min(1000 * 2 ** this.#retries, 15000);
	}

	#schedule(ms: number): void {
		if (this.#timer) clearTimeout(this.#timer);
		this.#timer = setTimeout(() => void this.flush(), ms);
	}

	async #reconcileTags(): Promise<void> {
		if (!this.contentId) return;
		const add = this.desiredTags.filter((t) => !this.#committedTags.includes(t));
		const remove = this.#committedTags.filter((t) => !this.desiredTags.includes(t));
		for (const t of add) await this.#deps.attachTag(this.contentId, t);
		for (const t of remove) await this.#deps.detachTag(this.contentId, t);
		this.#committedTags = [...this.desiredTags];
	}

	async #reconcileSeries(): Promise<void> {
		if (!this.contentId || this.desiredSeries === this.#committedSeries) return;
		if (this.#committedSeries) await this.#deps.unlinkSeries(this.#committedSeries, this.contentId);
		if (this.desiredSeries) await this.#deps.linkSeries(this.desiredSeries, this.contentId, 0);
		this.#committedSeries = this.desiredSeries;
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test composerState`
Expected: PASS (all lifecycle, linkage, publish-gate, and failure-class tests).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/create/composerState.svelte.ts apps/main/src/lib/components/create/composerState.svelte.test.ts
git commit -m "feat(main): ComposerState autosave machine + deferred linkage"
```

---

## Phase 2 — Route, loader, shell

### Task 7: Pure UI helpers — publish gate, status label, exception-route (TDD)

**Files:**
- Create: `src/lib/components/create/gate.ts`
- Test: `src/lib/components/create/gate.test.ts`

**Interfaces:**
- Consumes: `ComposerStatus`, `FormKind`.
- Produces:
  - `interface PublishInput { title: string; spaceId: string | null; requiredFieldsFilled: boolean; formKind: FormKind; itemCount: number; dimensionCount: number }`
  - `evaluatePublish(i: PublishInput): { ok: boolean; reason: string | null }`
  - `statusLabel(status: ComposerStatus): string` (returns an i18n KEY, not copy)
  - `isExceptionRoute(routeId: string | null): boolean`

- [ ] **Step 1: Write the failing test**

`src/lib/components/create/gate.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { evaluatePublish, statusLabel, isExceptionRoute } from './gate';

describe('evaluatePublish', () => {
	const base = { title: 'Hi', spaceId: 's1', requiredFieldsFilled: true, formKind: 'standard' as const, itemCount: 0, dimensionCount: 0 };
	it('passes a valid standard post', () => {
		expect(evaluatePublish({ ...base, formKind: 'standard' }).ok).toBe(true);
	});
	it('blocks with no title', () => {
		expect(evaluatePublish({ ...base, title: '  ' }).reason).toBe('create.gate.title');
	});
	it('blocks with no space', () => {
		expect(evaluatePublish({ ...base, spaceId: null }).reason).toBe('create.gate.space');
	});
	it('blocks when required custom fields are unfilled', () => {
		expect(evaluatePublish({ ...base, requiredFieldsFilled: false }).reason).toBe('create.gate.fields');
	});
	it('requires >=2 items and >=1 dimension for a comparison', () => {
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 1, dimensionCount: 1 }).reason).toBe('create.gate.items');
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 2, dimensionCount: 0 }).reason).toBe('create.gate.dimensions');
		expect(evaluatePublish({ ...base, formKind: 'comparison', itemCount: 2, dimensionCount: 1 }).ok).toBe(true);
	});
});

describe('statusLabel', () => {
	it('maps each status to a key', () => {
		expect(statusLabel('saving')).toBe('create.status.saving');
		expect(statusLabel('saved')).toBe('create.status.saved');
		expect(statusLabel('error')).toBe('create.status.error');
		expect(statusLabel('scheduled')).toBe('create.status.scheduled');
	});
});

describe('isExceptionRoute', () => {
	it('matches the create route', () => {
		expect(isExceptionRoute('/(app)/create')).toBe(true);
	});
	it('does not match the home route', () => {
		expect(isExceptionRoute('/(app)')).toBe(false);
		expect(isExceptionRoute(null)).toBe(false);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test create/gate`
Expected: FAIL (module not found).

- [ ] **Step 3: Write the implementation**

`src/lib/components/create/gate.ts`:

```ts
import type { ComposerStatus } from '$lib/types';
import type { FormKind } from './contentTypeRegistry';

export interface PublishInput {
	title: string;
	spaceId: string | null;
	requiredFieldsFilled: boolean;
	formKind: FormKind;
	itemCount: number;
	dimensionCount: number;
}

export function evaluatePublish(i: PublishInput): { ok: boolean; reason: string | null } {
	if (i.title.trim().length === 0) return { ok: false, reason: 'create.gate.title' };
	if (!i.spaceId) return { ok: false, reason: 'create.gate.space' };
	if (i.formKind === 'comparison') {
		if (i.itemCount < 2) return { ok: false, reason: 'create.gate.items' };
		if (i.dimensionCount < 1) return { ok: false, reason: 'create.gate.dimensions' };
	}
	if (!i.requiredFieldsFilled) return { ok: false, reason: 'create.gate.fields' };
	return { ok: true, reason: null };
}

const STATUS_KEYS: Record<ComposerStatus, string> = {
	pristine: 'create.status.draft',
	creating: 'create.status.saving',
	saved: 'create.status.saved',
	dirty: 'create.status.unsaved',
	saving: 'create.status.saving',
	published: 'create.status.published',
	scheduled: 'create.status.scheduled',
	error: 'create.status.error'
};

export function statusLabel(status: ComposerStatus): string {
	return STATUS_KEYS[status];
}

export function isExceptionRoute(routeId: string | null): boolean {
	return routeId === '/(app)/create';
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bun run test create/gate`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/create/gate.ts apps/main/src/lib/components/create/gate.test.ts
git commit -m "feat(main): pure publish-gate + status-label + exception-route helpers"
```

---

### Task 8: SSR loader + exception-layout flag

**Files:**
- Create: `src/routes/(app)/create/+page.ts`
- Create (replace stub): `src/routes/(app)/create/+page.server.ts`

**Interfaces:**
- Consumes: `apiContext`, `apiGet`, `apiList` (`$lib/server/api`); `availableTypes` (Task 3); types (Task 1).
- Produces: page `data` = `{ types: TypeEntry[]; spaces: MeSpace[]; fieldGroups: CustomFieldGroup[]; series: BlogSeries[]; draft: ContentRecord | null; loadError: boolean }`.

- [ ] **Step 1: Write `+page.ts`** (no test — SvelteKit page option)

```ts
// The create surface is ADR-058 Exception #2: it opts out of the normal shell.
// The (app) layout reads this via page.route.id (see gate.isExceptionRoute).
export const ssr = true;
```

- [ ] **Step 2: Write the loader** `src/routes/(app)/create/+page.server.ts`:

```ts
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { availableTypes } from '$lib/components/create/contentTypeRegistry';
import type { ContentTypeMeta, MeSpace, CustomFieldGroup, BlogSeries, ContentRecord } from '$lib/types';

/** Load everything the composer needs; every fetch degrades independently. */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const draftId = event.url.searchParams.get('draft');

	const [typesRes, flagsRes, spacesRes, groupsRes, seriesRes] = await Promise.allSettled([
		apiList<ContentTypeMeta>(ctx, '/content-types'),
		apiGet<Record<string, boolean>>(ctx, '/feature-flags'),
		apiList<MeSpace>(ctx, '/me/spaces'),
		apiList<CustomFieldGroup>(ctx, '/custom-field-groups'),
		apiList<BlogSeries>(ctx, '/blog/series')
	]);

	const rawTypes = typesRes.status === 'fulfilled' ? typesRes.value.items : [];
	const flags = flagsRes.status === 'fulfilled' ? flagsRes.value : {};
	const spaces = spacesRes.status === 'fulfilled' ? spacesRes.value.items : [];
	const fieldGroups =
		groupsRes.status === 'fulfilled'
			? groupsRes.value.items.filter((g) => g.entity_type === 'content')
			: [];
	const series = seriesRes.status === 'fulfilled' ? seriesRes.value.items : [];

	let draft: ContentRecord | null = null;
	if (draftId) {
		try {
			draft = await apiGet<ContentRecord>(ctx, `/content/${draftId}`);
		} catch {
			draft = null;
		}
	}

	return {
		types: availableTypes(rawTypes, flags),
		spaces,
		fieldGroups,
		series,
		draft,
		loadError: typesRes.status === 'rejected'
	};
};
```

- [ ] **Step 3: Verify types + a smoke check**

Run: `cd apps/main && bun run check`
Expected: PASS. (The `availableTypes` intersection is already unit-tested in Task 3; the loader is verified end-to-end in Task 22.)

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/routes/\(app\)/create/+page.ts apps/main/src/routes/\(app\)/create/+page.server.ts
git commit -m "feat(main): /create loader (types∩flags, spaces, content field-groups, series, draft)"
```

---

### Task 9: Exception-layout in `(app)/+layout.svelte`

**Files:**
- Modify: `src/routes/(app)/+layout.svelte`

**Interfaces:**
- Consumes: `isExceptionRoute` (Task 7); `page` from `$app/state`.

- [ ] **Step 1: Read the current layout** to find where `AppShell`/`LeftNav`/`RightSidebar` render.

Run: `cat src/routes/(app)/+layout.svelte`

- [ ] **Step 2: Add the exception branch.** Derive the flag and pass it to the shell so the nav renders collapsed and the right sidebar is suppressed on `/create`:

```svelte
<script lang="ts">
	import { page } from '$app/state';
	import { isExceptionRoute } from '$lib/components/create/gate';
	// ...existing imports (AppShell, data, etc.)
	let { children, data } = $props();
	const exception = $derived(isExceptionRoute(page.route.id));
</script>

<AppShell
	navCollapsed={exception || data.navCollapsed}
	hideRightSidebar={exception}
	{data}
>
	{@render children()}
</AppShell>
```

> Follow the actual prop names in `AppShell.svelte`; if it lacks `hideRightSidebar`/`navCollapsed` props, add them (a boolean that switches the grid template to `72px 1fr` and omits `<RightSidebar>`), per jc-main-develop shell rules. Keep changes minimal and behavior-preserving for non-exception routes.

- [ ] **Step 3: Verify**

Run: `cd apps/main && bun run check`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/routes/\(app\)/+layout.svelte apps/main/src/lib/components/shell/AppShell.svelte
git commit -m "feat(main): collapse nav + hide right sidebar on /create (Exception #2)"
```

---

### Task 10: `CreateShell` + `SaveBar`

**Files:**
- Create: `src/lib/components/create/CreateShell.svelte`
- Create: `src/lib/components/create/SaveBar.svelte`
- Test: `src/lib/components/create/SaveBar.test.ts`

**Interfaces:**
- `CreateShell` props: `{ data: PageData }` (the loader output). It instantiates `ComposerState` (wiring the Task-5 API wrappers as deps + `classifyError`), holds the form field `$state`, resolves the editor/panel via the registry, and lays out left column + `CreateRightPanel` + `SaveBar`.
- `SaveBar` props: `{ status: ComposerStatus; savedAt: number | null; canPublish: boolean; publishReason: string | null; onSaveDraft: () => void; onPublish: () => void }`.

- [ ] **Step 1: Write the failing test** (SaveBar renders the right status key + disables Publish when blocked)

`src/lib/components/create/SaveBar.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SaveBar from './SaveBar.svelte';

describe('SaveBar', () => {
	it('disables Publish when canPublish is false and shows the reason', () => {
		const { getByRole, getByText } = render(SaveBar, {
			props: {
				status: 'saved', savedAt: Date.now(), canPublish: false,
				publishReason: 'create.gate.space', onSaveDraft: () => {}, onPublish: () => {}
			}
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(true);
		// reason is rendered (as its i18n key text via the test t() fallback)
		expect(getByText(/space/i)).toBeTruthy();
	});
	it('enables Publish when canPublish is true', () => {
		const { getByRole } = render(SaveBar, {
			props: { status: 'saved', savedAt: Date.now(), canPublish: true, publishReason: null, onSaveDraft: () => {}, onPublish: () => {} }
		});
		expect((getByRole('button', { name: /publish/i }) as HTMLButtonElement).disabled).toBe(false);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bun run test SaveBar`
Expected: FAIL (component not found).

- [ ] **Step 3: Implement `SaveBar.svelte`** — sticky bar; status pill via `statusLabel(status)`; two buttons. Use Runes + Tailwind tokens:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import { statusLabel } from './gate';
	import type { ComposerStatus } from '$lib/types';
	interface Props {
		status: ComposerStatus;
		savedAt: number | null;
		canPublish: boolean;
		publishReason: string | null;
		onSaveDraft: () => void;
		onPublish: () => void;
	}
	let { status, canPublish, publishReason, onSaveDraft, onPublish }: Props = $props();
</script>

<div class="sticky bottom-0 flex items-center justify-between gap-3 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-3">
	<span class="text-sm text-[var(--color-muted)]">{t(statusLabel(status))}</span>
	<div class="flex items-center gap-2">
		{#if !canPublish && publishReason}
			<span class="text-xs text-[var(--color-muted)]">{t(publishReason)}</span>
		{/if}
		<button type="button" class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={onSaveDraft}>
			{t('create.action.save_draft')}
		</button>
		<button type="button" disabled={!canPublish} onclick={onPublish}
			class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50">
			{t('create.action.publish')}
		</button>
	</div>
</div>
```

- [ ] **Step 4: Implement `CreateShell.svelte`** — the host. Structure (follow jc-main-develop layout tokens):

```svelte
<script lang="ts">
	import type { PageData } from '../../../routes/(app)/create/$types';
	import { ComposerState } from './composerState.svelte';
	import { resolveType } from './contentTypeRegistry';
	import { evaluatePublish } from './gate';
	import * as create from '$lib/api/create';
	import { classifyError } from '$lib/api/create';
	import { suggestSlug } from './slug';
	import PostEditor from './editors/PostEditor.svelte';
	import ComparisonBuilder from './editors/ComparisonBuilder.svelte';
	import CreateRightPanel from './panel/CreateRightPanel.svelte';
	import SaveBar from './SaveBar.svelte';
	import TitleField from './fields/TitleField.svelte';
	import TagInput from './fields/TagInput.svelte';
	import SpaceSelector from './fields/SpaceSelector.svelte';
	import CoverUploader from './fields/CoverUploader.svelte';

	let { data }: { data: PageData } = $props();

	// form field state (hydrated from data.draft when present)
	let contentType = $state(data.draft?.content_type ?? 'blog_post');
	let title = $state(data.draft?.title ?? '');
	let bodyHtml = $state(data.draft?.body_html ?? '');
	let spaceId = $state<string | null>(data.draft?.space_id ?? null);
	let fieldValues = $state<Record<string, unknown>>(data.draft?.field_values ?? {});
	let visibility = $state(data.draft?.visibility ?? 'public');
	let publishAt = $state<string | null>(null);
	let canonicalUrl = $state<string | null>(data.draft?.canonical_url ?? null);
	// comparison-only counters (set by ComparisonBuilder via callbacks)
	let itemCount = $state(0);
	let dimensionCount = $state(0);

	const entry = $derived(resolveType(contentType));

	const gate = $derived(
		evaluatePublish({
			title, spaceId,
			requiredFieldsFilled: true, // TODO wired in Task 20 from required custom fields
			formKind: entry.formKind, itemCount, dimensionCount
		})
	);

	const state = new ComposerState(
		{
			create: create.createContent, patch: create.patchContent,
			publish: create.publishContent, schedule: create.scheduleContent,
			attachTag: create.attachTag, detachTag: create.detachTag,
			linkSeries: create.linkSeries, unlinkSeries: create.unlinkSeries,
			classify: classifyError
		},
		{
			getSnapshot: () => ({
				content_type: contentType, title, slug: suggestSlug(title),
				body_html: bodyHtml, field_values: fieldValues,
				canonical_url: canonicalUrl, visibility, space_id: spaceId
			}),
			canPublish: () => gate.ok,
			getPublishAt: () => publishAt
		}
	);

	$effect(() => () => state.dispose());
</script>

<div class="grid grid-cols-1 gap-6 lg:grid-cols-[1fr_350px]">
	<div class="flex min-h-[calc(100vh-var(--nav-height))] flex-col">
		<div class="flex-1 space-y-4 px-4 py-6">
			<CoverUploader />
			<TitleField bind:value={title} onedit={() => state.markDirty()} />
			<TagInput ontags={(ids) => state.setTags(ids)} />
			<SpaceSelector spaces={data.spaces} bind:value={spaceId} />
			{#if entry.formKind === 'comparison'}
				<ComparisonBuilder oncounts={(i, d) => { itemCount = i; dimensionCount = d; }} onedit={() => state.markDirty()} />
			{:else}
				<PostEditor bind:html={bodyHtml} onedit={() => state.markDirty()} />
			{/if}
		</div>
		<SaveBar
			status={state.status} savedAt={state.savedAt}
			canPublish={gate.ok} publishReason={gate.reason}
			onSaveDraft={() => state.flush()} onPublish={() => state.publish()}
		/>
	</div>
	<CreateRightPanel {data} bind:contentType bind:visibility bind:publishAt bind:canonicalUrl bind:fieldValues onedit={() => state.markDirty()} />
</div>
```

> This shell is the integration seam; the child components it imports are built in Tasks 11–20 and the required-fields wiring is completed in Task 20. Expect `bun run check` to fail on missing child components until then — that is why the full-assembly verification is Task 21.

- [ ] **Step 5: Run the SaveBar test**

Run: `cd apps/main && bun run test SaveBar`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/main/src/lib/components/create/CreateShell.svelte apps/main/src/lib/components/create/SaveBar.svelte apps/main/src/lib/components/create/SaveBar.test.ts
git commit -m "feat(main): CreateShell host + SaveBar (status pill + publish gate)"
```

---

## Phase 3 — Fields

> Each field component: Svelte 5 Runes, Bits UI dot-notation where a primitive applies, Tailwind tokens, i18n. Follow the `jc-main-develop` and `svelte-components` skills for exact Bits UI markup. Each owns one input and emits a typed value via a callback prop.

### Task 11: `TitleField`

**Files:** Create `src/lib/components/create/fields/TitleField.svelte`
**Props:** `{ value: string (bindable); onedit: () => void }`
**Behavior:** an autosizing `<textarea>` styled as an H1; on input, updates `value` and calls `onedit()` (which drives autosave + slug suggestion in the shell). Placeholder `t('create.title.placeholder')`.

- [ ] Step 1: Implement the component (bindable value, `oninput` → `onedit()`).
- [ ] Step 2: `bun run check` passes for this file.
- [ ] Step 3: Commit `feat(main): TitleField (autosizing title)`.

### Task 12: `SpaceSelector` (required)

**Files:** Create `src/lib/components/create/fields/SpaceSelector.svelte`; Test `.../SpaceSelector.test.ts`
**Props:** `{ spaces: MeSpace[]; value: string | null (bindable) }`
**Behavior:** Bits UI `Select` listing `spaces`; marked required with a visible asterisk + `t('create.space.required')` hint when `value` is null. Selecting sets `value`.

- [ ] **Step 1: Write the failing test** — with `spaces=[]`, the required hint renders; with a value chosen, the hint is gone.

```ts
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import SpaceSelector from './SpaceSelector.svelte';

it('shows the required hint when no space is selected', () => {
	const { getByText } = render(SpaceSelector, { props: { spaces: [{ id: 's1', slug: 'a', name: 'A' }], value: null } });
	expect(getByText(/required/i)).toBeTruthy();
});
```

- [ ] Step 2: Run `bun run test SpaceSelector` → FAIL.
- [ ] Step 3: Implement with Bits UI `Select.*`.
- [ ] Step 4: Run test → PASS.
- [ ] Step 5: Commit `feat(main): SpaceSelector (required primary-space gate)`.

### Task 13: `TagInput`

**Files:** Create `src/lib/components/create/fields/TagInput.svelte`
**Props:** `{ ontags: (tagIds: string[]) => void }`
**Behavior:** a chip input; typing queries `create.searchHits('tags', q)` (debounced ~200ms) in a Bits UI `Popover`; selecting a suggestion adds a chip and calls `ontags(currentIds)`; removing a chip re-emits. On search failure, show `t('create.search.failed')` inline; typing still works.

- [ ] Step 1: Implement (reuse the Task-5 `searchHits`; debounce via a small timeout).
- [ ] Step 2: `bun run check` passes.
- [ ] Step 3: Commit `feat(main): TagInput (chips + autocomplete)`.

### Task 14: `CoverUploader` (2-step media)

**Files:** Create `src/lib/components/create/fields/CoverUploader.svelte`
**Prerequisite:** confirm the `POST /media/upload` contract:

Run: `sed -n '/func.*Upload/,/^}/p' ../../internal/media/handlers.go | head -60` (from repo root: `internal/media/handlers.go`)

**Props:** `{ value?: MediaAsset | null (bindable) }`
**Behavior:** file input → client-side type/size validation → `uploadMedia(file)` → `confirmMedia(id)` → set `value`; on failure show a retry affordance on the control only (never block the form). Preview the returned `url`.

- [ ] Step 1: Read the media handler; finalize the two flagged wrappers in `create.ts` (Task 5) to match the real multipart contract; adjust the `create.test.ts` mock if the path/shape changed.
- [ ] Step 2: Implement the component.
- [ ] Step 3: `bun run check` passes.
- [ ] Step 4: Commit `feat(main): CoverUploader (2-step media upload)`.

---

## Phase 4 — Editors

> **Load the `tiptap` skill before these tasks.** Do not fabricate ProseMirror/TipTap suggestion-plugin APIs; wire them per that skill. The plan fixes the *interfaces, data flow, and tests*; the skill supplies the exact extension code.

### Task 15: TipTap `PostEditor` base (formatting + `/` slash menu)

**Files:**
- Create: `src/lib/tiptap/slashMenu.ts` (block list config), `src/lib/tiptap/extensions.ts` (`buildExtensions(opts)`)
- Create: `src/lib/components/create/editors/PostEditor.svelte`
- Test: `src/lib/tiptap/slashMenu.test.ts`

**Interfaces:**
- `PostEditor` props: `{ html: string (bindable); onedit: () => void }`.
- `buildExtensions(opts: { onSearch: (kind, q) => Promise<SearchHit[]> }): Extension[]` — StarterKit + Image + the four suggestion extensions (Task 16 fills @ # [[).
- `slashMenu.ts`: `export const SLASH_BLOCKS: { key: string; labelKey: string; run: (editor) => void }[]`.

**Behavior:** mounts a TipTap editor bound to `html`; formatting (headings, bold/italic, lists, quote, code block, link, inline image via `CoverUploader`'s upload path). `/` opens a block menu from `SLASH_BLOCKS`. Emits `onedit()` on every transaction.

- [ ] **Step 1: Write the failing test** for the pure slash config (keys/labels present, `run` is a function):

```ts
import { describe, it, expect } from 'vitest';
import { SLASH_BLOCKS } from './slashMenu';
it('defines heading/list/quote/code/image blocks with i18n label keys', () => {
	const keys = SLASH_BLOCKS.map((b) => b.key);
	expect(keys).toEqual(expect.arrayContaining(['heading1', 'bulletList', 'blockquote', 'codeBlock', 'image']));
	for (const b of SLASH_BLOCKS) { expect(b.labelKey.startsWith('create.slash.')).toBe(true); expect(typeof b.run).toBe('function'); }
});
```

- [ ] Step 2: Run `bun run test slashMenu` → FAIL.
- [ ] Step 3: Implement `slashMenu.ts`, `extensions.ts`, and `PostEditor.svelte` (per the tiptap skill).
- [ ] Step 4: Run `bun run test slashMenu` → PASS; `bun run check` passes for the new files.
- [ ] Step 5: Commit `feat(main): TipTap PostEditor base + slash menu`.

### Task 16: `@` mention · `#` hashtag · `[[` interlink triggers

**Files:** Modify `src/lib/tiptap/extensions.ts`; add mention/hashtag/interlink suggestion configs.
**Interfaces:** each suggestion uses `create.searchHits(kind, query)` (Task 5) via the `onSearch` opt; min-length + ~200ms debounce come from `searchPath` (Task 4). Inserts nodes: mention (user id + handle), hashtag (tag), interlink (content id + slug).
**Behavior:** on failure, the suggestion list shows `t('create.search.failed')`; the author can finish the token as plain text. Keyboard nav in the popup.

- [ ] Step 1: Wire the three suggestions (tiptap skill). The query-shaping + mapping are already unit-tested (Tasks 4–5); no new pure logic.
- [ ] Step 2: `bun run check` passes; re-run `bun run test tiptap` (search tests still green).
- [ ] Step 3: Commit `feat(main): TipTap @ mention, # hashtag, [[ interlink triggers`.

### Task 17: `ComparisonBuilder`

**Files:** Create `src/lib/components/create/editors/ComparisonBuilder.svelte`
**Prerequisite:** confirm sub-resource payloads in `internal/comparison/handlers.go` (items/dimensions/values create shapes).
**Props:** `{ oncounts: (items: number, dims: number) => void; onedit: () => void }`
**Behavior:** an items (rows) × dimensions (columns) grid. Adding the first comparison mints the shell via `create.createComparison` (needs the content title — read from shell); items/dimensions/values persist immediately via their own mutations (`addComparisonItem`, `addComparisonDimension`, `setDimensionValues`) — NOT via the debounced blob. Reorder + cell edit supported. Reports counts up via `oncounts` so the publish gate can require ≥2 items + ≥1 dimension.

- [ ] Step 1: Read the comparison handlers; finalize the comparison wrappers in `create.ts` to match.
- [ ] Step 2: Implement the grid (Bits UI where applicable; follow jc-main-develop).
- [ ] Step 3: `bun run check` passes.
- [ ] Step 4: Commit `feat(main): ComparisonBuilder (items × dimensions grid)`.

---

## Phase 5 — Right panel

### Task 18: `CreateRightPanel` (tabs)

**Files:** Create `src/lib/components/create/panel/CreateRightPanel.svelte`
**Props:** `{ data: PageData; contentType, visibility, publishAt, canonicalUrl, fieldValues (all bindable); onedit: () => void }`
**Behavior:** Bits UI `Tabs` with `[ Guide | Settings ]`; Guide is the default tab. Renders `CreateGuidePanel` and `CreateSettingsPanel`, forwarding a `focusedField` signal (see Task 19).

- [ ] Step 1: Implement the Tabs host (Bits UI `Tabs.*`).
- [ ] Step 2: `bun run check` passes.
- [ ] Step 3: Commit `feat(main): CreateRightPanel ([Guide|Settings] tabs)`.

### Task 19: `CreateGuidePanel` (focus-reactive)

**Files:** Create `src/lib/components/create/panel/CreateGuidePanel.svelte`; Test `.../CreateGuidePanel.test.ts`
**Props:** `{ focusedField: 'space' | 'tags' | 'series' | 'body' | null }`
**Behavior:** shows help copy keyed to `focusedField` (space→primary-space rule, tags→discovery, series→grouping, body→`/ @ # [[` reference); default copy when null. All copy via i18n keys.

- [ ] **Step 1: Write the failing test** — focusedField='series' renders the series help key text.

```ts
import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import CreateGuidePanel from './CreateGuidePanel.svelte';
it('shows series help when the series field is focused', () => {
	const { getByText } = render(CreateGuidePanel, { props: { focusedField: 'series' } });
	expect(getByText(/series/i)).toBeTruthy();
});
```

- [ ] Step 2: Run `bun run test CreateGuidePanel` → FAIL.
- [ ] Step 3: Implement (a `$derived` map from `focusedField` → help key).
- [ ] Step 4: Run test → PASS.
- [ ] Step 5: Commit `feat(main): CreateGuidePanel (focus-reactive help)`.

### Task 20: `CreateSettingsPanel` + `SeriesSelector` + `CustomFieldGroup`

**Files:** Create `.../panel/CreateSettingsPanel.svelte`, `.../panel/SeriesSelector.svelte`, `.../panel/CustomFieldGroup.svelte`
**Prerequisite:** confirm the field-definition JSON property names (`field_type`, `is_required`, `options`) from `internal/customfields` sqlc rows.
**Props:**
- `CreateSettingsPanel`: `{ data; visibility, publishAt, canonicalUrl, fieldValues (bindable); formKind: FormKind; onedit; onRequiredFilledChange: (filled: boolean) => void }`
- `SeriesSelector`: `{ series: BlogSeries[]; value: string | null (bindable) }` — pick existing (Bits UI `Select`) or create-new inline (`create.createSeries`).
- `CustomFieldGroup`: `{ group: CustomFieldGroup; values: Record<string, unknown> (bindable) }` — renders one input per `CustomFieldDef.field_type`.
**Behavior:** stacks Visibility (`Select`: public/unlisted/private), Schedule (`datetime-local` → `publishAt`), Canonical (`url`), `SeriesSelector` (only when `formKind==='standard'`), the `CustomFieldGroup[]` from `data.fieldGroups`, and SEO fields (meta title/desc, OG image via `CoverUploader` pattern, index toggle). Custom-field values merge into `fieldValues` (which the shell sends inline). Computes whether all required fields are filled and reports via `onRequiredFilledChange` (feeds the publish gate `requiredFieldsFilled`).

- [ ] Step 1: Read the customfields row shape; confirm property names.
- [ ] Step 2: Implement `CustomFieldGroup` (switch on `field_type` → text/textarea/number/boolean/date/select/multiselect/rating/url inputs); compute required-filled.
- [ ] Step 3: Implement `SeriesSelector` (existing + create-new).
- [ ] Step 4: Implement `CreateSettingsPanel` composing the above; wire `onRequiredFilledChange`.
- [ ] Step 5: In `CreateShell.svelte`, replace the `requiredFieldsFilled: true` TODO with the reported value (add `let requiredFilled = $state(true)` and pass `onRequiredFilledChange`).
- [ ] Step 6: `bun run check` passes.
- [ ] Step 7: Commit `feat(main): CreateSettingsPanel + SeriesSelector + CustomFieldGroup`.

---

## Phase 6 — Assembly, i18n, acceptance

### Task 21: Page assembly + i18n keys

**Files:** Modify `src/routes/(app)/create/+page.svelte` (replace stub); Modify `src/lib/i18n/en.json`, `src/lib/i18n/fr.json`.

- [ ] **Step 1: Replace the stub page** to host the shell:

```svelte
<script lang="ts">
	import CreateShell from '$lib/components/create/CreateShell.svelte';
	let { data } = $props();
</script>

<svelte:head><title>{data.draft ? 'Edit draft' : 'Create'} · JapaConnect</title></svelte:head>
<CreateShell {data} />
```

- [ ] **Step 2: Add every `create.*` key** used across Tasks 3–20 to `en.json` AND `fr.json`. Collect them by grepping:

Run: `cd apps/main && grep -rohE "create\.[a-z_.]+" src/lib/components/create src/lib/tiptap | sort -u`

Add each key to both locale files (English copy in `en.json`, French in `fr.json`). Required groups: `create.status.*`, `create.gate.*`, `create.action.*`, `create.title.*`, `create.space.*`, `create.search.*`, `create.slash.*`, `create.guide.*`, `create.settings.*`, `create.series.*`, `create.seo.*`.

- [ ] **Step 3: Verify no missing keys**

Run: `cd apps/main && bun run check && bun run test`
Expected: PASS (all suites green).

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/routes/\(app\)/create/+page.svelte apps/main/src/lib/i18n/en.json apps/main/src/lib/i18n/fr.json
git commit -m "feat(main): assemble /create page + i18n keys (en/fr)"
```

### Task 22: Full suite + live round-trip acceptance

**Files:** none (verification task).

- [ ] **Step 1: Full frontend gate**

Run: `cd apps/main && bun run test && bun run check`
Expected: all five suites PASS; zero `svelte-check` errors. If anything fails, fix before proceeding — it is a regression against the baseline.

- [ ] **Step 2: Live round-trip** (backend on :8080, app on :3000). Load the `verify` or `run` skill to drive the app. Exercise, against the running backend, for a `blog_post`:
  1. Open `/create`; confirm Exception #2 layout (nav collapsed, no right sidebar, tabbed Guide|Settings).
  2. Type a title + body; confirm the status pill goes *Saving… → Saved*, and a draft row appears in `GET /me/drafts`.
  3. Add a tag and a series; reload via `?draft=<id>`; confirm both persisted and the body restored.
  4. Try Publish with no Space → blocked with the space reason; pick a Space → Publish enables; publish → status transitions and the content is retrievable.
  5. Repeat the create→autosave→publish path once for a `comparison` (add ≥2 items + ≥1 dimension) to exercise the second create path.
- [ ] **Step 3: Record the result** in the plan's PR/commit description. Any envelope/shape drift found here (the class of bug unit tests structurally miss) is fixed now, with a regression test added where feasible.
- [ ] **Step 4: Final commit** (if fixes were needed)

```bash
git add -p
git commit -m "fix(main): /create live round-trip corrections"
```

---

## Self-Review (completed by plan author)

- **Spec coverage:** Exception-#2 layout (T8/T9) · registry two-path (T3/T5) · autosave machine + deferred linkage (T6) · four triggers (T4/T15/T16) · full settings incl. series + custom fields (T20) · feature-flag selector (T3/T8) · error classes + publish gate (T5/T6/T7) · inline `field_values` atomic (T5/T6/T20) · testing incl. live round-trip (T22). All spec sections map to tasks.
- **Type consistency:** `TypeEntry`/`FormKind` (T3) reused in T7/T8/T10/T20; `ComposerDeps`/`ComposerOptions` (T6) match the wrappers in T5 and the wiring in T10; `SearchHit` mappers (T4) consumed by `searchHits` (T5) consumed by T13/T16.
- **Known deferred confirmations (read-only backend lookups, flagged inline):** media multipart contract (T14), tag-attach body key (T14), comparison sub-resource payloads (T17), custom-field row property names (T20). None affect the design; each is resolved by reading a handler at task time.
```
