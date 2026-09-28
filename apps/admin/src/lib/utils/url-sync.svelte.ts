import { page } from '$app/state';
import { replaceState } from '$app/navigation';
import { untrack } from 'svelte';

/**
 * Read the current page number (0-based) from the URL `?page=` param.
 * The URL stores 1-based page numbers (matching Pagination.svelte), so we
 * subtract 1. Returns 0 if the param is absent or invalid.
 */
export function pageFromUrl(): number {
	const raw = page.url.searchParams.get('page');
	if (!raw) return 0;
	const n = parseInt(raw, 10);
	return isNaN(n) || n < 1 ? 0 : n - 1;
}

/**
 * Read the page size from the URL `?pageSize=` param.
 * Returns the provided default if absent or invalid.
 */
export function pageSizeFromUrl(defaultSize = 25): number {
	const raw = page.url.searchParams.get('pageSize');
	if (!raw) return defaultSize;
	const n = parseInt(raw, 10);
	return isNaN(n) || n < 1 ? defaultSize : n;
}

/**
 * Read a search query from the URL `?q=` (or `?search=`) param.
 */
export function searchFromUrl(): string {
	return page.url.searchParams.get('q') ?? page.url.searchParams.get('search') ?? '';
}

/**
 * Read a single filter value from a URL query param.
 */
export function filterFromUrl(key: string): string {
	return page.url.searchParams.get(key) ?? '';
}

/**
 * Write pagination + search + filter state to the URL via `replaceState`
 * (no navigation, no scroll). Matches the pattern used by Pagination.svelte
 * and DataTable's own urlSync.
 *
 * Pass the current state; only non-default values are written (page=0 and
 * pageSize=default are omitted to keep URLs clean).
 */
export function syncListStateToUrl(opts: {
	page: number;
	pageSize: number;
	defaultPageSize?: number;
	search?: string;
	filters?: Record<string, string>;
	searchParam?: string;
}): void {
	if (typeof window === 'undefined') return;
	const { page: p, pageSize: ps, defaultPageSize = 25, search = '', filters = {}, searchParam = 'q' } = opts;
	const params = new URLSearchParams(page.url.searchParams.toString());

	// Preserve sort/dir (managed by DataTable) — don't touch them.
	// Page: 1-based in URL, omit when 0.
	if (p > 0) params.set('page', String(p + 1));
	else params.delete('page');

	// PageSize: omit when default.
	if (ps !== defaultPageSize) params.set('pageSize', String(ps));
	else params.delete('pageSize');

	// Search
	if (search) params.set(searchParam, search);
	else params.delete(searchParam);

	// Filters: each key becomes its own query param
	for (const [key, val] of Object.entries(filters)) {
		if (val) params.set(key, val);
		else params.delete(key);
	}

	const qs = params.toString();
	const url = `${page.url.pathname}${qs ? `?${qs}` : ''}`;
	try {
		replaceState(url, {});
	} catch {
		// ignore URL sync failures in non-browser contexts
	}
}

/**
 * Read the active tab from the URL `?tab=` param (or a custom param name).
 * Returns '' when absent; pass `valid` to reject unknown values.
 */
export function tabFromUrl(param = 'tab', valid?: readonly string[]): string {
	const raw = page.url.searchParams.get(param) ?? '';
	return valid ? (valid.includes(raw) ? raw : '') : raw;
}

/**
 * Write the active tab to the URL via `replaceState` (no navigation, no
 * scroll). With `exclusive: true` every other query param is dropped —
 * use this for pages whose tabs each host their own url-synced list state
 * (EntityPage/DataTable) so stale `page`/`q`/filter params do not leak
 * across tabs (same convention as community/wiki).
 */
export function syncTabToUrl(
	tab: string,
	opts?: { param?: string; preserve?: readonly string[] }
): void {
	if (typeof window === 'undefined') return;
	const param = opts?.param ?? 'tab';
	let url: string;
	if (opts?.preserve) {
		// Wipe every query param except the tab param and the preserved keys —
		// use for pages whose tabs each host their own url-synced list state
		// (EntityPage/DataTable) so stale page/q/sort/filter params do not leak
		// across tabs (same convention as community/wiki). Keep page-level
		// params such as ?from=&to= by listing them in `preserve`.
		const params = new URLSearchParams();
		if (tab) params.set(param, tab);
		for (const key of opts.preserve) {
			const v = page.url.searchParams.get(key);
			if (v !== null) params.set(key, v);
		}
		const qs = params.toString();
		url = `${page.url.pathname}${qs ? `?${qs}` : ''}`;
	} else {
		const params = new URLSearchParams(page.url.searchParams.toString());
		if (tab) params.set(param, tab);
		else params.delete(param);
		const qs = params.toString();
		url = `${page.url.pathname}${qs ? `?${qs}` : ''}`;
	}
	try {
		replaceState(url, {});
	} catch {
		// ignore URL sync failures in non-browser contexts
	}
}

/**
 * Two-way sync between a `$state` tab variable and a URL query param.
 * Call once during component init; installs two effects:
 *   URL → state  (back/forward navigation and manual URL edits restore the tab)
 *   state → URL  (any assignment to the state var writes `?tab=`)
 * Ordering matters: the URL→state effect is registered first so a
 * back/forward URL change updates state before the state→URL effect runs
 * (it then sees equality and does not write the stale value back).
 * Pass `preserve` to keep specific params (e.g. ['from','to']) while
 * wiping the rest on tab switch; omit to merge with existing params.
 */
export function bindTabParam<T extends string>(
	get: () => T,
	set: (v: T) => void,
	opts?: { param?: string; valid?: readonly T[] | (() => readonly T[]); preserve?: readonly string[] }
): void {
	const param = opts?.param ?? 'tab';
	const preserve = opts?.preserve;
	const isValid = (v: string): boolean => {
		const valid = typeof opts?.valid === 'function' ? opts.valid() : opts?.valid;
		return !valid || (valid as readonly string[]).includes(v);
	};
	$effect(() => {
		const raw = page.url.searchParams.get(param) ?? '';
		if (raw && isValid(raw) && raw !== untrack(get)) set(raw as T);
	});
	$effect(() => {
		const cur = get();
		if ((page.url.searchParams.get(param) ?? '') !== cur) syncTabToUrl(cur, { param, preserve });
	});
}

/**
 * Seed a tab `$state` initializer from the URL — use as
 * `let activeTab = $state(initTabParam('default', { valid: [...] }))`.
 * Pair with `bindTabParam` for ongoing two-way sync.
 */
export function initTabParam<T extends string>(
	defaultTab: T,
	opts?: { param?: string; valid?: readonly T[] }
): T {
	const param = opts?.param ?? 'tab';
	if (typeof window === 'undefined') return defaultTab;
	const raw = new URL(window.location.href).searchParams.get(param) ?? '';
	const valid = opts?.valid;
	if (raw && (!valid || (valid as readonly string[]).includes(raw))) return raw as T;
	return defaultTab;
}
