import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGetOrNull } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { Scholarship, SavedScholarship } from '$lib/types';

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
	if (!isToolEnabled(tool, event.locals.siteSettings)) {
		return { enabled: false, tool, scholarships: [], saved: [], user: event.locals.user };
	}

	const ctx = apiContext(event);

	// Pass through any filter params from the URL for SSR + shareability.
	const searchParams = event.url.searchParams;
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

	return {
		enabled: true,
		tool,
		scholarships,
		saved,
		user: event.locals.user
	};
};
