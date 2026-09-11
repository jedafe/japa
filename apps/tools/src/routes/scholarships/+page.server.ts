import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { Scholarship, SavedScholarship, GeoSpaceOption, CatalogOption } from '$lib/types';
import { loadCatalog } from '$lib/server/catalog';
import { defaultReminderDays } from '$lib/scholarships/defaultReminder';

const PAGE_SIZE = 20;

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
		return {
			enabled: false,
			tool,
			countries: [],
			scholarships: [],
			saved: [],
			statusOptions: [],
			renewableOptions: [],
			user: event.locals.user,
			defaultReminderDays: defaultReminderDays(event.locals.siteSettings),
			currentPage: 1,
			pageSize: PAGE_SIZE,
			total: 0
		};
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
	// Pagination is 1-based in the URL, converted to 0-based page_offset.
	const searchParams = event.url.searchParams;
	const page = Math.max(1, parseInt(searchParams.get('page') ?? '1', 10) || 1);
	const pageSize = Math.max(1, Math.min(100, parseInt(searchParams.get('page_size') ?? String(PAGE_SIZE), 10) || PAGE_SIZE));
	searchParams.set('page_size', String(pageSize));
	searchParams.set('page_offset', String((page - 1) * pageSize));
	if (!searchParams.has('page')) {
		// Keep the canonical URL clean on first load; still include page in the
		// API path so the backend sees the offset.
		searchParams.set('page', String(page));
	}
	const filterQuery = searchParams.toString();
	const filterPath = filterQuery ? `/scholarships?${filterQuery}` : '/scholarships';

	let scholarships: Scholarship[] = [];
	let total = 0;
	try {
		const res = await apiList<Scholarship>(ctx, filterPath);
		scholarships = res.items;
		total = res.meta?.total ?? scholarships.length;
	} catch {
		scholarships = [];
		total = 0;
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
		user: event.locals.user,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings),
		currentPage: page,
		pageSize,
		total
	};
};
