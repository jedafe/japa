import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { Scholarship, SavedScholarship, GeoSpaceOption, CatalogOption } from '$lib/types';
import { loadCatalog } from '$lib/server/catalog';

/**
 * Scholarship Finder — §73, ADR-073.
 * SSR loads the initial scholarship list (with any URL filter params) and
 * the user's saved scholarships (if authenticated).
 *
 * API: GET /scholarships — finder with filter query params.
 *      GET /scholarships/saved — user's saved list + statuses (auth only).
 *
 * Ref: §73 (Scholarship Finder), ROADMAP-TOOLS 02.05.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when scholarship_finder.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('scholarships')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, countries: [], scholarships: [], saved: [], statusOptions: [], renewableOptions: [], user: event.locals.user };
	}

	const ctx = apiContext(event);

	// Country list — the geo-aware selector source (Rule 16).
	let countries: GeoSpaceOption[] = [];
	try {
		countries = await loadCountries(ctx);
	} catch {
		countries = [];
	}

	// Pass through any filter params from the URL for SSR + shareability.
	const searchParams = event.url.searchParams;
	if (!searchParams.has('page_size')) {
		searchParams.set('page_size', '100');
	}
	const filterQuery = searchParams.toString();
	const filterPath = filterQuery ? `/scholarships?${filterQuery}` : '/scholarships';

	let scholarships: Scholarship[] = [];
	try {
		const res = await apiList<Scholarship>(ctx, filterPath);
		scholarships = res.items;
	} catch {
		scholarships = [];
	}

	// Load saved scholarships for authenticated users.
	let saved: SavedScholarship[] = [];
	if (event.locals.user) {
		try {
			const res = await apiList<SavedScholarship>(ctx, '/scholarships/saved');
			saved = res.items;
		} catch {
			saved = [];
		}
	}

	// Canonical status options from the backend (Rule 16 — no hardcoded list).
	let statusOptions: CatalogOption[] = [];
	try {
		const res = await apiList<CatalogOption>(ctx, '/scholarships/statuses');
		statusOptions = res.items;
	} catch {
		statusOptions = [];
	}

	// Renewable filter options from the public catalog (Rule 16).
	let renewableOptions: CatalogOption[] = [];
	try {
		renewableOptions = await loadCatalog(ctx, 'scholarships.renewable');
	} catch {
		renewableOptions = [];
	}

	return {
		enabled: true,
		tool,
		countries,
		scholarships,
		saved,
		statusOptions,
		renewableOptions,
		user: event.locals.user
	};
};
