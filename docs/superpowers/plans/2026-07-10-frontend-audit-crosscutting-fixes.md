# Frontend Audit — Cross-Cutting Critical Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the bugs from `.jeos/audit/AUDIT-FE-2026-07-10.md` that appear identically in 2-3 of the 5 frontend apps (admin/control/learn/main), so each root cause is fixed once instead of patched per app.

**Architecture:** No new architecture — these are targeted bug fixes inside each app's existing SvelteKit conventions (`$env/static/public` for client-exposed config, `$lib/server/api.ts` for server-only backend calls, Bits-UI-free plain form actions for SvelteKit `+page.server.ts` actions). One new dependency (`@sentry/sveltekit`, pointed at each app's already-provisioned `PUBLIC_JC_GLITCHTIP_DSN`) is added to 3 apps for error tracking.

**Tech Stack:** SvelteKit 2.63 / Svelte 5.56 (runes) / Bun / Vitest + @testing-library/svelte / TypeScript.

## Global Constraints

- Session cookie is always named `jc_token`, set `httpOnly: true`, `secure: true`, `sameSite: 'lax'`, `domain: '.japaconnect.com'` — never narrow the domain, never make it client-readable.
- Client-exposed env vars use the `PUBLIC_JC_*` naming convention via `$env/static/public`. Server-only env vars use `$env/dynamic/private` (`PRIVATE_API_BASE_URL`) via each app's `src/lib/server/api.ts`. Never read `import.meta.env.VITE_*` — that convention is not wired to any real value in this monorepo.
- Never attribute code to AI authorship in comments or commit messages (house Rule 17). Comments referencing an audit finding ID (e.g. `AUDIT-FE-ADM-001`) are fine — they reference a spec/tracking document, not authorship.
- Every touched app has its own `package.json`/`bun install`; there is no shared workspace root — run `bun` commands from inside each `apps/<name>` directory.
- Existing test convention: Vitest + `@testing-library/svelte`, one `*.test.ts` per route/module, run via `bun run test` (aliases to `vitest run`) inside the app directory.
- Do not expand scope into findings not listed in a task below (e.g. do not attempt the full `apps/main` env-var rename, or admin's `$lib/api.ts`/`ai/generate` gaps) — those are separate, already-documented audit findings for a later pass.

---

### Task 1: Fix admin's broken permission/user hydration (AUDIT-FE-ADM-001)

**Files:**
- Modify: `apps/admin/src/lib/stores/permissions.ts`
- Modify: `apps/admin/src/routes/+layout.ts`
- Test: `apps/admin/src/lib/stores/permissions.test.ts` (create)
- Test: `apps/admin/src/routes/layout.test.ts` (create)

**Interfaces:**
- Consumes: none (self-contained; `$lib/api/client`'s `apiFetch<T>(path): Promise<{data: T|null, error, status}>` already exists and is unchanged).
- Produces: `fetchPermissions(): Promise<void>` (signature changes — drops its unused `token` parameter). Any other future caller of `fetchPermissions` must call it with zero arguments.

The root bug: `+layout.ts` parses `document.cookie` for `jc_token` to decide whether to call `fetchPermissions()`/`apiFetch<User>('/me')`. `jc_token` is `httpOnly`, so `document.cookie` never contains it — `token` is always `null`, so neither call ever fires, and both `stores/permissions.ts`'s `permissions` store (read by `Sidebar.svelte`'s nav gating) and `stores/auth.ts`'s `permissions`/`user` stores (populated via `+layout.svelte`'s `$effect` from `data.user`, read by `TopBar.svelte` and page-level `hasPermission()` checks) stay empty for every legitimately authenticated admin. `$lib/api/client.ts`'s `apiFetch` already sends the cookie automatically via `credentials: 'include'` — the fix is to stop gating on a client-unreadable token and call these unconditionally, while still skipping on `/login` so a 401 there (handled by `apiFetch` redirecting to `/login`) can't create a refetch loop.

`data.token` must stay in the returned shape (as a static `null`) because two existing pages (`apps/admin/src/routes/users/+page.svelte:95` and `apps/admin/src/routes/users/[userId]/+page.svelte:173`) call `setToken(data.token)` from the separate legacy `$lib/api.ts` client (AUDIT-FE-ADM-007 territory — out of scope here); removing the field entirely would be a type error at those two call sites.

- [ ] **Step 1: Write the failing test for `fetchPermissions`**

```ts
// apps/admin/src/lib/stores/permissions.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn()
}));

import { apiFetch } from '$lib/api/client';
import { fetchPermissions, permissions, permissionsLoaded, hasPermission } from './permissions';

describe('fetchPermissions', () => {
	beforeEach(() => {
		permissions.set([]);
		permissionsLoaded.set(false);
		vi.mocked(apiFetch).mockReset();
	});

	it('takes no arguments and populates the store from /me/permissions', async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			data: { permissions: ['admin.users.view', 'admin.settings'] },
			error: null,
			status: 200
		});

		await fetchPermissions();

		expect(apiFetch).toHaveBeenCalledWith('/me/permissions');
		expect(get(permissions)).toEqual(['admin.users.view', 'admin.settings']);
		expect(get(permissionsLoaded)).toBe(true);
		expect(hasPermission('admin.users.view')).toBe(true);
		expect(hasPermission('admin.broadcast')).toBe(false);
	});

	it('sets an empty list (not an exception) when the API call fails', async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			data: null,
			error: { code: 'server_error', message: 'boom', status: 500 },
			status: 500
		});

		await fetchPermissions();

		expect(get(permissions)).toEqual([]);
		expect(get(permissionsLoaded)).toBe(true);
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/admin && bun run test -- permissions.test.ts`
Expected: FAIL — `fetchPermissions()` (called with 0 args) does not match the current `(token: string | null)` signature's early-return branch, and `apiFetch` is never called with `'/me/permissions'` because `token` is `undefined`/falsy under the old code, hitting the `if (!token)` branch instead.

- [ ] **Step 3: Fix `permissions.ts` — drop the unused token gate**

```ts
// apps/admin/src/lib/stores/permissions.ts
import { writable } from 'svelte/store';
import { apiFetch } from '$lib/api/client';

export const permissions = writable<string[]>([]);
export const permissionsLoaded = writable(false);

export function hasPermission(permKey: string): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = perms.includes(permKey);
	});
	unsub();
	return result;
}

export function hasAllPermissions(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.every((k) => perms.includes(k));
	});
	unsub();
	return result;
}

export function hasAnyPermission(permKeys: string[]): boolean {
	let result = false;
	const unsub = permissions.subscribe((perms) => {
		result = permKeys.some((k) => perms.includes(k));
	});
	unsub();
	return result;
}

export async function fetchPermissions(): Promise<void> {
	const result = await apiFetch<{ permissions: string[] }>('/me/permissions');

	if (result.data) {
		permissions.set(result.data.permissions ?? []);
	} else {
		permissions.set([]);
	}
	permissionsLoaded.set(true);
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/admin && bun run test -- permissions.test.ts`
Expected: PASS (2 tests)

- [ ] **Step 5: Write the failing test for `+layout.ts`**

```ts
// apps/admin/src/routes/layout.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$app/environment', () => ({ browser: false }));

vi.mock('$lib/stores/permissions', () => ({
	fetchPermissions: vi.fn().mockResolvedValue(undefined)
}));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn()
}));

import { load } from './+layout';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';

describe('admin root +layout load', () => {
	beforeEach(() => {
		vi.mocked(fetchPermissions).mockClear();
		vi.mocked(apiFetch).mockReset();
	});

	it('skips fetching permissions/user on /login to avoid a redirect loop', async () => {
		const result = await load({ url: new URL('http://admin.japaconnect.com/login') } as any);

		expect(fetchPermissions).not.toHaveBeenCalled();
		expect(apiFetch).not.toHaveBeenCalled();
		expect(result.user).toBeNull();
		expect(result.token).toBeNull();
	});

	it('fetches permissions and the current user unconditionally on non-login routes', async () => {
		const fakeUser = {
			id: '1',
			display_name: 'Ada',
			avatar_url: '',
			permissions: ['admin.users.view']
		};
		vi.mocked(apiFetch).mockResolvedValue({ data: fakeUser, error: null, status: 200 });

		const result = await load({ url: new URL('http://admin.japaconnect.com/users') } as any);

		expect(fetchPermissions).toHaveBeenCalledTimes(1);
		expect(apiFetch).toHaveBeenCalledWith('/me');
		expect(result.user).toEqual(fakeUser);
	});
});
```

- [ ] **Step 6: Run the test to verify it fails**

Run: `cd apps/admin && bun run test -- layout.test.ts`
Expected: FAIL — current `load` reads `document.cookie` (not exercised by this test's mock) and never calls `fetchPermissions`/`apiFetch` at all, so both assertions on the second test fail.

- [ ] **Step 7: Fix `+layout.ts`**

```ts
// apps/admin/src/routes/+layout.ts
import type { LayoutLoad } from './$types';
import { browser } from '$app/environment';
import { locale } from '$lib/stores/i18n.svelte';
import { fetchPermissions } from '$lib/stores/permissions';
import { apiFetch } from '$lib/api/client';
import type { User } from '$lib/api/types';

export const ssr = false;

export const load: LayoutLoad = async ({ url }) => {
	let localeCode = 'en';

	if (browser) {
		const localeMatch = document.cookie.match(/jc_locale=([^;]+)/);
		if (localeMatch) localeCode = localeMatch[1];
	}

	const translations = await import(`$lib/locales/${localeCode}.json`)
		.then((m) => m.default)
		.catch(() => ({}));
	locale.init(localeCode, translations);

	// jc_token is httpOnly (AUDIT-FE-ADM-001) — it can never be read via
	// document.cookie, only sent automatically by apiFetch's
	// `credentials: 'include'`. Fetch permissions/user unconditionally
	// instead of gating on a client-readable token. Skip on /login: apiFetch
	// redirects to /login on a 401, which would otherwise refetch and 401
	// again in a loop.
	let user: User | null = null;
	if (!url.pathname.startsWith('/login')) {
		await fetchPermissions();
		const result = await apiFetch<User>('/me');
		user = result.data;
	}

	// `token` is always null now — kept only so the legacy `$lib/api.ts`
	// bearer-token client (still used by users/+page.svelte and
	// users/[userId]/+page.svelte, see AUDIT-FE-ADM-007) keeps compiling.
	// That client's Authorization-header auth model is a separate, larger
	// issue and out of scope here.
	return { token: null as string | null, user };
};
```

- [ ] **Step 8: Run the test to verify it passes**

Run: `cd apps/admin && bun run test -- layout.test.ts`
Expected: PASS (2 tests)

- [ ] **Step 9: Type-check the app**

Run: `cd apps/admin && bun run check`
Expected: no new errors (in particular, confirm `users/+page.svelte` and `users/[userId]/+page.svelte` still compile against `data.token: string | null`)

- [ ] **Step 10: Commit**

```bash
git add apps/admin/src/lib/stores/permissions.ts apps/admin/src/lib/stores/permissions.test.ts apps/admin/src/routes/+layout.ts apps/admin/src/routes/layout.test.ts
git commit -m "fix(admin): populate permissions/user unconditionally instead of gating on an unreadable httpOnly cookie

AUDIT-FE-ADM-001"
```

---

### Task 2: Harden admin logout given the httpOnly-cookie limitation (AUDIT-FE-ADM-003, partial)

**Files:**
- Modify: `apps/admin/src/lib/components/layout/TopBar.svelte`
- Test: `apps/admin/src/lib/components/layout/TopBar.test.ts` (create)

**Interfaces:**
- Consumes: `apiFetch` from `$lib/api/client` (Task 1's dependency, unchanged), `clearUser` from `$lib/stores/auth` (existing), `permissions` writable from `$lib/stores/permissions` (Task 1).
- Produces: none consumed by later tasks.

**Important — this does NOT fully close AUDIT-FE-ADM-003.** `jc_token` is `httpOnly`; no client-side JavaScript can ever read or clear it (that's the point of `httpOnly`). `apps/admin/svelte.config.js` configures `adapter-static` with `fallback: 'index.html'` (a pure SPA build, confirmed by reading the file — "No server runtime is needed in production"), so there is no live SvelteKit server in production to run a `+page.server.ts` action that could call `cookies.delete(...)` server-side (that's exactly what Task 3 does for `apps/control`, which uses `adapter-node`). Until AUDIT-FE-ADM-002 is resolved (switch admin off `adapter-static`, or accept and formally document client-only enforcement), a true fix isn't possible. This task does the best available mitigation: revoke the session server-side (so a captured/replayed token stops working) and clear the app's in-memory state (so gated UI disappears immediately), and leaves a clear code comment + this plan as the paper trail that the cookie itself still lingers client-side until it naturally expires (7 days, per `login/+page.server.ts`'s `maxAge`).

- [ ] **Step 1: Write the failing test**

```ts
// apps/admin/src/lib/components/layout/TopBar.test.ts
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';

vi.mock('$env/static/public', () => ({ PUBLIC_JC_ENV: 'development' }));

vi.mock('$lib/api/client', () => ({
	apiFetch: vi.fn().mockResolvedValue({ data: null, error: null, status: 200 })
}));

const clearUserMock = vi.fn();
vi.mock('$lib/stores/auth', () => ({
	user: { subscribe: (fn: (v: unknown) => void) => (fn({ display_name: 'Ada', avatar_url: '' }), () => {}) },
	clearUser: clearUserMock
}));

const permissionsSetMock = vi.fn();
vi.mock('$lib/stores/permissions', () => ({
	permissions: { set: permissionsSetMock }
}));

vi.mock('$lib/stores/theme', () => ({
	theme: { subscribe: (fn: (v: unknown) => void) => (fn('light'), () => {}), update: vi.fn() }
}));

import { apiFetch } from '$lib/api/client';
import TopBar from './TopBar.svelte';

describe('admin TopBar logout', () => {
	beforeEach(() => {
		vi.mocked(apiFetch).mockClear();
		clearUserMock.mockClear();
		permissionsSetMock.mockClear();
		// @ts-expect-error jsdom navigation stub
		delete window.location;
		// @ts-expect-error jsdom navigation stub
		window.location = { href: '' };
	});

	it('best-effort revokes the session server-side and clears in-memory state before redirecting', async () => {
		render(TopBar);

		await fireEvent.click(screen.getByRole('button', { name: /ada/i }));
		await fireEvent.click(screen.getByText('Logout'));

		expect(apiFetch).toHaveBeenCalledWith('/auth/logout', { method: 'POST' });
		expect(clearUserMock).toHaveBeenCalled();
		expect(permissionsSetMock).toHaveBeenCalledWith([]);
		expect(window.location.href).toBe('/login');
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/admin && bun run test -- TopBar.test.ts`
Expected: FAIL — current `logout()` never calls `apiFetch`, `clearUser`, or `permissions.set`.

- [ ] **Step 3: Fix `TopBar.svelte`**

```svelte
<script lang="ts">
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user, clearUser } from '$lib/stores/auth';
	import { permissions } from '$lib/stores/permissions';
	import { apiFetch } from '$lib/api/client';
	import { theme } from '$lib/stores/theme';
	import Avatar from '$lib/components/ui/Avatar.svelte';
	import { Bell, Sun, Moon } from 'lucide-svelte';

	let showDropdown = $state(false);
	let unreadCount = $state(0);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
	}

	async function logout() {
		// NOTE (AUDIT-FE-ADM-003, partial fix only): jc_token is httpOnly, so
		// it can never be cleared from client JS, and this app ships as a
		// static SPA (adapter-static + fallback: 'index.html', see
		// svelte.config.js) with no live server to run a cookie-clearing
		// action in production. This best-effort revokes the session
		// server-side (Go backend drops its jc_refresh cookie + JTI
		// allowlist entry) and clears in-memory client state, but the
		// jc_token cookie itself keeps being sent until it naturally expires.
		// Do not consider AUDIT-FE-ADM-003 closed — a full fix needs
		// AUDIT-FE-ADM-002 resolved first (move off adapter-static, or add a
		// real server endpoint capable of clearing the cookie).
		await apiFetch('/auth/logout', { method: 'POST' }).catch(() => {});
		clearUser();
		permissions.set([]);
		window.location.href = '/login';
	}
</script>

<header class="flex h-14 items-center justify-between border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-900">
	<div class="flex items-center gap-3">
		<a href="/" class="text-sm font-semibold text-gray-900 dark:text-gray-100">JapaConnect Admin</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<button
			type="button"
			class="relative rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
		>
			<Bell class="h-4 w-4" />
			{#if unreadCount > 0}
				<span class="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] text-white">
					{unreadCount}
				</span>
			{/if}
		</button>

		<button
			type="button"
			class="rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
			onclick={toggleTheme}
		>
			{#if $theme === 'dark'}
				<Sun class="h-4 w-4" />
			{:else}
				<Moon class="h-4 w-4" />
			{/if}
		</button>

		{#if $user}
			<div class="relative">
				<button
					type="button"
					class="flex items-center gap-2 rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-800"
					onclick={() => (showDropdown = !showDropdown)}
				>
					<Avatar src={$user.avatar_url} alt={$user.display_name} size="sm" />
					<span class="text-sm text-gray-700 dark:text-gray-300">{$user.display_name}</span>
				</button>

				{#if showDropdown}
					<div class="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800">
						<button
							type="button"
							class="w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
							onclick={logout}
						>
							Logout
						</button>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/admin && bun run test -- TopBar.test.ts`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add apps/admin/src/lib/components/layout/TopBar.svelte apps/admin/src/lib/components/layout/TopBar.test.ts
git commit -m "fix(admin): best-effort session revocation + state clear on logout

Partial fix for AUDIT-FE-ADM-003 — full fix is blocked on AUDIT-FE-ADM-002
(the app ships as a static SPA with no server to clear the httpOnly cookie)."
```

---

### Task 3: Build control's real server-side logout (AUDIT-FE-CTL-010)

**Files:**
- Create: `apps/control/src/routes/logout/+page.server.ts`
- Create: `apps/control/src/routes/logout/page.server.test.ts`
- Modify: `apps/control/src/lib/components/layout/TopBar.svelte`

**Interfaces:**
- Consumes: `API_BASE` (string) exported from `apps/control/src/lib/server/api.ts` (already exists, unchanged).
- Produces: a `POST /logout` route other control pages may link/form-submit to.

Unlike admin, `apps/control/svelte.config.js` uses `@sveltejs/adapter-node` — a real server runs in production, so a `+page.server.ts` action can genuinely clear the cookie. Note the Go backend's `POST /auth/logout` (`internal/auth/handlers.go:385`) only clears its own `jc_refresh` cookie and revokes the access-token JTI — it does not and cannot know about `jc_token` (that cookie is set entirely by each frontend app's own login action). So control must clear `jc_token` itself via `cookies.delete(...)`.

- [ ] **Step 1: Write the failing test**

```ts
// apps/control/src/routes/logout/page.server.test.ts
import { describe, it, expect, vi } from 'vitest';
import { actions } from './+page.server';

function makeCookies(initial: Record<string, string> = {}) {
	const store = { ...initial };
	return {
		get: (name: string) => store[name],
		delete: vi.fn((name: string) => {
			delete store[name];
		})
	};
}

describe('control /logout action', () => {
	it('best-effort calls the backend logout, deletes jc_token with the correct domain/path, and redirects to /login', async () => {
		const cookies = makeCookies({ jc_token: 'abc123' });
		const fetchMock = vi.fn().mockResolvedValue(new Response('{}', { status: 200 }));

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(fetchMock).toHaveBeenCalledWith(
			expect.stringContaining('/auth/logout'),
			expect.objectContaining({ method: 'POST', headers: { Authorization: 'Bearer abc123' } })
		);
		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});

	it('still clears the cookie and redirects even if the backend call throws', async () => {
		const cookies = makeCookies({ jc_token: 'abc123' });
		const fetchMock = vi.fn().mockRejectedValue(new Error('network down'));

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});

	it('redirects without a backend call when there is no session cookie', async () => {
		const cookies = makeCookies({});
		const fetchMock = vi.fn();

		await expect(actions.default({ cookies, fetch: fetchMock } as any)).rejects.toMatchObject({
			status: 303,
			location: '/login'
		});

		expect(fetchMock).not.toHaveBeenCalled();
		expect(cookies.delete).toHaveBeenCalledWith('jc_token', {
			path: '/',
			domain: '.japaconnect.com'
		});
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/control && bun run test -- logout/page.server.test.ts`
Expected: FAIL — `./+page.server` does not exist yet.

- [ ] **Step 3: Create `+page.server.ts`**

```ts
// apps/control/src/routes/logout/+page.server.ts
import { redirect } from '@sveltejs/kit';
import type { Actions, PageServerLoad } from './$types';
import { API_BASE } from '$lib/server/api';

// A direct GET to /logout (e.g. a bookmarked link) just sends the visitor
// back to login rather than rendering anything.
export const load: PageServerLoad = async () => {
	throw redirect(303, '/login');
};

export const actions: Actions = {
	default: async ({ cookies, fetch }) => {
		const token = cookies.get('jc_token');

		if (token) {
			// Best-effort: revokes the refresh token + access-token JTI
			// server-side so a captured token can't be replayed. Never block
			// the redirect on this — clearing jc_token below is what actually
			// ends the session as far as this app is concerned.
			try {
				await fetch(`${API_BASE}/auth/logout`, {
					method: 'POST',
					headers: { Authorization: `Bearer ${token}` }
				});
			} catch {
				// Ignore — proceed to clear the cookie regardless.
			}
		}

		cookies.delete('jc_token', { path: '/', domain: '.japaconnect.com' });
		throw redirect(303, '/login');
	}
};
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/control && bun run test -- logout/page.server.test.ts`
Expected: PASS (3 tests)

- [ ] **Step 5: Wire `TopBar.svelte` to submit to the new route**

```svelte
<script lang="ts">
	import { enhance } from '$app/forms';
	import { PUBLIC_JC_ENV } from '$env/static/public';
	import { user } from '$lib/stores/auth';
	import { theme } from '$lib/stores/theme';
	import Avatar from '$lib/components/ui/Avatar.svelte';

	let showDropdown = $state(false);

	function toggleTheme() {
		theme.update((v) => (v === 'dark' ? 'light' : 'dark'));
	}
</script>

<header class="flex h-14 items-center justify-between border-b border-gray-200 bg-white px-4 dark:border-gray-700 dark:bg-gray-900">
	<div class="flex items-center gap-3">
		<a href="/" class="text-sm font-semibold text-gray-900 dark:text-gray-100">JapaConnect Control</a>
		{#if PUBLIC_JC_ENV === 'production'}
			<span class="rounded bg-red-600 px-2 py-0.5 text-xs font-bold text-white">
				PRODUCTION
			</span>
		{/if}
	</div>

	<div class="flex items-center gap-4">
		<button
			type="button"
			class="rounded p-1 text-gray-500 hover:bg-gray-100 dark:text-gray-400 dark:hover:bg-gray-800"
			onclick={toggleTheme}
		>
			{$theme === 'dark' ? '☀️' : '🌙'}
		</button>

		{#if $user}
			<div class="relative">
				<button
					type="button"
					class="flex items-center gap-2 rounded p-1 hover:bg-gray-100 dark:hover:bg-gray-800"
					onclick={() => (showDropdown = !showDropdown)}
				>
					<Avatar src={$user.avatar_url} alt={$user.display_name} size="sm" />
					<span class="text-sm text-gray-700 dark:text-gray-300">{$user.display_name}</span>
				</button>

				{#if showDropdown}
					<div class="absolute right-0 top-full z-50 mt-1 w-48 rounded-md border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800">
						<form method="POST" action="/logout" use:enhance>
							<button
								type="submit"
								class="w-full px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-gray-700"
							>
								Logout
							</button>
						</form>
					</div>
				{/if}
			</div>
		{/if}
	</div>
</header>
```

- [ ] **Step 6: Type-check the app**

Run: `cd apps/control && bun run check`
Expected: no new errors

- [ ] **Step 7: Commit**

```bash
git add apps/control/src/routes/logout apps/control/src/lib/components/layout/TopBar.svelte
git commit -m "fix(control): add a real server-side logout that clears jc_token

AUDIT-FE-CTL-010"
```

---

### Task 4: Fix control's broken API base URL env var (AUDIT-FE-CTL-011)

**Files:**
- Modify: `apps/control/src/routes/login/+page.server.ts`
- Modify: `apps/control/src/lib/stores/i18n.svelte.ts`
- Test: `apps/control/src/routes/login/page.server.test.ts` (create)

**Interfaces:**
- Consumes: `API_BASE` from `$lib/server/api` (server-side, already used correctly elsewhere in the app), `PUBLIC_JC_API_URL` from `$env/static/public` (client-side, already defined in `.env`/`.env.example`).
- Produces: none.

`import.meta.env.VITE_API_URL` is never defined anywhere in this monorepo (`.env`/`.env.example` only define `PUBLIC_JC_API_URL`), so both call sites always fall back to the hardcoded `http://localhost:8080/api/v1` default — including in staging/production, meaning login and i18n-pack loading currently target the wrong host outside local dev.

- [ ] **Step 1: Write the failing test**

```ts
// apps/control/src/routes/login/page.server.test.ts
import { describe, it, expect, vi } from 'vitest';
import { actions } from './+page.server';

describe('control login action', () => {
	it('posts to the configured API_BASE, not a hardcoded localhost fallback', async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ data: { token: 'abc123' } }), { status: 200 })
		);
		const cookies = { set: vi.fn() };
		const formData = new FormData();
		formData.set('email', 'admin@japaconnect.com');
		formData.set('password', 'correct-horse-battery-staple');
		const request = { formData: () => Promise.resolve(formData) };

		await expect(
			actions.default({ request, cookies, fetch: fetchMock } as any)
		).rejects.toMatchObject({ status: 302, location: '/' });

		const [calledUrl] = fetchMock.mock.calls[0];
		expect(calledUrl).not.toContain('localhost:8080');
		expect(calledUrl).toContain('/auth/login');
	});
});
```

Note: this test only proves the URL is no longer hardcoded to `localhost:8080` when `PRIVATE_API_BASE_URL` is set to something else in the test environment. Since `apps/control/.env` currently has no `PRIVATE_API_BASE_URL` override, `API_BASE`'s own fallback is also `http://localhost:8080/api/v1` locally — so this test's real value is guarding against a *hardcoded* `import.meta.env.VITE_API_URL` fallback reappearing, not asserting a specific non-localhost URL. That's the correct scope: the bug was "reads the wrong env var name," not "the fallback value is wrong."

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/control && bun run test -- login/page.server.test.ts`
Expected: FAIL is not guaranteed here (both old and new code may resolve to the same localhost fallback value in this environment) — this is expected and fine; step 4 confirms the *source* changed via a `grep`, not just this test. Proceed to Step 3 regardless.

- [ ] **Step 3: Fix `login/+page.server.ts`**

```ts
// apps/control/src/routes/login/+page.server.ts
import { fail, redirect } from '@sveltejs/kit';
import type { Actions } from './$types';
import { API_BASE } from '$lib/server/api';

export const actions: Actions = {
	default: async ({ request, cookies, fetch }) => {
		const formData = await request.formData();
		const email = formData.get('email') as string;
		const password = formData.get('password') as string;

		if (!email || !password) {
			return fail(400, { error: 'Email and password are required.' });
		}

		try {
			const res = await fetch(`${API_BASE}/auth/login`, {
				method: 'POST',
				headers: { 'Content-Type': 'application/json' },
				body: JSON.stringify({ email, password })
			});

			const body = await res.json();

			if (!res.ok || body.error) {
				return fail(res.status, {
					error: body.error?.message || 'Invalid credentials.'
				});
			}

			const token = body.data?.token;
			if (!token) {
				return fail(500, { error: 'Login succeeded but no token was returned.' });
			}

			cookies.set('jc_token', token, {
				path: '/',
				httpOnly: true,
				secure: true,
				sameSite: 'lax',
				domain: '.japaconnect.com',
				maxAge: 60 * 60 * 24 * 7
			});

			return redirect(302, '/');
		} catch {
			return fail(502, { error: 'Could not reach authentication service.' });
		}
	}
};
```

- [ ] **Step 4: Confirm the source no longer references `VITE_API_URL`**

Run: `grep -n "VITE_API_URL" apps/control/src/routes/login/+page.server.ts`
Expected: no output (no matches)

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd apps/control && bun run test -- login/page.server.test.ts`
Expected: PASS

- [ ] **Step 6: Fix `i18n.svelte.ts`'s `loadTranslations` (currently unused, but must not carry the same bug forward)**

```ts
// apps/control/src/lib/stores/i18n.svelte.ts
import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';

type Locale = string;

let currentLocale: Locale = $state('en');
let translations: Record<string, string> = $state({});
let loaded: Record<string, Record<string, string>> = {};

export function getLocale() {
	return {
		get current() {
			return currentLocale;
		},
		set: (locale: Locale) => {
			currentLocale = locale;
			if (browser) {
				document.documentElement.lang = locale;
				document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
			}
		},
		init: (serverLocale: Locale, serverTranslations?: Record<string, string>) => {
			currentLocale = serverLocale;
			if (serverTranslations) {
				translations = serverTranslations;
				loaded[serverLocale] = serverTranslations;
			}
			if (browser) document.documentElement.lang = serverLocale;
		}
	};
}

export const locale = getLocale();

export function t(key: string, fallback?: string): string {
	return translations[key] ?? fallback ?? key;
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translations = loaded[localeCode];
		return loaded[localeCode];
	}
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/i18n/packs/${localeCode}`);
		if (!res.ok) throw new Error(`HTTP ${res.status}`);
		const body = await res.json();
		const data = body?.data ?? body ?? {};
		loaded[localeCode] = data;
		translations = data;
		return data;
	} catch {
		return {};
	}
}
```

- [ ] **Step 7: Confirm no remaining `VITE_API_URL` references in the app**

Run: `grep -rn "VITE_API_URL" apps/control/src`
Expected: no output

- [ ] **Step 8: Type-check and run the full control test suite**

Run: `cd apps/control && bun run check && bun run test`
Expected: no new errors, all tests pass

- [ ] **Step 9: Commit**

```bash
git add apps/control/src/routes/login/+page.server.ts apps/control/src/routes/login/page.server.test.ts apps/control/src/lib/stores/i18n.svelte.ts
git commit -m "fix(control): stop reading the undefined VITE_API_URL env var

AUDIT-FE-CTL-011"
```

---

### Task 5: Fix learn's broken API base URL env var (AUDIT-FE-LRN-007)

**Files:**
- Modify: `apps/learn/src/lib/api.ts`
- Modify: `apps/learn/src/lib/stores/i18n.svelte.ts`
- Test: `apps/learn/src/lib/api.test.ts` (create)

**Interfaces:**
- Consumes: `PUBLIC_JC_API_URL` from `$env/static/public` (already defined in `apps/learn/.env`/`.env.example`).
- Produces: none.

Same root bug as Task 4, in the sibling app: `import.meta.env.VITE_API_URL` is never defined, so `apps/learn/src/lib/api.ts`'s `API_BASE` and `stores/i18n.svelte.ts`'s `loadTranslations` both always fall back to `http://localhost:8080/api/v1`, even in production. `api.ts` is actively used client-side (`getProgress`/`updateProgress` on the lesson page), so today's client-side progress calls silently target `localhost:8080` and fail in every deployed environment. This task only fixes the URL — it does not touch the larger, separately-tracked "3 competing API clients" finding (AUDIT-FE-LRN-021) or the endpoint/body-shape bug (AUDIT-FE-LRN-017).

- [ ] **Step 1: Write the failing test**

```ts
// apps/learn/src/lib/api.test.ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

vi.mock('$env/static/public', () => ({ PUBLIC_JC_API_URL: 'https://api.japaconnect.com' }));

describe('learn api client base URL', () => {
	const originalFetch = global.fetch;

	beforeEach(() => {
		global.fetch = vi.fn().mockResolvedValue(
			new Response(JSON.stringify({ data: [] }), { status: 200 })
		);
	});

	afterEach(() => {
		global.fetch = originalFetch;
		vi.resetModules();
	});

	it('builds request URLs from PUBLIC_JC_API_URL, never localhost:8080', async () => {
		const { api } = await import('./api');
		await api.listTracks();

		const [calledUrl] = (global.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(calledUrl).toContain('https://api.japaconnect.com');
		expect(calledUrl).not.toContain('localhost:8080');
	});
});
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd apps/learn && bun run test -- api.test.ts`
Expected: FAIL — current `API_BASE` reads `import.meta.env.VITE_API_URL` (undefined) and falls back to `http://localhost:8080/api/v1`, so the assertion `not.toContain('localhost:8080')` fails.

- [ ] **Step 3: Fix `lib/api.ts`'s base URL (first line only — everything else in this file is unchanged)**

```ts
// apps/learn/src/lib/api.ts
import { PUBLIC_JC_API_URL } from '$env/static/public';

const API_BASE = `${PUBLIC_JC_API_URL}/api/v1`;

interface ApiOk<T> {
	data: T;
}

interface ApiErr {
	error: { code: string; message: string };
}

let jwtToken: string | null = null;

export function setToken(token: string | null) {
	jwtToken = token;
}

export function getToken(): string | null {
	return jwtToken;
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...Object.fromEntries(Object.entries(init?.headers ?? {}))
	};
	if (jwtToken) {
		headers['Authorization'] = `Bearer ${jwtToken}`;
	}

	const res = await fetch(`${API_BASE}${path}`, {
		...init,
		headers
	});

	const body = (await res.json()) as ApiOk<T> | ApiErr;
	if (!res.ok || 'error' in body) {
		const err = 'error' in body ? body.error : { code: 'unknown', message: res.statusText };
		throw new Error(`${err.code}: ${err.message}`);
	}
	return (body as ApiOk<T>).data;
}

export const api = {
	listTracks() {
		return request<Track[]>('/learn/tracks');
	},
	getTrack(slug: string) {
		return request<TrackDetail>('/learn/tracks/' + slug);
	},
	getLesson(trackSlug: string, moduleSlug: string, lessonSlug: string) {
		return request<Lesson>(
			`/learn/tracks/${trackSlug}/modules/${moduleSlug}/lessons/${lessonSlug}`
		);
	},
	updateProgress(lessonId: string, data: { completed: boolean }) {
		return request<{ progress: number }>('/learn/lessons/' + lessonId + '/progress', {
			method: 'POST',
			body: JSON.stringify(data)
		});
	},
	getProgress(trackId: string) {
		return request<{ track_id: string; progress: number; completed_lessons: string[] }>(
			'/learn/progress/' + trackId
		);
	},
	getCertificates() {
		return request<Certificate[]>('/me/certificates');
	}
};

// Types
export interface Track {
	id: string;
	slug: string;
	title: string;
	description: string;
	thumbnail_url?: string;
	module_count: number;
	lesson_count: number;
	difficulty: string;
	estimated_hours: number;
}

export interface TrackDetail extends Track {
	modules: Module[];
}

export interface Module {
	id: string;
	slug: string;
	title: string;
	description: string;
	order: number;
	lessons: LessonSummary[];
}

export interface LessonSummary {
	id: string;
	slug: string;
	title: string;
	duration_minutes: number;
	order: number;
}

export interface Lesson {
	id: string;
	slug: string;
	title: string;
	body: string;
	content_type: string;
	duration_minutes: number;
	order: number;
	module_id: string;
}

export interface Certificate {
	id: string;
	track_id: string;
	track_title: string;
	issued_at: string;
	certificate_url?: string;
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd apps/learn && bun run test -- api.test.ts`
Expected: PASS

- [ ] **Step 5: Fix `stores/i18n.svelte.ts` the same way as control's (Task 4, Step 6) — identical file today**

```ts
// apps/learn/src/lib/stores/i18n.svelte.ts
import { browser } from '$app/environment';
import { PUBLIC_JC_API_URL } from '$env/static/public';

type Locale = string;

let currentLocale: Locale = $state('en');
let translations: Record<string, string> = $state({});
let loaded: Record<string, Record<string, string>> = {};

export function getLocale() {
	return {
		get current() {
			return currentLocale;
		},
		set: (locale: Locale) => {
			currentLocale = locale;
			if (browser) {
				document.documentElement.lang = locale;
				document.cookie = `jc_locale=${locale};path=/;max-age=31536000;samesite=lax`;
			}
		},
		init: (serverLocale: Locale, serverTranslations?: Record<string, string>) => {
			currentLocale = serverLocale;
			if (serverTranslations) {
				translations = serverTranslations;
				loaded[serverLocale] = serverTranslations;
			}
			if (browser) document.documentElement.lang = serverLocale;
		}
	};
}

export const locale = getLocale();

export function t(key: string, fallback?: string): string {
	return translations[key] ?? fallback ?? key;
}

export async function loadTranslations(localeCode: string): Promise<Record<string, string>> {
	if (loaded[localeCode]) {
		translations = loaded[localeCode];
		return loaded[localeCode];
	}
	try {
		const res = await fetch(`${PUBLIC_JC_API_URL}/api/v1/i18n/packs/${localeCode}`);
		if (!res.ok) throw new Error(`HTTP ${res.status}`);
		const body = await res.json();
		const data = body?.data ?? body ?? {};
		loaded[localeCode] = data;
		translations = data;
		return data;
	} catch {
		return {};
	}
}
```

- [ ] **Step 6: Confirm no remaining `VITE_API_URL` references in the app**

Run: `grep -rn "VITE_API_URL" apps/learn/src`
Expected: no output

- [ ] **Step 7: Type-check and run the full learn test suite**

Run: `cd apps/learn && bun run check && bun run test`
Expected: no new errors, all tests pass

- [ ] **Step 8: Commit**

```bash
git add apps/learn/src/lib/api.ts apps/learn/src/lib/api.test.ts apps/learn/src/lib/stores/i18n.svelte.ts
git commit -m "fix(learn): stop reading the undefined VITE_API_URL env var

AUDIT-FE-LRN-007"
```

---

### Task 6: Wire GlitchTip error tracking in main (AUDIT-FE-MAI-034)

**Files:**
- Modify: `apps/main/package.json` (new dependency)
- Modify: `apps/main/.env`, `apps/main/.env.example`
- Create: `apps/main/src/hooks.client.ts`
- Create: `apps/main/src/hooks.client.test.ts`
- Modify: `apps/main/src/hooks.server.ts`

**Interfaces:**
- Consumes: none.
- Produces: `shouldInitErrorTracking(dsn: string, isDev: boolean): boolean`, exported from `hooks.client.ts` — a small pure predicate, exported purely so it's unit-testable without triggering the module's top-level `Sentry.init` side effect.

`apps/main/.env` currently defines no `PUBLIC_JC_*` variables at all (it uses `PRIVATE_API_BASE_URL`/`VITE_API_URL`/`VITE_SITE_URL`/`VITE_WS_URL` — a separate, larger, already-documented naming gap, AUDIT-FE-MAI-020, intentionally out of scope for this plan). This task adds only the one new variable GlitchTip needs, without renaming anything else.

- [ ] **Step 1: Install the SDK**

Run: `cd apps/main && bun add @sentry/sveltekit`
Expected: `@sentry/sveltekit` added to `dependencies` in `apps/main/package.json`

- [ ] **Step 2: Add the DSN env var**

Add to `apps/main/.env`:
```
PUBLIC_JC_GLITCHTIP_DSN=
```

Add to `apps/main/.env.example`:
```
PUBLIC_JC_GLITCHTIP_DSN=
```

- [ ] **Step 3: Write the failing test for the init predicate**

```ts
// apps/main/src/hooks.client.test.ts
import { describe, it, expect } from 'vitest';
import { shouldInitErrorTracking } from './hooks.client';

describe('shouldInitErrorTracking', () => {
	it('is false when the DSN is empty', () => {
		expect(shouldInitErrorTracking('', false)).toBe(false);
	});

	it('is false in dev mode even with a DSN set', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', true)).toBe(false);
	});

	it('is true with a DSN set and not in dev mode', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', false)).toBe(true);
	});
});
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd apps/main && bun run test -- hooks.client.test.ts`
Expected: FAIL — `apps/main/src/hooks.client.ts` does not exist yet.

- [ ] **Step 5: Create `hooks.client.ts`**

```ts
// apps/main/src/hooks.client.ts
import * as Sentry from '@sentry/sveltekit';
import { dev } from '$app/environment';
import { PUBLIC_JC_GLITCHTIP_DSN } from '$env/static/public';

export function shouldInitErrorTracking(dsn: string, isDev: boolean): boolean {
	return dsn.length > 0 && !isDev;
}

if (shouldInitErrorTracking(PUBLIC_JC_GLITCHTIP_DSN, dev)) {
	Sentry.init({
		dsn: PUBLIC_JC_GLITCHTIP_DSN,
		tracesSampleRate: 0
	});
}

export const handleError = Sentry.handleErrorWithSentry();
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd apps/main && bun run test -- hooks.client.test.ts`
Expected: PASS (3 tests)

- [ ] **Step 7: Wire server-side error capture in `hooks.server.ts`**

```ts
// apps/main/src/hooks.server.ts
import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { API_BASE_URL, apiFetch } from '$lib/server/api';
import type { FeatureFlags } from '$lib/types';

/** Routes that require an authenticated user. */
const PROTECTED_ROUTES = [
	'/notifications',
	'/settings',
	'/settings/security',
	'/create',
	'/onboarding',
	'/messages',
	'/payments',
	'/checkout',
	'/dashboard'
];

/** Check if a pathname requires authentication. */
function isProtectedRoute(pathname: string): boolean {
	return PROTECTED_ROUTES.some(
		(route) => pathname === route || pathname.startsWith(`${route}/`)
	);
}

export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('jc_token');

	if (token) {
		try {
			const res = await fetch(`${API_BASE_URL}/me`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.ok) {
				const { data } = await res.json();
				event.locals.token = token;
				event.locals.user = {
					id: data.id,
					email: data.email,
					username: data.username,
					displayName: data.display_name,
					avatarUrl: data.avatar_url,
					profileType: data.profile_type,
					status: data.status
				};
			} else {
				event.locals.user = null;
			}
		} catch {
			event.locals.user = null;
		}
	} else {
		event.locals.user = null;
	}

	// Redirect unauthenticated users away from protected routes.
	if (isProtectedRoute(event.url.pathname) && !event.locals.user) {
		throw redirect(303, '/login');
	}

	// Fetch feature flags from the backend.
	let featureFlags: FeatureFlags = {};
	try {
		const flagsResult = await apiFetch<FeatureFlags>('/feature-flags');
		if (flagsResult.ok && flagsResult.data) {
			featureFlags = flagsResult.data;
		}
	} catch {
		// Fall back to empty flags on error.
	}
	event.locals.featureFlags = featureFlags;
	event.locals.notificationCount = 0;

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error'
	};
};
```

- [ ] **Step 8: Type-check and run the full main test suite**

Run: `cd apps/main && bun run check && bun run test`
Expected: no new errors, all tests pass (verify `@sentry/sveltekit` resolves cleanly against SvelteKit 2.63 / Svelte 5.56 — if `svelte-check` reports a peer-dependency warning, note it but do not block on a warning, only on a hard error)

- [ ] **Step 9: Commit**

```bash
git add apps/main/package.json apps/main/.env apps/main/.env.example apps/main/src/hooks.client.ts apps/main/src/hooks.client.test.ts apps/main/src/hooks.server.ts
git commit -m "feat(main): wire GlitchTip error tracking

AUDIT-FE-MAI-034"
```

---

### Task 7: Wire GlitchTip error tracking in admin (AUDIT-FE-ADM-028)

**Files:**
- Modify: `apps/admin/package.json` (new dependency)
- Create: `apps/admin/src/hooks.client.ts`
- Create: `apps/admin/src/hooks.client.test.ts`
- Modify: `apps/admin/src/hooks.server.ts`

**Interfaces:**
- Consumes: `PUBLIC_JC_GLITCHTIP_DSN` (already defined in `apps/admin/.env`/`.env.example` — confirmed present).
- Produces: `shouldInitErrorTracking(dsn: string, isDev: boolean): boolean`, same shape as Task 6 (each app keeps its own copy — these apps don't share a lib directory).

Note the caveat from Task 2: `apps/admin` ships as a static SPA (`adapter-static` + `fallback: 'index.html'`), so `hooks.server.ts`'s `handleError` only ever runs at build/prerender time in this app's actual production deployment, not per live request (AUDIT-FE-ADM-002). `hooks.client.ts` is unaffected by that — it always runs in-browser regardless of adapter — so it's the primary payoff here; the `hooks.server.ts` change is still correct to make (it's real, useful code during `bun dev`/prerendering) but won't catch live production server errors until AUDIT-FE-ADM-002 is resolved.

- [ ] **Step 1: Install the SDK**

Run: `cd apps/admin && bun add @sentry/sveltekit`
Expected: `@sentry/sveltekit` added to `dependencies` in `apps/admin/package.json`

- [ ] **Step 2: Write the failing test**

```ts
// apps/admin/src/hooks.client.test.ts
import { describe, it, expect } from 'vitest';
import { shouldInitErrorTracking } from './hooks.client';

describe('shouldInitErrorTracking', () => {
	it('is false when the DSN is empty', () => {
		expect(shouldInitErrorTracking('', false)).toBe(false);
	});

	it('is false in dev mode even with a DSN set', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', true)).toBe(false);
	});

	it('is true with a DSN set and not in dev mode', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', false)).toBe(true);
	});
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd apps/admin && bun run test -- hooks.client.test.ts`
Expected: FAIL — `apps/admin/src/hooks.client.ts` does not exist yet.

- [ ] **Step 4: Create `hooks.client.ts`**

```ts
// apps/admin/src/hooks.client.ts
import * as Sentry from '@sentry/sveltekit';
import { dev } from '$app/environment';
import { PUBLIC_JC_GLITCHTIP_DSN } from '$env/static/public';

export function shouldInitErrorTracking(dsn: string, isDev: boolean): boolean {
	return dsn.length > 0 && !isDev;
}

if (shouldInitErrorTracking(PUBLIC_JC_GLITCHTIP_DSN, dev)) {
	Sentry.init({
		dsn: PUBLIC_JC_GLITCHTIP_DSN,
		tracesSampleRate: 0
	});
}

export const handleError = Sentry.handleErrorWithSentry();
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd apps/admin && bun run test -- hooks.client.test.ts`
Expected: PASS (3 tests)

- [ ] **Step 6: Wire server-side error capture in `hooks.server.ts`**

```ts
// apps/admin/src/hooks.server.ts
import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import { building } from '$app/environment';
import * as Sentry from '@sentry/sveltekit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

const PUBLIC_ROUTES = ['/login', '/_app'];

function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + '/'));
}

export const handle: Handle = async ({ event, resolve }) => {
	if (building) {
		return resolve(event);
	}

	if (isPublicRoute(event.url.pathname) || event.url.pathname.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('jc_token');

	if (!token) {
		throw redirect(303, '/login');
	}

	try {
		const meRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me`, {
			headers: { Authorization: `Bearer ${token}` },
		});

		if (meRes.ok) {
			const { data } = await meRes.json();
			event.locals.user = data;
			event.locals.token = token;
		} else {
			event.cookies.delete('jc_token', { path: '/' });
			throw redirect(303, '/login');
		}
	} catch (e) {
		if (e instanceof Response) throw e;
		event.cookies.delete('jc_token', { path: '/' });
		throw redirect(303, '/login');
	}

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};
```

- [ ] **Step 7: Type-check and run the full admin test suite**

Run: `cd apps/admin && bun run check && bun run test`
Expected: no new errors, all tests pass

- [ ] **Step 8: Commit**

```bash
git add apps/admin/package.json apps/admin/src/hooks.client.ts apps/admin/src/hooks.client.test.ts apps/admin/src/hooks.server.ts
git commit -m "feat(admin): wire GlitchTip error tracking

Server-side capture is only fully effective once AUDIT-FE-ADM-002 is
resolved (this app currently ships as a static SPA with no live server
per request in production) — client-side capture works today regardless.

AUDIT-FE-ADM-028"
```

---

### Task 8: Wire GlitchTip error tracking in control (AUDIT-FE-CTL-042)

**Files:**
- Modify: `apps/control/package.json` (new dependency)
- Create: `apps/control/src/hooks.client.ts`
- Create: `apps/control/src/hooks.client.test.ts`
- Modify: `apps/control/src/hooks.server.ts`

**Interfaces:**
- Consumes: `PUBLIC_JC_GLITCHTIP_DSN` (already defined in `apps/control/.env`/`.env.example` — confirmed present).
- Produces: `shouldInitErrorTracking(dsn: string, isDev: boolean): boolean`, same shape as Tasks 6/7.

Unlike admin, `apps/control` uses `adapter-node` — a real server runs per request in production, so this app's `handleError` capture is fully effective, not just a dev-time nicety.

- [ ] **Step 1: Install the SDK**

Run: `cd apps/control && bun add @sentry/sveltekit`
Expected: `@sentry/sveltekit` added to `dependencies` in `apps/control/package.json`

- [ ] **Step 2: Write the failing test**

```ts
// apps/control/src/hooks.client.test.ts
import { describe, it, expect } from 'vitest';
import { shouldInitErrorTracking } from './hooks.client';

describe('shouldInitErrorTracking', () => {
	it('is false when the DSN is empty', () => {
		expect(shouldInitErrorTracking('', false)).toBe(false);
	});

	it('is false in dev mode even with a DSN set', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', true)).toBe(false);
	});

	it('is true with a DSN set and not in dev mode', () => {
		expect(shouldInitErrorTracking('https://key@glitchtip.example/1', false)).toBe(true);
	});
});
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd apps/control && bun run test -- hooks.client.test.ts`
Expected: FAIL — `apps/control/src/hooks.client.ts` does not exist yet.

- [ ] **Step 4: Create `hooks.client.ts`**

```ts
// apps/control/src/hooks.client.ts
import * as Sentry from '@sentry/sveltekit';
import { dev } from '$app/environment';
import { PUBLIC_JC_GLITCHTIP_DSN } from '$env/static/public';

export function shouldInitErrorTracking(dsn: string, isDev: boolean): boolean {
	return dsn.length > 0 && !isDev;
}

if (shouldInitErrorTracking(PUBLIC_JC_GLITCHTIP_DSN, dev)) {
	Sentry.init({
		dsn: PUBLIC_JC_GLITCHTIP_DSN,
		tracesSampleRate: 0
	});
}

export const handleError = Sentry.handleErrorWithSentry();
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd apps/control && bun run test -- hooks.client.test.ts`
Expected: PASS (3 tests)

- [ ] **Step 6: Wire server-side error capture in `hooks.server.ts`**

```ts
// apps/control/src/hooks.server.ts
import { redirect, type Handle, type HandleServerError } from '@sveltejs/kit';
import * as Sentry from '@sentry/sveltekit';
import { PUBLIC_JC_API_URL } from '$env/static/public';

const PUBLIC_ROUTES = ['/login', '/_app'];

function isPublicRoute(pathname: string): boolean {
	return PUBLIC_ROUTES.some((route) => pathname === route || pathname.startsWith(route + '/'));
}

export const handle: Handle = async ({ event, resolve }) => {
	if (isPublicRoute(event.url.pathname) || event.url.pathname.includes('.')) {
		return resolve(event);
	}

	const token = event.cookies.get('jc_token');

	if (!token) {
		throw redirect(303, '/login');
	}

	try {
		const meRes = await fetch(`${PUBLIC_JC_API_URL}/api/v1/me`, {
			headers: { Authorization: `Bearer ${token}` },
		});

		if (meRes.ok) {
			const { data } = await meRes.json();
			event.locals.user = data;
			event.locals.token = token;

			if (data.user_group_name !== 'super_admin') {
				return new Response(
					`<!DOCTYPE html><html><head><title>403</title></head><body>
					<h1>403 — Insufficient Privileges</h1>
					<p>The Control Panel is restricted to super_admin users only.</p>
					<a href="/login">Try a different account</a>
					</body></html>`,
					{ status: 403, headers: { 'Content-Type': 'text/html' } }
				);
			}
		} else {
			event.cookies.delete('jc_token', { path: '/' });
			throw redirect(303, '/login');
		}
	} catch (e) {
		if (e instanceof Response) throw e;
		event.cookies.delete('jc_token', { path: '/' });
		throw redirect(303, '/login');
	}

	const response = await resolve(event);
	return response;
};

export const handleError: HandleServerError = ({ error }) => {
	Sentry.captureException(error);
	return {
		code: 'INTERNAL_ERROR',
		message: 'Internal error',
	};
};
```

- [ ] **Step 7: Type-check and run the full control test suite**

Run: `cd apps/control && bun run check && bun run test`
Expected: no new errors, all tests pass

- [ ] **Step 8: Commit**

```bash
git add apps/control/package.json apps/control/src/hooks.client.ts apps/control/src/hooks.client.test.ts apps/control/src/hooks.server.ts
git commit -m "feat(control): wire GlitchTip error tracking

AUDIT-FE-CTL-042"
```

---

### Task 9: Full verification pass across all touched apps

**Files:** none (verification only)

**Interfaces:** none.

- [ ] **Step 1: Run type-check + full test suite for every app touched by this plan**

```bash
for app in admin control learn main; do
  echo "=== $app ===";
  (cd apps/$app && bun run check && bun run test) || exit 1
done
```

Expected: all four apps report zero `svelte-check` errors and all Vitest suites passing (including every test written in Tasks 1-8, plus every pre-existing test in each app — this catches any cross-task regression, e.g. Task 1's `permissions.ts` signature change breaking some other untouched file that still called `fetchPermissions(token)`).

- [ ] **Step 2: Grep-confirm the two fixed bug patterns are fully gone**

```bash
grep -rn "VITE_API_URL" apps/control/src apps/learn/src apps/main/src/lib/api.ts apps/main/src/lib/stores/i18n.svelte.ts 2>/dev/null
```

Expected: no output (the `apps/main` env-var rename beyond `api.ts` is explicitly out of scope for this plan — `PRIVATE_API_BASE_URL`/`VITE_SITE_URL`/`VITE_WS_URL` references elsewhere in `apps/main` are expected and untouched).

- [ ] **Step 3: No commit for this task** — it's a verification gate, not a code change. If Step 1 or 2 finds a regression, fix it inside the task that introduced it and re-run this task.

---

## Self-Review Notes

- **Spec coverage:** every finding named in the user-selected "cross-cutting critical fixes" scope is covered: AUDIT-FE-ADM-001 (Task 1), AUDIT-FE-ADM-003 (Task 2, explicitly partial), AUDIT-FE-CTL-010 (Task 3), AUDIT-FE-CTL-011 (Task 4), AUDIT-FE-LRN-007 (Task 5), AUDIT-FE-MAI-034/AUDIT-FE-ADM-028/AUDIT-FE-CTL-042 (Tasks 6-8).
- **Descoped, and why:** AUDIT-FE-MAI-020 (main's full `PUBLIC_JC_*` env-var rename) turned out, on inspection, to be a Medium-severity naming/standards gap rather than a live functional bug like its control/learn counterparts — main's `.env` already defines a consistent (if non-compliant) `VITE_API_URL` value, so the app isn't actually broken today. Renaming it touches `VITE_SITE_URL`/`VITE_WS_URL`/`PRIVATE_API_BASE_URL` too and needs its own plan. AUDIT-FE-ADM-002 (admin's `adapter-static` vs live-server question) is a deployment-model decision, not a mechanical fix, and blocks a full resolution of AUDIT-FE-ADM-003 — flagged clearly in Task 2 rather than silently worked around.
- **Type consistency:** `fetchPermissions()` signature change (Task 1) is grepped for other call sites before assuming it's safe (none found beyond the one call site being fixed). `data.token` is deliberately kept as a static `null` in admin's layout return type so the two legacy `$lib/api.ts` call sites (`users/+page.svelte`, `users/[userId]/+page.svelte`) keep compiling unchanged.
