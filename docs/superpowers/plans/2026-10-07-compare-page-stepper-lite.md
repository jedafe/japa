# /compare Stepper-Lite Redesign — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reorganize `apps/main` `/compare` into a landing mode (type cards + popular comparisons) and a workspace mode (step header + collapsing narrow panel + unified filter row + sticky compare tray).

**Architecture:** Frontend-only restructure. `+page.svelte` splits into two URL-driven modes; all new UI lives in `src/lib/components/compare/` as small components fed by a pure-helper module (`compareFlow.ts`). `+page.server.ts` gains one landing-mode fetch. All URL params, `/jce/*` calls, and feature behaviors are preserved.

**Tech Stack:** SvelteKit 2, Svelte 5 runes, Tailwind v4, bits-ui 2.18 (`Popover`), lucide-svelte, vitest + @testing-library/svelte, bun.

## Global Constraints

- **Rule 16:** no hardcoded data — all lists come from `data.*` / API.
- **Rule 3/16:** all user-facing strings via `t()` — new keys added to `apps/main/src/lib/i18n/jce_en.json` AND `jce_fr.json` (flat dotted keys, matching existing `jce.compare.*` style).
- Svelte 5 runes only: `$props()`, `$state()`, `$derived()`, `$effect()`, `onclick` (never `on:click`), no `createEventDispatcher` — callback props.
- Design tokens only: `var(--color-surface)`, `var(--color-border)`, `var(--color-bg)`, `var(--color-text)`, `var(--color-muted)`, `var(--radius-card)`, `var(--radius-button)`. No hardcoded colors except the existing `#ef4444` error pattern.
- SvelteKit navigation: `goto(url, { replaceState: true })` for URL-synced state (existing `navigateURL`/`updateURL` helpers in the page — keep them).
- a11y: interactive elements are `<button>`/`[href]`; icon buttons get `aria-label`; chips' remove buttons get `aria-label`.
- No AI attribution anywhere (Rule 17): plain commit messages, no trailers.
- `bun`/`bunx` for all JS tooling (Rule 18). `cd apps/main` for all test/check commands.
- Verification per task: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json` — require **zero new errors in touched files** (pre-existing errors elsewhere are out of scope).

## File Structure

| File | Responsibility |
|---|---|
| `src/lib/components/compare/compareFlow.ts` | Pure helpers: scope-chain derivation, required-scope detection, summary strings, source-option building. Unit-tested. |
| `src/lib/components/compare/compareFlow.test.ts` | vitest for the above. |
| `src/lib/components/compare/StepHeader.svelte` | 3-chip step indicator (type → narrow → select/browse). |
| `src/lib/components/compare/NarrowPanel.svelte` | Expandable card wrapping `ScopeChain`; Skip/Done/Clear; required-mode. |
| `src/lib/components/compare/CompareTray.svelte` | Sticky bottom bar: item chips, count, Compare/Login/AI actions, max hint. |
| `src/lib/components/compare/AiQuickPanel.svelte` | Extracted AI Quick Compare form + result display. |
| `src/lib/components/compare/TypeCardGrid.svelte` | Landing grid of comparison-type cards. |
| `src/lib/components/compare/ScopeChain.svelte` | Additive `bind:labels` prop (id→title map for the summary chip). Rendered UI unchanged. |
| `src/routes/(app)/compare/+page.server.ts` | Add `popularComparisons` fetch for landing mode only. |
| `src/routes/(app)/compare/+page.svelte` | Restructure into landing/workspace branches; mount all components. |
| `src/lib/i18n/jce_en.json` / `jce_fr.json` | New `jce.compare.*` keys. |

---

### Task 1: `compareFlow.ts` pure helpers + tests

**Files:**
- Create: `apps/main/src/lib/components/compare/compareFlow.ts`
- Test: `apps/main/src/lib/components/compare/compareFlow.test.ts`

**Interfaces:**
- Consumes: `ComparisonType`, `ResolvedItem` from `$lib/types/jce`.
- Produces (all later tasks rely on these exact signatures):

```ts
export function scopeChainOf(type: ComparisonType | null | undefined): string[];
export function defaultItemRefType(type: ComparisonType | null | undefined): string;
export function requiredScopeStep(source: string, chain: string[]): string | null;
export function missingRequiredScope(source: string, chain: string[], selections: Record<string, string>): string | null;
export function scopeSummary(chain: string[], selections: Record<string, string>, labels: Record<string, string>): string;
export function sourceOptions(itemRefTypes: string[], labelFor: (key: string) => string): { value: string; label: string }[];
```

- [ ] **Step 1: Write the failing test** — `apps/main/src/lib/components/compare/compareFlow.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import {
	scopeChainOf,
	defaultItemRefType,
	requiredScopeStep,
	missingRequiredScope,
	scopeSummary,
	sourceOptions
} from './compareFlow';
import type { ComparisonType } from '$lib/types/jce';

const type = (config: Record<string, unknown>): ComparisonType => ({
	id: '1', name: 'T', slug: 't', item_label: 'Item', config: config as never
});

describe('compareFlow', () => {
	it('scopeChainOf returns config chain or []', () => {
		expect(scopeChainOf(type({ scope_chain: ['country', 'city'] }))).toEqual(['country', 'city']);
		expect(scopeChainOf(type({}))).toEqual([]);
		expect(scopeChainOf(null)).toEqual([]);
	});

	it('defaultItemRefType prefers config.default_item_ref_type then first item_ref_types then space', () => {
		expect(defaultItemRefType(type({ default_item_ref_type: 'country', item_ref_types: ['col_city', 'comparison'] }))).toBe('country');
		expect(defaultItemRefType(type({ item_ref_types: ['university', 'comparison'] }))).toBe('university');
		expect(defaultItemRefType(type({}))).toBe('space');
	});

	it('requiredScopeStep: course requires university, everything else null', () => {
		expect(requiredScopeStep('course', ['university'])).toBe('university');
		expect(requiredScopeStep('university', ['country', 'city'])).toBeNull();
		expect(requiredScopeStep('space', [])).toBeNull();
	});

	it('missingRequiredScope reports the missing step only while unset', () => {
		const chain = ['university'];
		expect(missingRequiredScope('course', chain, {})).toBe('university');
		expect(missingRequiredScope('course', chain, { university: 'u1' })).toBeNull();
		expect(missingRequiredScope('col_city', ['country', 'city'], {})).toBeNull();
	});

	it('scopeSummary joins selected labels in chain order', () => {
		const chain = ['country', 'state', 'city'];
		const selections = { country: 'c1', city: 'c3' };
		const labels = { country: 'Nigeria', city: 'Lagos' };
		expect(scopeSummary(chain, selections, labels)).toBe('Nigeria · Lagos');
		expect(scopeSummary(chain, {}, {})).toBe('');
	});

	it('sourceOptions maps item_ref_types to labelled options', () => {
		const opts = sourceOptions(['col_city', 'comparison'], (k) => `L:${k}`);
		expect(opts).toEqual([
			{ value: 'col_city', label: 'L:col_city' },
			{ value: 'comparison', label: 'L:comparison' }
		]);
	});
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd apps/main && bunx vitest run src/lib/components/compare/compareFlow.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `compareFlow.ts`**

```ts
import type { ComparisonType } from '$lib/types/jce';

export function scopeChainOf(type: ComparisonType | null | undefined): string[] {
	return (type?.config?.scope_chain as string[] | undefined) ?? [];
}

export function defaultItemRefType(type: ComparisonType | null | undefined): string {
	const configured = type?.config?.default_item_ref_type;
	if (typeof configured === 'string' && configured) return configured;
	return type?.config?.item_ref_types?.[0] ?? 'space';
}

export function requiredScopeStep(source: string, _chain: string[]): string | null {
	// Course search must be scoped to a university — same rule as +page.server.ts.
	if (source === 'course') return 'university';
	return null;
}

export function missingRequiredScope(
	source: string,
	chain: string[],
	selections: Record<string, string>
): string | null {
	const step = requiredScopeStep(source, chain);
	if (step && chain.includes(step) && !selections[step]) return step;
	return null;
}

export function scopeSummary(
	chain: string[],
	selections: Record<string, string>,
	labels: Record<string, string>
): string {
	return chain
		.filter((step) => selections[step] && labels[step])
		.map((step) => labels[step])
		.join(' · ');
}

export function sourceOptions(
	itemRefTypes: string[],
	labelFor: (key: string) => string
): { value: string; label: string }[] {
	return itemRefTypes.map((t) => ({ value: t, label: labelFor(t) }));
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd apps/main && bunx vitest run src/lib/components/compare/compareFlow.test.ts`
Expected: PASS (6 tests).

- [ ] **Step 5: Commit**

```bash
git add apps/main/src/lib/components/compare/compareFlow.ts apps/main/src/lib/components/compare/compareFlow.test.ts
git commit -m "feat(compare): pure flow helpers for stepper-lite picker"
```

---

### Task 2: i18n keys (en + fr)

**Files:**
- Modify: `apps/main/src/lib/i18n/jce_en.json`
- Modify: `apps/main/src/lib/i18n/jce_fr.json`

**Interfaces:**
- Produces keys used by Tasks 4–10 (exact key names listed below — implementers of later tasks must use them verbatim).

- [ ] **Step 1: Add keys to `jce_en.json`** — insert alphabetically among existing `jce.compare.*` keys (the file is a flat JSON object; keep one key per line, trailing commas correct):

```json
"jce.compare.landing.popular": "Popular comparisons",
"jce.compare.landing.see_all": "See all",
"jce.compare.landing.type_items": "{n} items",
"jce.compare.step.narrow": "Narrow",
"jce.compare.step.narrow_optional": "Narrow (optional)",
"jce.compare.step.select": "Select {count}/{max}",
"jce.compare.step.browse": "Browse",
"jce.compare.step.change_type": "Change type",
"jce.compare.step.edit_narrow": "Edit filters",
"jce.compare.narrow.skip": "Skip",
"jce.compare.narrow.done": "Done",
"jce.compare.narrow.required_prompt": "Choose a {step} to see {label}.",
"jce.compare.tray.hint_max": "Max {max} items — remove one to change picks",
"jce.compare.tray.pick_hint": "Pick {min}–{max} {label} to compare",
"jce.compare.filters.sort_display": "Sort & display",
```

- [ ] **Step 2: Add French equivalents to `jce_fr.json`** (same keys):

```json
"jce.compare.landing.popular": "Comparaisons populaires",
"jce.compare.landing.see_all": "Tout voir",
"jce.compare.landing.type_items": "{n} éléments",
"jce.compare.step.narrow": "Affiner",
"jce.compare.step.narrow_optional": "Affiner (optionnel)",
"jce.compare.step.select": "Sélection {count}/{max}",
"jce.compare.step.browse": "Parcourir",
"jce.compare.step.change_type": "Changer de type",
"jce.compare.step.edit_narrow": "Modifier les filtres",
"jce.compare.narrow.skip": "Ignorer",
"jce.compare.narrow.done": "Terminé",
"jce.compare.narrow.required_prompt": "Choisissez {step} pour voir {label}.",
"jce.compare.tray.hint_max": "Maximum {max} éléments — retirez-en un pour changer",
"jce.compare.tray.pick_hint": "Choisissez {min}–{max} {label} à comparer",
"jce.compare.filters.sort_display": "Tri et affichage",
```

- [ ] **Step 3: Validate JSON**

Run: `cd apps/main && bun -e "JSON.parse(require('fs').readFileSync('src/lib/i18n/jce_en.json','utf8')); JSON.parse(require('fs').readFileSync('src/lib/i18n/jce_fr.json','utf8')); console.log('ok')"`
Expected: `ok`

- [ ] **Step 4: Commit**

```bash
git add apps/main/src/lib/i18n/jce_en.json apps/main/src/lib/i18n/jce_fr.json
git commit -m "i18n(compare): stepper-lite copy keys (en, fr)"
```

---

### Task 3: `ScopeChain.svelte` — additive `bind:labels` prop

**Files:**
- Modify: `apps/main/src/lib/components/compare/ScopeChain.svelte`

**Interfaces:**
- Produces: optional `labels?: Record<string, string>` bindable prop — maps step → selected option title. `NarrowPanel` (Task 5) binds it. Existing `chain`, `selections`, `onChange` props unchanged; rendered UI unchanged.

- [ ] **Step 1: Add the prop and a label-sync effect**

In the `$props` destructure (currently lines 15–19), change to:

```ts
let {
	chain,
	selections = $bindable({}),
	labels = $bindable({}),
	onChange
} = $props<{
	chain: string[];
	selections: Record<string, string>;
	labels?: Record<string, string>;
	onChange: (key: string, value: string) => void;
}>();
```

After the existing trailing `$effect` (the options loader, lines 96–110), add a non-reactive snapshot var + the sync effect. The snapshot (`prevLabels`) is a plain `let` — deliberately NOT `$state` — so writing to `labels` never re-triggers this effect (avoids a self-dependency loop):

```ts
let prevLabels = '';

$effect(() => {
	// Surface id→title for selected steps so parents can render a summary
	// (e.g. "Nigeria · Lagos"). Unresolved ids simply omit until options load.
	const next: Record<string, string> = {};
	for (const step of chain) {
		const id = selections[step];
		if (!id) continue;
		const opt = options[step]?.find((o) => o.id === id);
		if (opt) next[step] = opt.title;
	}
	const serialized = JSON.stringify(next);
	if (serialized !== prevLabels) {
		prevLabels = serialized;
		labels = next;
	}
});
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "ScopeChain" ; echo "exit:$?"`
Expected: no output above `exit:1` (grep found nothing = no errors in this file).

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/ScopeChain.svelte
git commit -m "feat(compare): expose selected scope labels via bind:labels"
```

---

### Task 4: `StepHeader.svelte`

**Files:**
- Create: `apps/main/src/lib/components/compare/StepHeader.svelte`

**Interfaces:**
- Consumes: `t()` from `$lib/i18n`; lucide `X`, `ChevronRight`, `SlidersHorizontal` icons; `Component` type from `svelte`.
- Produces (used in Task 10 — exact prop names):

```ts
interface Props {
	typeName: string;            // selected comparison type name
	typeIcon?: Component;        // resolved lucide component
	showNarrow: boolean;         // type has a scope_chain
	narrowOpen: boolean;         // panel currently expanded
	narrowSummary: string;       // 'Nigeria · Lagos' or ''
	narrowRequired: boolean;     // required scope unset → no "optional" wording
	browseOnly: boolean;         // source === 'comparison' → step 3 is "Browse"
	selectSummary: string;       // e.g. '2/4' — '' when browseOnly
	onChangeType: () => void;
	onToggleNarrow: () => void;
}
```

- [ ] **Step 1: Write the component** — `apps/main/src/lib/components/compare/StepHeader.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import { ChevronRight, SlidersHorizontal, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface Props {
		typeName: string;
		typeIcon?: Component;
		showNarrow: boolean;
		narrowOpen: boolean;
		narrowSummary: string;
		narrowRequired: boolean;
		browseOnly: boolean;
		selectSummary: string;
		onChangeType: () => void;
		onToggleNarrow: () => void;
	}

	let {
		typeName,
		typeIcon: TypeIcon,
		showNarrow,
		narrowOpen,
		narrowSummary,
		narrowRequired,
		browseOnly,
		selectSummary,
		onChangeType,
		onToggleNarrow
	}: Props = $props();

	let narrowChipLabel = $derived(
		narrowSummary ||
			(narrowRequired ? t('jce.compare.step.narrow') : t('jce.compare.step.narrow_optional'))
	);
</script>

<nav
	class="flex flex-wrap items-center gap-1.5 text-sm"
	aria-label={t('jce.compare.heading')}
>
	<!-- Step 1: type -->
	<span
		class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 font-medium"
		style="background-color: var(--color-bg); border-color: var(--color-border); color: var(--color-text)"
	>
		{#if TypeIcon}<TypeIcon size={13} />{/if}
		{typeName}
		<button
			type="button"
			onclick={onChangeType}
			class="rounded-full p-0.5"
			style="color: var(--color-muted)"
			aria-label={t('jce.compare.step.change_type')}
		>
			<X size={13} />
		</button>
	</span>

	{#if showNarrow}
		<ChevronRight size={14} style="color: var(--color-muted)" />
		<!-- Step 2: narrow -->
		<button
			type="button"
			onclick={onToggleNarrow}
			class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 font-medium transition-colors"
			style="background-color: {narrowOpen ? 'var(--color-bg)' : 'var(--color-surface)'}; border-color: var(--color-border); color: var(--color-text)"
			aria-expanded={narrowOpen}
			aria-label={t('jce.compare.step.edit_narrow')}
		>
			<SlidersHorizontal size={13} />
			{narrowChipLabel}
		</button>
	{/if}

	<ChevronRight size={14} style="color: var(--color-muted)" />
	<!-- Step 3: select / browse -->
	<span
		class="inline-flex items-center rounded-full border px-3 py-1 font-medium"
		style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
	>
		{#if browseOnly}
			{t('jce.compare.step.browse')}
		{:else}
			{selectSummary || t('jce.compare.step.browse')}
		{/if}
	</span>
</nav>
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "StepHeader" ; echo "exit:$?"`
Expected: `exit:1` (no errors in this file).

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/StepHeader.svelte
git commit -m "feat(compare): step header chip indicator"
```

---

### Task 5: `NarrowPanel.svelte`

**Files:**
- Create: `apps/main/src/lib/components/compare/NarrowPanel.svelte`

**Interfaces:**
- Consumes: `ScopeChain` (incl. `bind:labels` from Task 3); i18n keys from Task 2.
- Produces (used in Task 10):

```ts
interface Props {
	chain: string[];                          // data.scopeChain
	selections: Record<string, string>;       // current scope selections (read-only down)
	labels?: Record<string, string>;          // bindable — id→title per step
	required: boolean;                        // missingRequiredScope != null
	requiredStepLabel?: string;               // e.g. 'University' — for title when required
	requiredItemLabel?: string;               // e.g. 'courses' — for title when required
	onChange: (step: string, value: string) => void;  // → page setScopeSelection
	onSkip: () => void;
	onDone: () => void;
	onClear: () => void;                      // → page clearScope
}
```

- [ ] **Step 1: Write the component** — `apps/main/src/lib/components/compare/NarrowPanel.svelte`:

```svelte
<script lang="ts">
	import ScopeChain from './ScopeChain.svelte';
	import { t } from '$lib/i18n';

	interface Props {
		chain: string[];
		selections: Record<string, string>;
		labels?: Record<string, string>;
		required: boolean;
		requiredStepLabel?: string;
		onChange: (step: string, value: string) => void;
		onSkip: () => void;
		onDone: () => void;
		onClear: () => void;
	}

	let {
		chain,
		selections,
		labels = $bindable({}),
		required,
		requiredStepLabel = '',
		requiredItemLabel = '',
		onChange,
		onSkip,
		onDone,
		onClear
	}: Props = $props();

	let hasSelection = $derived(chain.some((step) => selections[step]));
</script>

<section
	class="rounded-[var(--radius-card)] border p-4 space-y-3"
	style="background-color: var(--color-surface); border-color: var(--color-border)"
	aria-label={t('jce.compare.scope.title')}
>
	<div class="flex items-center justify-between gap-3">
		<h2 class="text-sm font-semibold" style="color: var(--color-text)">
			{#if required && requiredStepLabel}
				{t('jce.compare.narrow.required_prompt', 'Choose a {step} to see {label}.', {
					step: requiredStepLabel,
					label: requiredItemLabel || t('jce.compare.items_heading')
				})}
			{:else}
				{t('jce.compare.scope.title')}
			{/if}
		</h2>
		<div class="flex items-center gap-3">
			{#if hasSelection}
				<button type="button" onclick={onClear} class="text-xs" style="color: var(--color-muted)">
					{t('jce.common.clear')}
				</button>
			{/if}
			{#if !required}
				<button type="button" onclick={onSkip} class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.narrow.skip')}
				</button>
			{/if}
		</div>
	</div>

	<ScopeChain {chain} {selections} bind:labels onChange={(k, v) => onChange(k, v)} />

	{#if hasSelection}
		<div class="flex justify-end">
			<button
				type="button"
				onclick={onDone}
				class="px-4 py-1.5 rounded-[var(--radius-button)] text-sm font-medium"
				style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
			>
				{t('jce.compare.narrow.done')}
			</button>
		</div>
	{/if}
</section>
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "NarrowPanel" ; echo "exit:$?"`
Expected: `exit:1`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/NarrowPanel.svelte
git commit -m "feat(compare): collapsible narrow panel wrapping scope chain"
```

---

### Task 6: `CompareTray.svelte`

**Files:**
- Create: `apps/main/src/lib/components/compare/CompareTray.svelte`

**Interfaces:**
- Consumes: `ResolvedItem` from `$lib/types/jce`; `Component` from `svelte`; lucide `X`, `ArrowRight`, `Sparkles`.
- Produces (used in Task 10):

```ts
interface Props {
	items: ResolvedItem[];       // selectedItems
	maxItems: number;
	authed: boolean;
	creating: boolean;
	aiEnabled: boolean;          // feature.jce.ai_quick && canAiQuick
	aiActive: boolean;           // panel open → pressed state
	hint?: string;               // already-translated hint line ('' hides)
	itemLabel: string;           // e.g. 'universities' — for the pick hint
	itemIcon?: Component;        // source icon for the chips
	onRemove: (item: ResolvedItem) => void;
	onCompare: () => void;
	onLogin: () => void;
	onToggleAi: () => void;
}
```

The tray is a **plain bar** — the page's sticky wrapper (Task 10) owns the `sticky bottom-0` boundary, so the component itself carries no sticky positioning (avoids nested sticky).

- [ ] **Step 1: Write the component** — `apps/main/src/lib/components/compare/CompareTray.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import type { ResolvedItem } from '$lib/types/jce';
	import { ArrowRight, Sparkles, X } from 'lucide-svelte';
	import type { Component } from 'svelte';

	interface Props {
		items: ResolvedItem[];
		maxItems: number;
		authed: boolean;
		creating: boolean;
		aiEnabled: boolean;
		aiActive: boolean;
		hint?: string;
		itemLabel: string;
		itemIcon?: Component;
		onRemove: (item: ResolvedItem) => void;
		onCompare: () => void;
		onLogin: () => void;
		onToggleAi: () => void;
	}

	let {
		items,
		maxItems,
		authed,
		creating,
		aiEnabled,
		aiActive,
		hint = '',
		itemLabel,
		itemIcon: ItemIcon,
		onRemove,
		onCompare,
		onLogin,
		onToggleAi
	}: Props = $props();

	let canCompare = $derived(items.length >= 2);
</script>

<div
	class="border-t px-4 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))] sm:px-6"
	style="background-color: var(--color-bg); border-color: var(--color-border)"
	role="region"
	aria-label={t('jce.compare.heading')}
>
	{#if hint}
		<p class="mb-2 text-xs" style="color: var(--color-muted)">{hint}</p>
	{/if}
	<div class="flex items-center gap-3">
		<div class="flex min-w-0 flex-1 flex-wrap items-center gap-1.5">
			{#if items.length === 0}
				<span class="text-sm" style="color: var(--color-muted)">
					{t('jce.compare.tray.pick_hint', 'Pick {min}–{max} {label} to compare', {
						min: '2',
						max: String(maxItems),
						label: itemLabel
					})}
				</span>
			{:else}
				{#each items as item (item.id)}
					<span
						class="inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm"
						style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
					>
						{#if ItemIcon}<ItemIcon size={12} />{/if}
						<span class="max-w-40 truncate">{item.title}</span>
						<button
							type="button"
							onclick={() => onRemove(item)}
							class="rounded-full p-0.5"
							style="color: var(--color-muted)"
							aria-label="{t('jce.common.delete')} {item.title}"
						>
							<X size={13} />
						</button>
					</span>
				{/each}
			{/if}
		</div>

		<div class="flex shrink-0 items-center gap-2">
			{#if items.length > 0}
				<span class="text-xs whitespace-nowrap" style="color: var(--color-muted)">
					{items.length}/{maxItems}
				</span>
			{/if}
			{#if authed}
				{#if aiEnabled}
					<button
						type="button"
						onclick={onToggleAi}
						aria-pressed={aiActive}
						class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-2 text-sm font-medium"
						style="background-color: {aiActive ? 'var(--color-bg)' : 'var(--color-surface)'}; border-color: var(--color-border); color: var(--color-text)"
					>
						<Sparkles size={14} />
						<span class="hidden sm:inline">{t('jce.compare.ai_quick.button')}</span>
					</button>
				{/if}
				<button
					type="button"
					disabled={!canCompare || creating}
					onclick={onCompare}
					class="inline-flex items-center gap-2 rounded-[var(--radius-button)] px-5 py-2 text-sm font-medium disabled:opacity-50"
					style="background-color: var(--color-surface); border: 1px solid var(--color-border); color: var(--color-text)"
				>
					{creating ? t('jce.compare.creating') : t('jce.compare.compare_now')}
					<ArrowRight size={15} />
				</button>
			{:else}
				<button
					type="button"
					disabled={!canCompare}
					onclick={onLogin}
					class="inline-flex items-center gap-2 rounded-[var(--radius-button)] px-5 py-2 text-sm font-medium disabled:opacity-50"
					style="background-color: var(--color-surface); border: 1px solid var(--color-border); color: var(--color-text)"
				>
					{t('jce.compare.login_to_compare')}
				</button>
			{/if}
		</div>
	</div>
</div>
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "CompareTray" ; echo "exit:$?"`
Expected: `exit:1`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/CompareTray.svelte
git commit -m "feat(compare): sticky compare tray with chips and actions"
```

---

### Task 7: `AiQuickPanel.svelte` — extract the AI form

**Files:**
- Create: `apps/main/src/lib/components/compare/AiQuickPanel.svelte`

**Interfaces:**
- Consumes: `CompareEntitiesResult` from `$lib/ai/compareApi`; `t()`.
- Produces (used in Task 10):

```ts
interface Props {
	loading: boolean;
	result: CompareEntitiesResult | null;
	error: string;                     // already-translated or '' 
	onRun: (opts: { focus: string; fields: string; createPost: boolean }) => void;
	onClose: () => void;
}
```

- [ ] **Step 1: Write the component** — moves the existing AI form markup from `+page.svelte` lines 744–857 into `apps/main/src/lib/components/compare/AiQuickPanel.svelte`, owning `focus`/`fields`/`createPost` locally and emitting via `onRun`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import type { CompareEntitiesResult } from '$lib/ai/compareApi';
	import { X } from 'lucide-svelte';

	interface Props {
		loading: boolean;
		result: CompareEntitiesResult | null;
		error: string;
		onRun: (opts: { focus: string; fields: string; createPost: boolean }) => void;
		onClose: () => void;
	}

	let { loading, result, error, onRun, onClose }: Props = $props();

	let focus = $state('');
	let fields = $state('');
	let createPost = $state(false);
</script>

<div
	class="max-h-[60vh] overflow-y-auto rounded-[var(--radius-card)] border p-3 space-y-3 text-sm"
	style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
	role="region"
	aria-label={t('jce.compare.ai_quick.button')}
>
	<div class="flex items-center justify-between gap-2">
		<span class="text-xs font-medium" style="color: var(--color-muted)">
			{t('jce.compare.ai_quick.button')}
		</span>
		<button
			type="button"
			onclick={onClose}
			class="rounded-full p-0.5"
			style="color: var(--color-muted)"
			aria-label={t('jce.common.close')}
		>
			<X size={14} />
		</button>
	</div>
	<div class="grid gap-3 sm:grid-cols-2">
		<label class="space-y-1">
			<span class="text-xs font-medium" style="color: var(--color-muted)">
				{t('jce.compare.ai_quick.focus_label')}
			</span>
			<input
				type="text"
				bind:value={focus}
				placeholder={t('jce.compare.ai_quick.focus_placeholder')}
				class="w-full rounded-[var(--radius-button)] border px-3 py-1.5 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
			/>
		</label>
		<label class="space-y-1">
			<span class="text-xs font-medium" style="color: var(--color-muted)">
				{t('jce.compare.ai_quick.fields_label')}
			</span>
			<input
				type="text"
				bind:value={fields}
				placeholder={t('jce.compare.ai_quick.fields_placeholder')}
				class="w-full rounded-[var(--radius-button)] border px-3 py-1.5 text-sm"
				style="border-color: var(--color-border); background-color: var(--color-bg); color: var(--color-text)"
			/>
		</label>
	</div>
	<div class="flex items-center gap-3 flex-wrap">
		<label class="inline-flex items-center gap-2 text-xs" style="color: var(--color-text)">
			<input type="checkbox" bind:checked={createPost} />
			{t('jce.compare.ai_quick.create_post')}
		</label>
		<button
			type="button"
			disabled={loading}
			onclick={() => onRun({ focus, fields, createPost })}
			class="inline-flex items-center gap-2 px-4 py-1.5 rounded-[var(--radius-button)] text-sm font-medium disabled:opacity-50"
			style="background-color: var(--color-bg); border: 1px solid var(--color-border); color: var(--color-text)"
		>
			{loading ? t('jce.compare.ai_quick.running') : t('jce.compare.ai_quick.run')}
		</button>
	</div>

	{#if error}
		<p class="text-xs" style="color: #ef4444">{error}</p>
	{/if}

	{#if result}
		<div class="space-y-2">
			<div class="flex items-center gap-2 flex-wrap">
				<span
					class="inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium"
					style="border-color: var(--color-border); color: var(--color-text)"
				>
					{t(`jce.compare.ai_quick.tier.${result.tier}`, result.tier)}
				</span>
				{#if result.type_name}
					<span class="text-xs" style="color: var(--color-muted)">{result.type_name}</span>
				{/if}
			</div>

			{#if result.existing_dimensions?.length}
				<div class="flex flex-wrap gap-1.5">
					{#each result.existing_dimensions as dim (dim)}
						<span
							class="rounded-full px-2 py-0.5 text-xs"
							style="background-color: var(--color-bg); color: var(--color-text)"
						>{dim}</span>
					{/each}
				</div>
			{/if}

			{#if result.missing_dimensions?.length}
				<p class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.ai_quick.missing_dimensions')}:
					{result.missing_dimensions.join(', ')}
				</p>
			{/if}

			{#if result.presearch_level || result.presearch_summary}
				<p class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.ai_quick.presearch')}:
					{result.presearch_summary ?? result.presearch_level}
				</p>
			{/if}

			{#if result.content_id}
				<p class="text-xs" style="color: var(--color-text)">
					{t('jce.compare.ai_quick.draft_created')}
					{#if result.comparison?.comparison?.slug}
						<a
							href="/compare/{result.comparison.comparison.slug}"
							class="underline"
							style="color: var(--color-accent, currentColor)"
						>
							{result.comparison.comparison.title}
						</a>
					{:else}
						<code>{result.content_id}</code>
					{/if}
				</p>
			{/if}
		</div>
	{/if}
</div>
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "AiQuickPanel" ; echo "exit:$?"`
Expected: `exit:1`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/AiQuickPanel.svelte
git commit -m "feat(compare): extract AI quick compare panel"
```

---

### Task 8: `TypeCardGrid.svelte`

**Files:**
- Create: `apps/main/src/lib/components/compare/TypeCardGrid.svelte`

**Interfaces:**
- Consumes: `ComparisonType` from `$lib/types/jce`; `Component` from `svelte`.
- Produces (used in Task 10):

```ts
interface Props {
	types: ComparisonType[];
	iconFor: (type: ComparisonType) => Component;  // page supplies lucide map
	onSelect: (type: ComparisonType) => void;
}
```

- [ ] **Step 1: Write the component** — `apps/main/src/lib/components/compare/TypeCardGrid.svelte`:

```svelte
<script lang="ts">
	import { t } from '$lib/i18n';
	import type { ComparisonType } from '$lib/types/jce';
	import type { Component } from 'svelte';

	interface Props {
		types: ComparisonType[];
		iconFor: (type: ComparisonType) => Component;
		onSelect: (type: ComparisonType) => void;
	}

	let { types, iconFor, onSelect }: Props = $props();
</script>

<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
	{#each types as cmpType (cmpType.id)}
		{@const TypeIcon = iconFor(cmpType)}
		<button
			type="button"
			onclick={() => onSelect(cmpType)}
			class="flex items-start gap-3 rounded-[var(--radius-card)] border p-4 text-left transition-colors hover:border-[var(--color-text)]"
			style="background-color: var(--color-surface); border-color: var(--color-border); color: var(--color-text)"
		>
			<span
				class="grid h-10 w-10 shrink-0 place-items-center rounded-[var(--radius-button)]"
				style="background-color: var(--color-bg); color: var(--color-muted)"
			>
				<TypeIcon size={20} />
			</span>
			<span class="min-w-0 flex-1">
				<span class="block font-medium" style="color: var(--color-text)">{cmpType.name}</span>
				{#if cmpType.description}
					<span class="mt-0.5 block text-sm line-clamp-2" style="color: var(--color-muted)">
						{cmpType.description}
					</span>
				{/if}
				{#if cmpType.item_count}
					<span class="mt-1 block text-xs" style="color: var(--color-muted)">
						{t('jce.compare.landing.type_items', '{n} items', { n: String(cmpType.item_count) })}
					</span>
				{/if}
			</span>
		</button>
	{/each}
</div>
```

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep -i "TypeCardGrid" ; echo "exit:$?"`
Expected: `exit:1`.

- [ ] **Step 3: Commit**

```bash
git add apps/main/src/lib/components/compare/TypeCardGrid.svelte
git commit -m "feat(compare): landing type-card grid"
```

---

### Task 9: `+page.server.ts` — popular comparisons for landing

**Files:**
- Modify: `apps/main/src/routes/(app)/compare/+page.server.ts`

**Interfaces:**
- Produces: `popularComparisons: JCEComparisonSummary[]` in the returned data (consumed in Task 10).

- [ ] **Step 1: Add the fetch**

After the `myComparisons` block (ends ~line 275, before the `canAiQuick` block), add:

```ts
	// Landing-mode discovery strip — published public detailed comparisons.
	// Only fetched when the picker is in landing mode (no ?type, not the
	// "mine" tab). Best-effort: failure hides the strip.
	let popularComparisons: JCEComparisonSummary[] = [];
	if (!typeSlug && !mine) {
		try {
			const res = await apiList<JCEComparisonSummary>(
				ctx,
				'/jce/comparisons?filter_depth=detailed&filter_status=published&filter_visibility=public&page_size=6&sort=popularity'
			);
			popularComparisons = mapComparisonTypes(res.items, allTypes);
		} catch {
			popularComparisons = [];
		}
	}
```

Then add `popularComparisons,` to the returned object (after `myMeta,` ~line 318).

Note: `mine` is declared at line 260 (`searchParams.get('mine') === 'true'`) — place the new block after it so the variable is in scope.

- [ ] **Step 2: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep "compare/+page.server" ; echo "exit:$?"`
Expected: `exit:1`.

- [ ] **Step 3: Commit**

```bash
git add "apps/main/src/routes/(app)/compare/+page.server.ts"
git commit -m "feat(compare): fetch popular comparisons for landing mode"
```

---

### Task 10: `+page.svelte` — full restructure

**Files:**
- Modify: `apps/main/src/routes/(app)/compare/+page.svelte` (rewrite template; script changes listed below)

**Interfaces:**
- Consumes: everything from Tasks 1–9: `scopeChainOf`, `missingRequiredScope`, `scopeSummary`, `sourceOptions`, `StepHeader`, `NarrowPanel`, `CompareTray`, `AiQuickPanel`, `TypeCardGrid`, `data.popularComparisons`, new i18n keys; bits-ui `Popover` for the sort/display menu; `PaginationControls` from `$lib/components/ui`.
- Produces: final page — landing/workspace modes per spec §3.

- [ ] **Step 1: Script changes** — keep all existing state/functions; add/modify:

New imports (top of script):

```ts
import StepHeader from '$lib/components/compare/StepHeader.svelte';
import NarrowPanel from '$lib/components/compare/NarrowPanel.svelte';
import CompareTray from '$lib/components/compare/CompareTray.svelte';
import AiQuickPanel from '$lib/components/compare/AiQuickPanel.svelte';
import TypeCardGrid from '$lib/components/compare/TypeCardGrid.svelte';
import { PaginationControls } from '$lib/components/ui';
import { Popover } from 'bits-ui';
import {
	missingRequiredScope,
	scopeSummary,
	sourceOptions
} from '$lib/components/compare/compareFlow';
```

New state + derived (after existing derived block ~line 134):

```ts
let popularComparisons = $derived(data.popularComparisons ?? []);
let scopeLabels = $state<Record<string, string>>({});
let showNarrow = $derived((data.scopeChain ?? []).length > 0);
let missingScope = $derived(
	missingRequiredScope(source, data.scopeChain ?? [], scopeSelections)
);
let requiredScopeMissing = $derived(missingScope !== null);
let narrowSummary = $derived(
	scopeSummary(data.scopeChain ?? [], scopeSelections, scopeLabels)
);
let isNestedSource = $derived(source === 'comparison');
let refTypeOptions = $derived(sourceOptions(itemRefTypes, sourceLabel));
let trayHint = $derived(
	selectedItems.length >= maxItems ? t('jce.compare.tray.hint_max', undefined, { max: String(maxItems) }) : ''
);
let selectSummary = $derived(t('jce.compare.step.select', undefined, {
	count: String(selectedItems.length),
	max: String(maxItems)
}));

let narrowDismissed = $state(false);
// Open immediately when the type is scoped and no scope params are set —
// this is also the SSR/first-render state (no collapsed→open flash).
let narrowOpen = $state(
	(data.scopeChain ?? []).length > 0 && Object.keys(data.scopeParams ?? {}).length === 0
);
```

Auto-open the narrow panel on subsequent client-side navigations — **open-only** effect: it must never force-close (a selection change mid-chain would otherwise collapse the panel after the first pick):

```ts
$effect(() => {
	if (showNarrow && !narrowDismissed && Object.keys(scopeSelections).length === 0) {
		narrowOpen = true;
	}
});
```

Reset narrow state on type/source switches so a new scoped type auto-opens again — inside `selectType` and `selectSource`, add to the existing reset blocks:

```ts
	narrowDismissed = false;
	scopeLabels = {};
```

`runAiQuick` signature change — the AI form now lives in `AiQuickPanel` and calls back with `{ focus, fields, createPost }`. Replace the existing function (lines 437–465) so it takes the payload and drops the component-local input refs:

```ts
async function runAiQuick(opts: { focus: string; fields: string; createPost: boolean }) {
	if (!selectedType || selectedItems.length < 2 || aiQuickLoading) return;
	aiQuickLoading = true;
	aiQuickError = '';
	aiQuickResult = null;
	try {
		const fields = opts.fields
			.split(',')
			.map((f) => f.trim())
			.filter(Boolean);
		const res = await compareEntities({
			entity_type: selectedType.slug,
			entity_ids: selectedItems.map((i) => i.title),
			...(fields.length > 0 ? { comparison_fields: fields } : {}),
			...(opts.focus.trim() ? { focus: opts.focus.trim() } : {}),
			...(user?.current_geo_space_id ? { geo_space_id: user.current_geo_space_id } : {}),
			create_post: opts.createPost
		});
		if (res.error || !res.data) {
			aiQuickError = res.error?.message ?? t('jce.compare.ai_quick.errors.unknown');
		} else {
			aiQuickResult = res.data;
		}
	} catch (e) {
		aiQuickError = e instanceof Error ? e.message : t('jce.compare.ai_quick.errors.unknown');
	} finally {
		aiQuickLoading = false;
	}
}
```

Delete now-unused state: `aiQuickFocus`, `aiQuickFields`, `aiQuickCreatePost` (lines 109–111). Keep `aiQuickOpen`, `aiQuickLoading`, `aiQuickResult`, `aiQuickError`, `aiQuickEnabled`.

Add narrow-panel callbacks. The collapse trigger is **Done, Skip, or selecting the deepest chain step** — NOT any pick (a country→state→city user must stay in the panel through intermediate steps; this refines spec §3.4):

```ts
function skipNarrow() {
	narrowDismissed = true;
	narrowOpen = false;
}
function doneNarrow() {
	narrowDismissed = true;
	narrowOpen = false;
}
function toggleNarrow() {
	narrowOpen = !narrowOpen;
	if (!narrowOpen) narrowDismissed = true;
}
function narrowChange(step: string, value: string) {
	setScopeSelection(step, value);
	const chain = data.scopeChain ?? [];
	if (value && step === chain[chain.length - 1]) doneNarrow();
}
function backToLanding() {
	const url = new URL(page.url);
	url.searchParams.delete('type');
	url.searchParams.delete('source');
	url.searchParams.delete('nested_type');
	url.searchParams.delete('q');
	url.searchParams.delete('sort');
	url.searchParams.delete('page_size');
	url.searchParams.delete('page');
	for (const step of data.scopeChain ?? []) {
		url.searchParams.delete(scopeParamFor(step));
	}
	goto(url, { replaceState: true });
}
```

- [ ] **Step 2: Template restructure** — replace the `{#if mine} … {:else} … {/if}` block (lines 583–1075) with three branches. Keep the existing `ListPageBar`/`UrlTabs` and error banner at top, but render tabs only outside workspace: wrap the existing `{#if user}<ListPageBar>…` block in `{#if user && (!selectedType || mine)}` — the `|| mine` covers `?type=x&mine=true`, where the mine grid still shows and the user needs a way back to "new".

Also remove `X` from the lucide import list (line 20) — after the restructure it is unused (`Sparkles` stays: the "Promote" button in the mine grid still uses it).

Branch A — `{:else if !selectedType}` (landing), replacing the old type-button-cloud section:

```svelte
	<section class="space-y-3">
		<h2 class="text-sm font-semibold" style="color: var(--color-text)">
			{t('jce.compare.choose_type')}
		</h2>
		<TypeCardGrid {types} iconFor={(t2) => typeIcon(t2)} onSelect={selectType} />
	</section>

	{#if popularComparisons.length > 0}
		<section class="space-y-3">
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-semibold" style="color: var(--color-text)">
					{t('jce.compare.landing.popular')}
				</h2>
				<a href="/comparisons" class="text-xs" style="color: var(--color-muted)">
					{t('jce.compare.landing.see_all')}
				</a>
			</div>
			<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
				{#each popularComparisons as item (item.id)}
					<a
						href="/comparisons/{item.slug}"
						class="rounded-[var(--radius-card)] border p-4 transition-colors hover:border-[var(--color-text)]"
						style="background-color: var(--color-surface); border-color: var(--color-border)"
					>
						<p class="font-medium truncate" style="color: var(--color-text)">{item.title}</p>
						{#if item.type_name}
							<span
								class="mt-2 inline-block rounded px-2 py-0.5 text-xs"
								style="background-color: var(--color-bg); color: var(--color-muted)"
							>{item.type_name}</span>
						{/if}
					</a>
				{/each}
			</div>
		</section>
	{/if}
```

Add a `typeIcon` helper next to `sourceIcon` — resolves a type's icon via the first non-`comparison` item_ref_type, falling back to `GitCompare`:

```ts
function typeIcon(cmpType: ComparisonType) {
	const refTypes = (cmpType.config?.item_ref_types as string[] | undefined) ?? [];
	const primary = refTypes.find((r) => r !== 'comparison') ?? 'comparison';
	return sourceIcon(primary);
}
```

Branch B — workspace (`{:else}`). Order: `StepHeader` → optional `NarrowPanel` → filter row → results count → grid/prompt → pagination → sticky zone (AiQuickPanel + CompareTray). The grid markup is the **existing** `{#each pagedItems}` card code, unchanged — only wrapped differently:

```svelte
	<StepHeader
		typeName={selectedType.name}
		typeIcon={typeIcon(selectedType)}
		{showNarrow}
		{narrowOpen}
		{narrowSummary}
		narrowRequired={requiredScopeMissing}
		browseOnly={isNestedSource}
		{selectSummary}
		onChangeType={backToLanding}
		onToggleNarrow={toggleNarrow}
	/>

	{#if showNarrow && narrowOpen}
		<NarrowPanel
			chain={data.scopeChain ?? []}
			selections={scopeSelections}
			bind:labels={scopeLabels}
			required={requiredScopeMissing}
			requiredStepLabel={missingScope ? sourceLabel(missingScope) : ''}
			requiredItemLabel={sourcePlural(source)}
			onChange={narrowChange}
			onSkip={skipNarrow}
			onDone={doneNarrow}
			onClear={clearScope}
		/>
	{/if}
```

`selections` is passed **down only** (not `bind:`) — `scopeSelections` is `$state` owned by the page, driven by an `$effect` syncing `data.scopeParams`, and all mutations go through `narrowChange` → `setScopeSelection` (URL navigation). `labels` is bound upward (page → NarrowPanel → ScopeChain) so the summary chip can render titles. `sourceLabel(missingScope)` resolves e.g. `university` → `jce.compare.sources.university.label` ("University") — the `sources.*` keys exist for every resolver type; `scope.select_*` keys are phrased for the dropdown ("Select university") and don't compose into the prompt sentence.

Filter row — replace the old standalone scope-chain section AND the old controls row (lines 883–939) with:

```svelte
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
		<div class="relative w-full sm:flex-1">
			<Search size={16} class="absolute left-3 top-1/2 -translate-y-1/2" style="color: var(--color-muted)" />
			<input
				type="text"
				value={query}
				oninput={(e) => setSearch(e.currentTarget.value)}
				placeholder={t('jce.compare.search_placeholder', 'Search {label}...', { label: sourcePlural(source) })}
				class="w-full rounded-[var(--radius-button)] border bg-[var(--color-surface)] pl-9 pr-4 py-2 text-sm outline-none"
				style="border-color: var(--color-border); color: var(--color-text)"
			/>
		</div>

		<div class="flex gap-2 items-center">
			{#if itemRefTypes.length > 1}
				<JceSelect
					options={refTypeOptions}
					value={source}
					onchange={(value) => selectSource(value)}
					class="w-auto"
				/>
			{/if}

			<Popover.Root>
				<Popover.Trigger
					class="inline-flex items-center gap-1.5 rounded-[var(--radius-button)] border px-3 py-2 text-sm font-medium"
					style="border-color: var(--color-border); background-color: var(--color-surface); color: var(--color-text)"
				>
					{t('jce.compare.filters.sort_display')}
				</Popover.Trigger>
				<Popover.Content
					class="z-50 w-56 space-y-3 rounded-[var(--radius-card)] border p-3"
					style="background-color: var(--color-surface); border-color: var(--color-border)"
					sideOffset={6}
				>
					<div class="space-y-1.5">
						<span class="text-xs font-medium" style="color: var(--color-muted)">{t('jce.comparisons.sort_placeholder')}</span>
						<JceSelect options={sortOptions} value={sortBy} onchange={(v) => setSort(v)} class="w-full" />
					</div>
					{#if isNestedSource && allowNestedComparisons && nestedTypeOptions.length > 1}
						<div class="space-y-1.5">
							<span class="text-xs font-medium" style="color: var(--color-muted)">{t('jce.compare.sources.comparison.label')}</span>
							<JceSelect options={nestedTypeOptions} value={nestedType} onchange={(v) => setNestedType(v)} class="w-full" />
						</div>
					{/if}
					<div class="space-y-1.5">
						<span class="text-xs font-medium" style="color: var(--color-muted)">{t('jce.common.per_page')}</span>
						<JceSelect options={pageSizeSelectOptions} value={String(pageSize)} onchange={(v) => setPageSize(parseInt(v, 10))} class="w-full" />
					</div>
				</Popover.Content>
			</Popover.Root>
		</div>
	</div>
```

Results-count paragraph: keep as-is.

Grid area — replace the old `{#if pagedItems.length === 0} … {/if}` block's outer condition to handle the required-scope prompt:

```svelte
	{#if requiredScopeMissing}
		<div class="rounded-[var(--radius-card)] border p-8 text-center" style="background-color: var(--color-surface); border-color: var(--color-border)">
			<Icon size={40} class="mx-auto mb-3" style="color: var(--color-muted)" />
			<p style="color: var(--color-muted)">
				{t('jce.compare.narrow.required_prompt', undefined, {
					step: missingScope ? sourceLabel(missingScope) : '',
					label: sourcePlural(source)
				})}
			</p>
		</div>
	{:else if pagedItems.length === 0}
		<!-- existing empty-state card, unchanged -->
	{:else}
		<!-- existing card grid, unchanged (both isNestedSource link-cards and select-cards branches) -->
		<!-- replace the hand-rolled pagination block (keep its {#if pageCount > 1} guard) with: -->
		<PaginationControls page={effectivePage} totalPages={pageCount} onPageChange={setPage} />
	{/if}
```

Sticky zone — last element inside the page container, replacing the deleted selected-panel + AI block (old lines 684–881). Hidden for nested source. The outer wrapper is the single sticky boundary; its negative margins stretch it to the padded container's edges and it re-pads children, so `CompareTray`'s own horizontal padding keeps working unchanged:

```svelte
	{#if !isNestedSource}
		<div class="sticky bottom-0 z-40 -mx-4 space-y-2 sm:-mx-6" style="background-color: var(--color-bg)">
			{#if aiQuickOpen && aiQuickEnabled}
				<div class="px-4 sm:px-6">
					<AiQuickPanel
						loading={aiQuickLoading}
						result={aiQuickResult}
						error={aiQuickError}
						onRun={runAiQuick}
						onClose={() => (aiQuickOpen = false)}
					/>
				</div>
			{/if}
			<CompareTray
				items={selectedItems}
				{maxItems}
				authed={!!user}
				{creating}
				aiEnabled={aiQuickEnabled}
				aiActive={aiQuickOpen}
				hint={trayHint}
				itemLabel={sourcePlural(source)}
				itemIcon={Icon}
				onRemove={removeSelected}
				onCompare={create}
				onLogin={saveGuestSelectionAndLogin}
				onToggleAi={() => (aiQuickOpen = !aiQuickOpen)}
			/>
		</div>
	{/if}
```

Branch A also keeps the `{#if mine}` content exactly as today (the my-comparisons grid) — restructure to `{#if mine} … {:else if !selectedType} …landing… {:else} …workspace… {/if}`.

Delete the now-dead template pieces: the `{#if selectedType && itemRefTypes.length > 1}` source section, the `{#if source !== 'comparison'}` selected-panel section (incl. chips — replaced by tray), the old `{#if data.scopeChain?.length}` scope section, and the old controls row.

- [ ] **Step 3: Check**

Run: `cd apps/main && bunx svelte-check --tsconfig ./tsconfig.json 2>&1 | grep "compare/+page" ; echo "exit:$?"`
Expected: `exit:1` — no errors in the page.

- [ ] **Step 4: Commit**

```bash
git add "apps/main/src/routes/(app)/compare/+page.svelte"
git commit -m "refactor(compare): stepper-lite landing + workspace layout"
```

---

### Task 11: Final verification

**Files:** none (verification only).

- [ ] **Step 1: Full check + tests**

Run: `cd apps/main && bun run check`
Expected: no new errors in touched files (compare/, compareFlow, i18n untouched by check).

Run: `cd apps/main && bun run test`
Expected: all tests pass (incl. 6 new compareFlow tests).

- [ ] **Step 2: Manual matrix smoke** (requires running dev server + backend — do if available, else note as pending):

| Case | Expected |
|---|---|
| `/compare` | Landing: tabs (authed), type cards, popular strip |
| `/compare?type=universities` | Workspace, narrow panel open, step chips visible |
| pick country (chain: country→state→city) | panel stays open, state/city unlock |
| pick city (deepest step) | panel auto-collapses, step-2 chip shows "Nigeria · Lagos" |
| pick a step then Done | panel collapses, chip shows partial summary |
| change type after skipping narrow | new type's panel auto-opens again |
| `/compare?type=courses` | narrow panel open without Skip; grid shows required prompt |
| select 2 items | tray shows chips + enabled Compare |
| select max items | tray hint appears; grid cards disabled |
| guest, 2 items | tray shows "Log in to compare" |
| `?type=X&preselect=a,b` | workspace with tray pre-filled, narrow collapsed |
| `?type=X&source=comparison` | browse mode: no tray, link cards, nested filter in popover |
| `?mine=true` | landing "My comparisons" |

- [ ] **Step 3: Final commit** (only if fixes were needed)

```bash
git add -A apps/main/src/routes/\(app\)/compare apps/main/src/lib/components/compare apps/main/src/lib/i18n
git commit -m "fix(compare): stepper-lite polish from verification"
```

---

## Spec coverage map

| Spec § | Task |
|---|---|
| §3.2 Landing (type cards, popular strip, tabs) | 8, 9, 10 |
| §3.3 Step header | 4, 10 |
| §3.4 Narrow panel (auto-expand, skip, required prompt) | 1, 3, 5, 10 |
| §3.5 Unified filter row (search + source + popover) | 10 |
| §3.6 Sticky tray (chips, count, Compare, AI, guest CTA) | 6, 7, 10 |
| §3.7 Card affordances + max hint | 6, 10 (hint in tray; cards keep existing disabled treatment + "+ Add" not added — see note) |
| §4 State matrix | 10 + manual matrix in 11 |
| §5 Components | 1–10 |
| §7 Testing | 1, 11 |

**Note on §3.7 "+ Add" affordance:** kept the existing check-state cards; adding a hover "+ Add" label is a one-line enhancement deferred — the tray hint covers the "max reached" discoverability gap, which was the actual problem.
