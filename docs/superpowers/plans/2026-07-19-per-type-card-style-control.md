# Per-Type Feed Card Style — Control Panel (Layer 3) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Follow the `jc-control-develop` conventions.

**Goal:** Replace the single `feed.card_style` dropdown in the Control settings page with a per-content-type editor: a Default select plus one override select per content type, persisting the `{ default, by_type }` map.

**Architecture:** A pure `cardStyleValue.ts` module parses the stored JSON string into `{ default, byType }` and rebuilds it, keeping serialization testable. A dedicated `FeedCardStyleEditor.svelte` renders the Default + per-type selects and emits the assembled JSON string through the settings page's existing `updateSetting(key, value)` → `saveAll()` path (which PATCHes `/control/system-settings/{key}` with `{ value }`). Content types come from `GET /content-types`.

**Tech Stack:** SvelteKit 2 + Svelte 5 runes, TypeScript, Vitest, bits-ui, the control `api` helper.

## Global Constraints

- The setting key is `feed.card_style`; its value is a JSON string of `{ "default": CardStyle, "by_type": { [content_type]: CardStyle } }`. `CardStyle ∈ { reddit, compact, grid, magazine }`.
- Reuse the existing save path: `updateSetting('feed.card_style', jsonString)` then the page's `saveAll()`. Do not add a new endpoint.
- Content types come from `GET /content-types` → `{ data: { value, label }[] }`.
- "Use default" for a type means omit it from `by_type`.
- Reuse existing i18n labels `settings.card_style_reddit|compact|grid|magazine`; new UI strings use `t(key, fallback)` fallbacks so they render without a seed.
- Tests: `npm run test` (vitest). Gate: `npm run check` + `npm run build` green.
- Depends on Layer 1 (endpoint returns/accepts the map). Spec: `docs/superpowers/specs/2026-07-19-per-type-feed-card-style-design.md` §4.

---

### Task 1: `cardStyleValue.ts` — parse/build the setting value (pure)

**Files:**
- Create: `apps/control/src/lib/feed/cardStyleValue.ts`
- Test: `apps/control/src/lib/feed/cardStyleValue.test.ts`

**Interfaces:**
- Produces:
  - `interface ParsedCardStyle { default: string; byType: Record<string, string> }`
  - `function parseCardStyleValue(raw: string): ParsedCardStyle`
  - `function buildCardStyleValue(def: string, byType: Record<string, string>): string`

- [ ] **Step 1: Write the failing test**

Create `apps/control/src/lib/feed/cardStyleValue.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { parseCardStyleValue, buildCardStyleValue } from './cardStyleValue';

describe('parseCardStyleValue', () => {
	it('parses the object form', () => {
		expect(parseCardStyleValue('{"default":"reddit","by_type":{"blog_post":"grid"}}')).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('parses a legacy scalar', () => {
		expect(parseCardStyleValue('"magazine"')).toEqual({ default: 'magazine', byType: {} });
	});
	it('coerces invalid default and drops invalid overrides', () => {
		expect(parseCardStyleValue('{"default":"x","by_type":{"blog_post":"grid","d":"z"}}')).toEqual({
			default: 'reddit',
			byType: { blog_post: 'grid' }
		});
	});
	it('falls back for empty/garbage', () => {
		expect(parseCardStyleValue('')).toEqual({ default: 'reddit', byType: {} });
		expect(parseCardStyleValue('nonsense')).toEqual({ default: 'reddit', byType: {} });
	});
});

describe('buildCardStyleValue', () => {
	it('serializes default + by_type, dropping invalid entries', () => {
		expect(buildCardStyleValue('reddit', { blog_post: 'grid', d: 'bad' })).toBe(
			'{"default":"reddit","by_type":{"blog_post":"grid"}}'
		);
	});
	it('round-trips through parse', () => {
		const s = buildCardStyleValue('compact', { discussion: 'reddit' });
		expect(parseCardStyleValue(s)).toEqual({ default: 'compact', byType: { discussion: 'reddit' } });
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/control && npx vitest run src/lib/feed/cardStyleValue.test.ts`
Expected: FAIL — cannot resolve `./cardStyleValue`.

- [ ] **Step 3: Write minimal implementation**

Create `apps/control/src/lib/feed/cardStyleValue.ts`:

```ts
export interface ParsedCardStyle {
	default: string;
	byType: Record<string, string>;
}

const STYLES = ['reddit', 'compact', 'grid', 'magazine'];

/** Parse the stored feed.card_style JSON string (object, legacy scalar, or junk). */
export function parseCardStyleValue(raw: string): ParsedCardStyle {
	const fallback: ParsedCardStyle = { default: 'reddit', byType: {} };
	if (!raw || !raw.trim()) return fallback;

	let v: unknown;
	try {
		v = JSON.parse(raw);
	} catch {
		return fallback;
	}

	if (typeof v === 'string') {
		return { default: STYLES.includes(v) ? v : 'reddit', byType: {} };
	}
	if (v && typeof v === 'object') {
		const o = v as { default?: unknown; by_type?: unknown };
		const def = typeof o.default === 'string' && STYLES.includes(o.default) ? o.default : 'reddit';
		const byType: Record<string, string> = {};
		if (o.by_type && typeof o.by_type === 'object') {
			for (const [k, val] of Object.entries(o.by_type as Record<string, unknown>)) {
				if (typeof val === 'string' && STYLES.includes(val)) byType[k] = val;
			}
		}
		return { default: def, byType };
	}
	return fallback;
}

/** Build the JSON string for storage: {default, by_type}, dropping invalid styles. */
export function buildCardStyleValue(def: string, byType: Record<string, string>): string {
	const cleaned: Record<string, string> = {};
	for (const [k, v] of Object.entries(byType)) {
		if (STYLES.includes(v)) cleaned[k] = v;
	}
	return JSON.stringify({ default: STYLES.includes(def) ? def : 'reddit', by_type: cleaned });
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/control && npx vitest run src/lib/feed/cardStyleValue.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control/src/lib/feed/cardStyleValue.ts apps/control/src/lib/feed/cardStyleValue.test.ts
git commit -m "feat(control): pure parse/build for feed.card_style map value

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: `FeedCardStyleEditor.svelte` — default + per-type selects

**Files:**
- Create: `apps/control/src/lib/components/FeedCardStyleEditor.svelte`

**Interfaces:**
- Consumes: `parseCardStyleValue`/`buildCardStyleValue` (Task 1), `t` (`$lib/stores/i18n`).
- Produces: `<FeedCardStyleEditor value={string} contentTypes={{ value, label }[]} onchange={(value: string) => void} />`.

- [ ] **Step 1: Create the component**

Create `apps/control/src/lib/components/FeedCardStyleEditor.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/stores/i18n';
	import { parseCardStyleValue, buildCardStyleValue } from '$lib/feed/cardStyleValue';

	interface Props {
		value: string;
		contentTypes: { value: string; label: string }[];
		onchange: (value: string) => void;
	}
	let { value, contentTypes, onchange }: Props = $props();

	const styleOptions = $derived([
		{ value: 'reddit', label: t('settings.card_style_reddit', 'Reddit') },
		{ value: 'compact', label: t('settings.card_style_compact', 'Compact') },
		{ value: 'grid', label: t('settings.card_style_grid', 'Grid') },
		{ value: 'magazine', label: t('settings.card_style_magazine', 'Magazine') }
	]);

	const parsed = $derived(parseCardStyleValue(value));

	const selectClass =
		'w-40 rounded-md border border-border-input bg-background px-2 py-1 text-sm';

	function setDefault(v: string) {
		onchange(buildCardStyleValue(v, parsed.byType));
	}
	function setType(ct: string, v: string) {
		const next = { ...parsed.byType };
		if (v === '') delete next[ct];
		else next[ct] = v;
		onchange(buildCardStyleValue(parsed.default, next));
	}
</script>

<div class="space-y-3">
	<label class="flex items-center justify-between gap-4">
		<span class="text-sm font-medium">{t('settings.card_style_default', 'Default card style')}</span>
		<select
			class={selectClass}
			value={parsed.default}
			onchange={(e) => setDefault((e.currentTarget as HTMLSelectElement).value)}
		>
			{#each styleOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
		</select>
	</label>

	<p class="text-xs text-muted-foreground">
		{t(
			'settings.card_style_grid_note',
			'In the mixed home feed a “grid” type shows as a full-width card; a single-type listing shows a 2-column grid.'
		)}
	</p>

	{#each contentTypes as ct (ct.value)}
		<label class="flex items-center justify-between gap-4">
			<span class="text-sm">{ct.label}</span>
			<select
				class={selectClass}
				value={parsed.byType[ct.value] ?? ''}
				onchange={(e) => setType(ct.value, (e.currentTarget as HTMLSelectElement).value)}
			>
				<option value="">{t('settings.card_style_use_default', 'Use default')}</option>
				{#each styleOptions as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
			</select>
		</label>
	{/each}
</div>
```

- [ ] **Step 2: Verify types**

Run: `cd apps/control && npm run check`
Expected: no errors in `FeedCardStyleEditor.svelte`. (If `border-input`/`muted-foreground` utility names differ in this app, match the page's existing classes — grep `inputClasses` in `settings/+page.svelte`.)

- [ ] **Step 3: Commit**

```bash
git add apps/control/src/lib/components/FeedCardStyleEditor.svelte
git commit -m "feat(control): FeedCardStyleEditor (default + per-type selects)

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Wire the editor into the settings page

**Files:**
- Modify: `apps/control/src/routes/settings/+page.svelte`

**Interfaces:**
- Consumes: `FeedCardStyleEditor` (Task 2); existing `api`, `updateSetting`, `currentSettings`, `load()`.

- [ ] **Step 1: Add the import + content-types state**

In the `<script>` of `apps/control/src/routes/settings/+page.svelte`:

Add to the imports:

```ts
import FeedCardStyleEditor from '$lib/components/FeedCardStyleEditor.svelte';
```

Add a state declaration near the other `$state` declarations:

```ts
let contentTypes = $state<{ value: string; label: string }[]>([]);
```

- [ ] **Step 2: Fetch content types in `load()`**

Inside `async function load()`, after the settings fetch succeeds (where `currentSettings` is populated), add:

```ts
	const ctRes = await api<{ value: string; label: string }[]>('/content-types');
	if (ctRes.ok && Array.isArray(ctRes.data)) {
		contentTypes = ctRes.data;
	}
```

(If the control `api` returns the envelope rather than unwrapped `data`, use the same access pattern the file already uses for list responses.)

- [ ] **Step 3: Special-case the `feed.card_style` render**

Find the block that renders a setting with select options (the `{#if hasSelectOptions(setting.key)}` / `<Select.Root ...>` branch, near line 929). Change its opening so `feed.card_style` renders the editor instead of the generic select. Replace:

```svelte
{#if hasSelectOptions(setting.key)}
```

with:

```svelte
{#if setting.key === 'feed.card_style'}
	<FeedCardStyleEditor
		value={setting.value}
		{contentTypes}
		onchange={(v) => updateSetting('feed.card_style', v)}
	/>
{:else if hasSelectOptions(setting.key)}
```

(The existing `{:else if ...}` / `{:else}` chain and its closing `{/if}` stay as-is — you are only inserting the first branch.)

- [ ] **Step 4: Verify type + build**

Run: `cd apps/control && npm run check && npm run build`
Expected: 0 svelte-check errors; build succeeds.

- [ ] **Step 5: Manual round-trip check (against a running backend)**

With the Control app + backend running, on the Settings page:
- The `feed.card_style` row now shows a **Default** select plus one select per content type (blog_post, discussion, comparison, page).
- Set Default = Reddit, `blog_post` = Grid; click Save. Reload the page: the selections persist (stored as `{"default":"reddit","by_type":{"blog_post":"grid"}}`).
- Set `blog_post` back to "Use default"; Save; reload: `by_type` no longer contains `blog_post`.
- Cross-check `GET /feed/card-style` returns the saved map (Layer 1).

- [ ] **Step 6: Commit**

```bash
git add apps/control/src/routes/settings/+page.svelte
git commit -m "feat(control): per-type card-style editor in settings page

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Acceptance (Layer 3 done)

- The Control settings page renders a Default select + per-content-type override selects for `feed.card_style`.
- Saving persists `{ default, by_type }` to `system_settings` via the existing PATCH path; "Use default" removes the override.
- The saved map is reflected by `GET /feed/card-style` and, end-to-end with Layer 2, changes what the feed renders (Rule 16: the selector is live).
- `npm run test`, `npm run check`, and `npm run build` are green.
