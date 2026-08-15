import { page } from '$app/state';
import { replaceState } from '$app/navigation';

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
		window.history.replaceState({}, '', url);
	}
}
