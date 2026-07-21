# User Hub — Plan 1: Infrastructure + Settings Core

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax. Load the **jc-main-develop** and **svelte** skills for the component tasks.

**Goal:** Ship a working `/settings` surface — the two-registry route machinery, the shared primitives, and three real form sections (profile, account, privacy) — as the foundation the rest of the hub builds on.

**Architecture:** A dynamic `/settings/[section]` route dispatches to a server-only loader registry and renders a client component from a client-meta registry (the two-registry split keeps server fetch code out of the browser bundle). Form sections wrap fields in a shared `SettingsSection` chrome bound to a `useDirtyForm` rune (dirty tracking + save/discard); discrete actions use an `optimisticAction` helper.

**Tech Stack:** SvelteKit 2 (adapter-node, Bun), Svelte 5 strict Runes, Bits UI, Tailwind v4, Vitest 4 + @testing-library/svelte 5 + jsdom.

**Spec:** `docs/superpowers/specs/2026-07-21-user-hub-design.md` (§3 architecture, §4 primitives, §6 settings). Read it first.

**This plan covers spec modules:** infrastructure (§3), all primitives (§4), and the `profile`/`account`/`privacy` sections (§6). It also lands the `devices` registry entry as a `backendReady:false` example (§5) without its component. Plans 2–3 cover the remaining sections and the dashboard family.

## Global Constraints

- **App root:** paths are under `apps/main/` unless noted. Backend `http://localhost:8080`, API under `/api/v1`.
- **Svelte 5 strict Runes only:** `$state`, `$derived`, `$props`, `$effect`. FORBIDDEN: `export let`, `$:`, `on:click` (use `onclick`), `createEventDispatcher`, `localStorage`/`sessionStorage` for state.
- **Bits UI** dot-notation for interactive primitives (`Select.*`, `Dialog.*`, `Tabs.*`).
- **Tailwind v4:** colors via `var(--color-*)`; dark mode via `[data-theme]`; never hardcode hex.
- **i18n:** every string via `t('key')` (`import { t } from '$lib/i18n'`); add keys to BOTH `src/lib/i18n/en.json` and `fr.json`.
- **Rule 16:** all data from `/api/v1`; `load()` never returns static data. Server reads via `$lib/server/api` (`apiContext`, `apiGet`); client mutations via `$lib/api/client` (`api.patch/post/delete` → `{ data, error, status }`).
- **TypeScript:** zero `any`; shared types in `src/lib/types/index.ts`.
- **No AI attribution** in code/commits (beyond the standard `Co-Authored-By` trailer).
- **Testing baseline:** all five suites green; `bun run test` + `bun run check` must pass before the final task. Any new failure is a regression.
- **Commit discipline:** commit per task on `main`; `git add` only the task's files.

## File Structure

```
src/lib/types/index.ts .................... (MODIFY) add hub types
src/lib/api/hub.ts ........................ (CREATE) client mutation wrappers + re-export classifyError
src/lib/components/hub/
  useDirtyForm.svelte.ts .................. (CREATE) dirty-tracked form rune
  useDirtyForm.svelte.test.ts ............. (CREATE)
  optimisticAction.ts ..................... (CREATE) apply→call→rollback helper
  optimisticAction.test.ts ................ (CREATE)
  SettingsSection.svelte ................. (CREATE) form chrome + Save/Discard footer
  SettingsSection.test.ts ................ (CREATE)
  DashboardPanel.svelte ................. (CREATE) read chrome (used by Plan 3; primitive lands here)
  MetricCard.svelte ..................... (CREATE)
  settings/
    settingsSections.ts ................. (CREATE) client meta registry
    settingsSections.test.ts ............ (CREATE)
    settingsLoaders.server.ts ........... (CREATE) server loader registry
    sections/
      ProfileSection.svelte ............. (CREATE)
      AccountSection.svelte ............. (CREATE)
      PrivacySection.svelte ............ (CREATE)
src/routes/(app)/settings/
  +page.server.ts ......................... (CREATE) redirect → /settings/profile
  +layout.svelte .......................... (CREATE) section rail + {@render children()}
  [section]/+page.server.ts ............... (CREATE) dispatch loader; 404 unknown/!backendReady
  [section]/+page.svelte .................. (CREATE) render registry component
src/lib/i18n/en.json, fr.json ............. (MODIFY) settings.* keys
```

---

## Phase 0 — Types & shared logic

### Task 1: Hub types

**Files:** Modify `src/lib/types/index.ts` (append).

- [ ] **Step 1: Add the types**

```ts
/* ------------------------------------------------------------------ *
 * User hub (/settings, /dashboard)
 * ------------------------------------------------------------------ */

/** Rail grouping for the settings/dashboard registries. */
export type HubGroup = 'account' | 'security' | 'notifications' | 'billing' | 'developer' | 'activity' | 'danger';

/** Own profile+account (GET /me). Extends SessionUser with editable fields. */
export interface MeProfile {
	id: string;
	email: string;
	username: string;
	displayName: string;
	bio?: string | null;
	avatar_url?: string | null;
	cover_url?: string | null;
	primary_space?: SpaceRef | null;
	ui?: { nav_collapsed?: boolean; theme?: string } & Record<string, unknown>;
}

/** GET/PATCH /me/privacy — a map of privacy toggles (nested). */
export interface PrivacySettings {
	profile_visibility?: 'public' | 'members' | 'private';
	show_activity?: boolean;
	searchable?: boolean;
	[key: string]: unknown;
}
```

- [ ] **Step 2:** Run `cd apps/main && bun run check` → PASS.
- [ ] **Step 3:** Commit `feat(main): hub domain types`.

---

### Task 2: `optimisticAction` helper (TDD)

**Files:** Create `src/lib/components/hub/optimisticAction.ts`, `optimisticAction.test.ts`.

**Interfaces:**
- Produces: `optimisticAction<T>({ apply, rollback, call }): Promise<{ ok: boolean; data: T | null; error: ... }>`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, vi } from 'vitest';
import { optimisticAction } from './optimisticAction';

describe('optimisticAction', () => {
	it('applies and keeps the change on success', async () => {
		const apply = vi.fn(), rollback = vi.fn();
		const call = vi.fn().mockResolvedValue({ data: { id: 'x' }, error: null, status: 200 });
		const res = await optimisticAction({ apply, rollback, call });
		expect(apply).toHaveBeenCalledOnce();
		expect(rollback).not.toHaveBeenCalled();
		expect(res.ok).toBe(true);
		expect(res.data).toEqual({ id: 'x' });
	});
	it('rolls back on error', async () => {
		const apply = vi.fn(), rollback = vi.fn();
		const call = vi.fn().mockResolvedValue({ data: null, error: { code: 'e', message: 'no', status: 403 }, status: 403 });
		const res = await optimisticAction({ apply, rollback, call });
		expect(rollback).toHaveBeenCalledOnce();
		expect(res.ok).toBe(false);
		expect(res.error?.status).toBe(403);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test optimisticAction` → FAIL.
- [ ] **Step 3: Implement**

```ts
interface Res<T> { data: T | null; error: { code: string; message: string; status: number } | null; status: number; }

export async function optimisticAction<T>(opts: {
	apply: () => void;
	rollback: () => void;
	call: () => Promise<Res<T>>;
}): Promise<{ ok: boolean; data: T | null; error: Res<T>['error'] }> {
	opts.apply();
	const res = await opts.call();
	if (res.error) {
		opts.rollback();
		return { ok: false, data: null, error: res.error };
	}
	return { ok: true, data: res.data, error: null };
}
```

- [ ] **Step 4:** Run `cd apps/main && bun run test optimisticAction` → PASS.
- [ ] **Step 5:** Commit `feat(main): optimisticAction helper`.

---

### Task 3: `useDirtyForm` rune (TDD) — crown jewel

**Files:** Create `src/lib/components/hub/useDirtyForm.svelte.ts`, `useDirtyForm.svelte.test.ts`.

**Interfaces:**
- Produces: `class DirtyForm<T>` with reactive `values: T`, `dirty: boolean`, `saving: boolean`, `error: { cls, message } | null`; methods `save()`, `reset()`, `set(patch: Partial<T>)`. Constructor `(initial: T, save: (v: T) => Promise<Res>, classify?)`.

- [ ] **Step 1: Write the failing test**

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { DirtyForm } from './useDirtyForm.svelte';

const ok = (data: unknown = {}) => ({ data, error: null, status: 200 });
const fail = (status: number) => ({ data: null, error: { code: 'e', message: 'x', status }, status });

describe('DirtyForm', () => {
	let save: ReturnType<typeof vi.fn>;
	beforeEach(() => (save = vi.fn().mockResolvedValue(ok())));

	it('is not dirty until a value changes', () => {
		const f = new DirtyForm({ name: 'Jane', bio: '' }, save);
		expect(f.dirty).toBe(false);
		f.set({ name: 'Janet' });
		expect(f.dirty).toBe(true);
	});

	it('detects changes in NESTED objects (not just top-level)', () => {
		const f = new DirtyForm({ prefs: { email: true, push: false } }, save);
		expect(f.dirty).toBe(false);
		f.set({ prefs: { email: true, push: true } });
		expect(f.dirty).toBe(true);
	});

	it('save() persists then rebases the snapshot (dirty → false)', async () => {
		const f = new DirtyForm({ name: 'Jane' }, save);
		f.set({ name: 'Janet' });
		await f.save();
		expect(save).toHaveBeenCalledWith({ name: 'Janet' });
		expect(f.dirty).toBe(false);
	});

	it('reset() restores the snapshot', () => {
		const f = new DirtyForm({ name: 'Jane' }, save);
		f.set({ name: 'Janet' });
		f.reset();
		expect(f.values.name).toBe('Jane');
		expect(f.dirty).toBe(false);
	});

	it('classifies a 4xx save failure and stays dirty', async () => {
		const f = new DirtyForm({ name: 'Jane' }, vi.fn().mockResolvedValue(fail(422)));
		f.set({ name: '' });
		await f.save();
		expect(f.error?.cls).toBe('validation');
		expect(f.dirty).toBe(true);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test useDirtyForm` → FAIL.
- [ ] **Step 3: Implement**

```ts
import type { SaveErrorClass } from '$lib/types';

type Res = { data: unknown; error: { message: string; status: number } | null; status: number };

function defaultClassify(status: number): SaveErrorClass {
	if (status === 0) return 'network';
	if (status === 409) return 'conflict';
	if (status >= 500) return 'server';
	return 'validation';
}

/** Structural deep-equal for plain JSON snapshots (settings values are JSON-serializable). */
function deepEqual(a: unknown, b: unknown): boolean {
	return JSON.stringify(a) === JSON.stringify(b);
}

export class DirtyForm<T extends Record<string, unknown>> {
	values = $state<T>({} as T);
	saving = $state(false);
	error = $state<{ cls: SaveErrorClass; message: string } | null>(null);

	#snapshot: T;
	#save: (v: T) => Promise<Res>;
	#classify: (s: number) => SaveErrorClass;

	constructor(initial: T, save: (v: T) => Promise<Res>, classify = defaultClassify) {
		this.values = structuredClone(initial);
		this.#snapshot = structuredClone(initial);
		this.#save = save;
		this.#classify = classify;
	}

	get dirty(): boolean {
		return !deepEqual(this.values, this.#snapshot);
	}

	set(patch: Partial<T>): void {
		this.values = { ...this.values, ...patch };
	}

	async save(): Promise<boolean> {
		if (!this.dirty) return true;
		this.saving = true;
		this.error = null;
		const res = await this.#save(this.values);
		this.saving = false;
		if (res.error) {
			this.error = { cls: this.#classify(res.status), message: res.error.message };
			return false;
		}
		this.#snapshot = structuredClone(this.values);
		return true;
	}

	reset(): void {
		this.values = structuredClone(this.#snapshot);
		this.error = null;
	}
}
```

> Note: `SaveErrorClass` is defined in `src/lib/types/index.ts` (added by the `/create` spec's Task 1; if that plan has not run, add it here: `export type SaveErrorClass = 'validation' | 'server' | 'network' | 'conflict';`).

- [ ] **Step 4:** Run `cd apps/main && bun run test useDirtyForm` → PASS.
- [ ] **Step 5:** Commit `feat(main): useDirtyForm rune (dirty tracking + save/reset)`.

---

### Task 4: Settings registry (TDD) + server loaders

**Files:** Create `src/lib/components/hub/settings/settingsSections.ts`, `settingsSections.test.ts`, `settingsLoaders.server.ts`.

**Interfaces:**
- Produces (client meta): `interface SectionMeta { slug: string; labelKey: string; icon: string; group: HubGroup; backendReady: boolean; component: Component }`; `SETTINGS_SECTIONS: SectionMeta[]`; `sectionsByGroup(): [HubGroup, SectionMeta[]][]`; `resolveSection(slug): SectionMeta | null`; `DEFAULT_SECTION = 'profile'`.
- Produces (server): `SETTINGS_LOADERS: Record<string, (ctx: ApiContext) => Promise<unknown>>`.

- [ ] **Step 1: Write the failing test** (registry meta — no component import needed for gating logic; use a lightweight meta list)

```ts
import { describe, it, expect } from 'vitest';
import { resolveSection, sectionsByGroup, SETTINGS_SECTIONS } from './settingsSections';

describe('settings registry', () => {
	it('resolves a known slug and returns null for unknown', () => {
		expect(resolveSection('profile')?.slug).toBe('profile');
		expect(resolveSection('nope')).toBeNull();
	});
	it('flags devices as not backend-ready', () => {
		expect(resolveSection('devices')?.backendReady).toBe(false);
	});
	it('marks profile/account/privacy backend-ready', () => {
		for (const s of ['profile', 'account', 'privacy']) {
			expect(resolveSection(s)?.backendReady).toBe(true);
		}
	});
	it('groups sections and puts danger last', () => {
		const groups = sectionsByGroup().map(([g]) => g);
		expect(groups[groups.length - 1]).toBe('danger');
	});
	it('every section has an i18n label key', () => {
		for (const s of SETTINGS_SECTIONS) expect(s.labelKey.startsWith('settings.nav.')).toBe(true);
	});
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test settingsSections` → FAIL.
- [ ] **Step 3: Implement `settingsSections.ts`** — import the three built components; the rest (Plan 2) reference a shared `Placeholder` until built, but are listed with `backendReady` per spec. To keep this task shippable, only wire components for profile/account/privacy and a `NotBuiltYet` placeholder for the rest (they 404 via a `built` flag OR are simply omitted from the rail until Plan 2). Decision for this plan: **list only the sections this plan/Plan 2 will build; gate unbuilt ones with `backendReady` false OR a `built:false` flag** so the rail shows them disabled. Implement:

```ts
import type { Component } from 'svelte';
import type { HubGroup } from '$lib/types';
import ProfileSection from './sections/ProfileSection.svelte';
import AccountSection from './sections/AccountSection.svelte';
import PrivacySection from './sections/PrivacySection.svelte';

export interface SectionMeta {
	slug: string;
	labelKey: string;
	icon: string; // lucide name
	group: HubGroup;
	backendReady: boolean;
	component: Component | null; // null = not built in this plan (rail shows disabled)
}

export const DEFAULT_SECTION = 'profile';

export const SETTINGS_SECTIONS: SectionMeta[] = [
	{ slug: 'profile', labelKey: 'settings.nav.profile', icon: 'user', group: 'account', backendReady: true, component: ProfileSection },
	{ slug: 'account', labelKey: 'settings.nav.account', icon: 'settings', group: 'account', backendReady: true, component: AccountSection },
	{ slug: 'privacy', labelKey: 'settings.nav.privacy', icon: 'lock', group: 'account', backendReady: true, component: PrivacySection },
	{ slug: 'security', labelKey: 'settings.nav.security', icon: 'shield', group: 'security', backendReady: true, component: null },
	{ slug: 'connected', labelKey: 'settings.nav.connected', icon: 'link', group: 'security', backendReady: true, component: null },
	{ slug: 'devices', labelKey: 'settings.nav.devices', icon: 'smartphone', group: 'security', backendReady: false, component: null },
	{ slug: 'notifications', labelKey: 'settings.nav.notifications', icon: 'bell', group: 'notifications', backendReady: true, component: null },
	{ slug: 'api-keys', labelKey: 'settings.nav.api_keys', icon: 'key', group: 'developer', backendReady: true, component: null },
	{ slug: 'affiliate', labelKey: 'settings.nav.affiliate', icon: 'share-2', group: 'developer', backendReady: true, component: null },
	{ slug: 'wallet', labelKey: 'settings.nav.wallet', icon: 'wallet', group: 'billing', backendReady: true, component: null },
	{ slug: 'subscription', labelKey: 'settings.nav.subscription', icon: 'credit-card', group: 'billing', backendReady: true, component: null },
	{ slug: 'danger', labelKey: 'settings.nav.danger', icon: 'alert-triangle', group: 'danger', backendReady: true, component: null }
];

export function resolveSection(slug: string): SectionMeta | null {
	return SETTINGS_SECTIONS.find((s) => s.slug === slug) ?? null;
}

const GROUP_ORDER: HubGroup[] = ['account', 'security', 'notifications', 'billing', 'developer', 'danger'];

export function sectionsByGroup(): [HubGroup, SectionMeta[]][] {
	return GROUP_ORDER.map((g) => [g, SETTINGS_SECTIONS.filter((s) => s.group === g)] as [HubGroup, SectionMeta[]])
		.filter(([, list]) => list.length > 0);
}
```

- [ ] **Step 4: Implement `settingsLoaders.server.ts`** (server-only; profile/account share `GET /me`, privacy uses `GET /me/privacy`):

```ts
import type { ApiContext } from '$lib/server/api';
import { apiGet } from '$lib/server/api';
import type { MeProfile, PrivacySettings } from '$lib/types';

export const SETTINGS_LOADERS: Record<string, (ctx: ApiContext) => Promise<unknown>> = {
	profile: (ctx) => apiGet<MeProfile>(ctx, '/me'),
	account: (ctx) => apiGet<MeProfile>(ctx, '/me'),
	privacy: (ctx) => apiGet<PrivacySettings>(ctx, '/me/privacy')
};
```

- [ ] **Step 5:** Run `cd apps/main && bun run test settingsSections` → PASS.
- [ ] **Step 6:** Commit `feat(main): settings registry (client meta + server loaders)`.

---

## Phase 1 — Route, rail, chrome

### Task 5: `/settings` route, `[section]` dispatch, redirect

**Files:** Create `src/routes/(app)/settings/+page.server.ts`, `[section]/+page.server.ts`, `[section]/+page.svelte`.

- [ ] **Step 1: Redirect** `settings/+page.server.ts`:

```ts
import { redirect } from '@sveltejs/kit';
import { DEFAULT_SECTION } from '$lib/components/hub/settings/settingsSections';
export const load = () => { throw redirect(307, `/settings/${DEFAULT_SECTION}`); };
```

> SvelteKit 2 note: `redirect()` throws — do NOT wrap it in try/catch (project memory: SvelteKit2 redirect traps). Here there is no try/catch, which is correct.

- [ ] **Step 2: Dispatch loader** `[section]/+page.server.ts`:

```ts
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext } from '$lib/server/api';
import { resolveSection } from '$lib/components/hub/settings/settingsSections';
import { SETTINGS_LOADERS } from '$lib/components/hub/settings/settingsLoaders.server';

export const load: PageServerLoad = async (event) => {
	const meta = resolveSection(event.params.section);
	if (!meta || !meta.backendReady || !meta.component) throw error(404, 'Unknown settings section');
	const loader = SETTINGS_LOADERS[meta.slug];
	const sectionData = loader ? await loader(apiContext(event)) : null;
	return { section: meta.slug, sectionData };
};
```

- [ ] **Step 3: Render component** `[section]/+page.svelte`:

```svelte
<script lang="ts">
	import { resolveSection } from '$lib/components/hub/settings/settingsSections';
	let { data } = $props();
	const meta = $derived(resolveSection(data.section));
	const SectionComponent = $derived(meta?.component ?? null);
</script>

{#if SectionComponent}
	<SectionComponent data={data.sectionData} />
{/if}
```

- [ ] **Step 4:** Run `cd apps/main && bun run check` → PASS (components exist after Phase 2; if running in order, this compiles once Task 8 lands — acceptable, verified in Task 9).
- [ ] **Step 5:** Commit `feat(main): /settings route + [section] dispatch + redirect`.

### Task 6: Settings rail layout

**Files:** Create `src/routes/(app)/settings/+layout.svelte`.

**Interfaces:** Consumes `sectionsByGroup`, `resolveSection`; `page` from `$app/state`; `t`.

- [ ] **Step 1: Implement** the two-pane `[rail | panel]`, rail grouped via `sectionsByGroup()`, active slug from `page.params.section`, disabled entries when `!backendReady || !component`:

```svelte
<script lang="ts">
	import { page } from '$app/state';
	import { t } from '$lib/i18n';
	import { sectionsByGroup } from '$lib/components/hub/settings/settingsSections';
	let { children } = $props();
	const groups = sectionsByGroup();
	const active = $derived(page.params.section);
</script>

<div class="grid grid-cols-1 gap-6 md:grid-cols-[220px_1fr]">
	<nav class="sticky top-[var(--nav-height)] hidden self-start md:block" aria-label={t('settings.nav.label')}>
		{#each groups as [group, items] (group)}
			<div class="mb-4">
				<p class="px-3 pb-1 text-xs font-medium uppercase text-[var(--color-muted)]">{t(`settings.group.${group}`)}</p>
				{#each items as item (item.slug)}
					{@const disabled = !item.backendReady || !item.component}
					<a href={disabled ? undefined : `/settings/${item.slug}`}
						aria-current={active === item.slug ? 'page' : undefined}
						aria-disabled={disabled}
						class="block rounded-[var(--radius)] px-3 py-1.5 text-sm {active === item.slug ? 'bg-[var(--color-surface-subtle)] text-[var(--color-text)]' : 'text-[var(--color-muted)] hover:text-[var(--color-text)]'} {disabled ? 'pointer-events-none opacity-40' : ''}">
						{t(item.labelKey)}
					</a>
				{/each}
			</div>
		{/each}
	</nav>
	<div class="min-w-0 px-1 py-4">{@render children()}</div>
</div>
```

- [ ] **Step 2:** Run `cd apps/main && bun run check` → PASS.
- [ ] **Step 3:** Commit `feat(main): settings rail layout (grouped, active-aware, disabled-gated)`.

### Task 7: `SettingsSection` chrome (TDD)

**Files:** Create `src/lib/components/hub/SettingsSection.svelte`, `SettingsSection.test.ts`.

**Props:** `{ titleKey: string; descKey?: string; form: DirtyForm<any>; children: Snippet }`.

- [ ] **Step 1: Write the failing test** — footer hidden when not dirty, visible + Save enabled when dirty:

```ts
import { render } from '@testing-library/svelte';
import { it, expect } from 'vitest';
import SettingsSection from './SettingsSection.svelte';
import { DirtyForm } from './useDirtyForm.svelte';

it('hides the save footer until the form is dirty', async () => {
	const form = new DirtyForm({ name: 'Jane' }, async () => ({ data: {}, error: null, status: 200 }));
	const { queryByRole, rerender } = render(SettingsSection, { props: { titleKey: 'settings.nav.profile', form, children: undefined as any } });
	expect(queryByRole('button', { name: /save/i })).toBeNull();
	form.set({ name: 'Janet' });
	await rerender({ titleKey: 'settings.nav.profile', form, children: undefined as any });
	expect(queryByRole('button', { name: /save/i })).not.toBeNull();
});
```

- [ ] **Step 2:** Run `cd apps/main && bun run test SettingsSection` → FAIL.
- [ ] **Step 3: Implement** (header + children slot + dirty-gated sticky footer with Discard/Save; Save shows saving state; error surfaced):

```svelte
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { beforeNavigate } from '$app/navigation';
	import { t } from '$lib/i18n';
	import type { DirtyForm } from './useDirtyForm.svelte';
	interface Props { titleKey: string; descKey?: string; form: DirtyForm<any>; children: Snippet }
	let { titleKey, descKey, form, children }: Props = $props();

	beforeNavigate((nav) => {
		if (form.dirty && !confirm(t('settings.unsaved_warning'))) nav.cancel();
	});
</script>

<section class="max-w-2xl">
	<header class="mb-4">
		<h1 class="text-lg font-medium text-[var(--color-text)]">{t(titleKey)}</h1>
		{#if descKey}<p class="text-sm text-[var(--color-muted)]">{t(descKey)}</p>{/if}
	</header>
	{@render children()}
	{#if form.dirty}
		<div class="sticky bottom-0 mt-6 flex items-center justify-end gap-2 border-t border-[var(--color-border)] bg-[var(--color-surface)] py-3">
			{#if form.error}<span class="mr-auto text-xs text-[var(--color-danger,#dc2626)]">{form.error.message}</span>{/if}
			<button type="button" class="rounded-[var(--radius)] px-3 py-1.5 text-sm text-[var(--color-text)] hover:bg-[var(--color-surface-subtle)]" onclick={() => form.reset()}>{t('settings.action.discard')}</button>
			<button type="button" disabled={form.saving} class="rounded-[var(--radius)] bg-[var(--color-primary)] px-4 py-1.5 text-sm font-medium text-white disabled:opacity-50" onclick={() => form.save()}>{t('settings.action.save')}</button>
		</div>
	{/if}
</section>
```

> `beforeNavigate` uses `confirm()` — a browser dialog. Acceptable for a nav guard. Do not use `alert()` elsewhere.

- [ ] **Step 4:** Run `cd apps/main && bun run test SettingsSection` → PASS.
- [ ] **Step 5:** Commit `feat(main): SettingsSection chrome (dirty-gated Save/Discard)`.

---

## Phase 2 — The three core sections

> Each section: Svelte 5 Runes, Bits UI where a primitive applies, Tailwind tokens, i18n. Build a `DirtyForm` from the loaded `data`, wire fields to `form.values` via `form.set(...)`, wrap in `SettingsSection`. Client mutations via `src/lib/api/hub.ts`. Follow `jc-main-develop`/`svelte` skills for exact Bits UI markup.

### Task 8: hub client wrappers + Profile / Account / Privacy sections

**Files:** Create `src/lib/api/hub.ts`, `sections/ProfileSection.svelte`, `AccountSection.svelte`, `PrivacySection.svelte`.

- [ ] **Step 1: Client wrappers** `src/lib/api/hub.ts`:

```ts
import { api } from '$lib/api/client';
import type { MeProfile, PrivacySettings } from '$lib/types';
export { classifyError } from '$lib/api/create'; // reuse the /create classifier if present; else inline the 4-line version

export const patchProfile = (v: Partial<MeProfile>) => api.patch<MeProfile>('/me/profile', v);
export const patchAccount = (v: Record<string, unknown>) => api.patch<MeProfile>('/me/account', v);
export const patchPrivacy = (v: PrivacySettings) => api.patch<PrivacySettings>('/me/privacy', v);
```

> If `$lib/api/create` does not exist (the /create plan hasn't run), inline: `export function classifyError(s:number){ if(s===0)return 'network'; if(s===409)return 'conflict'; if(s>=500)return 'server'; return 'validation'; }`.

- [ ] **Step 2: ProfileSection** — fields name/bio/avatar/cover; `DirtyForm` seeded from `data`; save → `patchProfile(form.values)`:

```svelte
<script lang="ts">
	import { DirtyForm } from '$lib/components/hub/useDirtyForm.svelte';
	import SettingsSection from '$lib/components/hub/SettingsSection.svelte';
	import { patchProfile } from '$lib/api/hub';
	import { t } from '$lib/i18n';
	import type { MeProfile } from '$lib/types';
	let { data }: { data: MeProfile } = $props();
	const form = new DirtyForm(
		{ displayName: data.displayName ?? '', bio: data.bio ?? '' },
		(v) => patchProfile(v)
	);
</script>

<SettingsSection titleKey="settings.nav.profile" descKey="settings.profile.desc" {form}>
	<label class="mb-3 block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.name')}</span>
		<input class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.displayName} oninput={(e) => form.set({ displayName: e.currentTarget.value })} />
	</label>
	<label class="block">
		<span class="mb-1 block text-sm text-[var(--color-muted)]">{t('settings.profile.bio')}</span>
		<textarea class="w-full rounded-[var(--radius)] border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-[var(--color-text)]"
			value={form.values.bio} oninput={(e) => form.set({ bio: e.currentTarget.value })}></textarea>
	</label>
</SettingsSection>
```

- [ ] **Step 3: AccountSection** — email (read-only display), username, theme (`ui.theme`) select; save → `patchAccount({ username, ui: { theme } })`. Follow the same `DirtyForm` + `SettingsSection` structure; use Bits UI `Select` for theme (light/dark).
- [ ] **Step 4: PrivacySection** — toggles from `PrivacySettings` (a nested map — proves `useDirtyForm`'s nested dirty check); save → `patchPrivacy(form.values)`. Use switch/checkbox controls.
- [ ] **Step 5:** Run `cd apps/main && bun run check` → PASS.
- [ ] **Step 6:** Commit `feat(main): hub client wrappers + profile/account/privacy sections`.

---

## Phase 3 — i18n + verification

### Task 9: i18n keys + full gate + live round-trip

**Files:** Modify `src/lib/i18n/en.json`, `fr.json`.

- [ ] **Step 1: Collect and add keys**

Run: `cd apps/main && grep -rohE "settings\.[a-z_.]+" src/lib/components/hub src/routes/\(app\)/settings | sort -u`

Add every key to `en.json` AND `fr.json` (groups: `settings.nav.*`, `settings.group.*`, `settings.action.*`, `settings.profile.*`, `settings.account.*`, `settings.privacy.*`, `settings.unsaved_warning`, `settings.nav.label`).

- [ ] **Step 2: Full gate**

Run: `cd apps/main && bun run test && bun run check`
Expected: all suites PASS; zero check errors.

- [ ] **Step 3: Live round-trip** (backend :8080, app :3000; load the `verify`/`run` skill):
  1. Visit `/settings` → redirects to `/settings/profile`; rail shows grouped sections; `devices`/unbuilt entries are disabled.
  2. Edit the display name → the Save/Discard footer appears; Save → PATCH `/me/profile`; reload → persisted.
  3. Toggle a privacy switch (nested map) → footer appears (proves nested dirty detection); Save → persisted.
  4. Direct-visit `/settings/devices` → 404 (backendReady:false); direct-visit `/settings/nonsense` → 404.
- [ ] **Step 4: Final commit** (if fixes needed) `fix(main): settings core round-trip corrections`.

---

## Self-Review (plan author)

- **Spec coverage (this plan's slice):** two-registry split (T4) · dynamic route + 404 gating (T5) · rail (T6) · `useDirtyForm` (T3) · `optimisticAction` (T2) · `SettingsSection` (T7) · profile/account/privacy (T8) · nested dirty proven (T3 test + T8 privacy) · backendReady 404 (T5 + T9 round-trip). `DashboardPanel`/`MetricCard`/charts and remaining sections are Plans 2–3 (explicitly deferred).
- **Type consistency:** `DirtyForm<T>` (T3) consumed by `SettingsSection` (T7) and the sections (T8); `SectionMeta`/`resolveSection` (T4) used by T5/T6; `MeProfile`/`PrivacySettings` (T1) used by loaders (T4) + wrappers (T8).
- **Cross-plan dependency:** `classifyError`/`SaveErrorClass` may come from the `/create` plan; T3 and T8 notes give the inline fallback so this plan stands alone.
