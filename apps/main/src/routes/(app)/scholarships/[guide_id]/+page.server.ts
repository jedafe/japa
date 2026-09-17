import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Scholarship, SavedScholarship } from '$lib/types';
import { defaultReminderDays } from '$lib/scholarships/defaultReminder';

/**
 * Scholarship detail page — Section 73, ADR-073.
 * SSR loads the full scholarship guide and, for authenticated users, the
 * user's saved record and status options.
 *
 * API: GET /scholarships/:guide_id — full scholarship detail.
 *      GET /scholarships/saved — user's saved list (auth only).
 *      GET /scholarships/statuses — canonical status options.
 *
 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['scholarship_finder.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scholarship Finder is not available' });
	}

	const ctx = apiContext(event);
	const guideId = event.params.guide_id;

	let scholarship: Scholarship | null = null;
	try {
		scholarship = await apiGet<Scholarship>(ctx, `/scholarships/${guideId}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'scholarship_not_found', message: 'Scholarship not found' });
		}
		scholarship = null;
	}

	if (!scholarship) {
		throw error(404, { code: 'scholarship_not_found', message: 'Scholarship not found' });
	}

	let savedItem: SavedScholarship | null = null;
	if (event.locals.user) {
		try {
			// Server-side guide_id filter — correct even when the user has more
			// saved scholarships than a single page would return.
			const res = await apiList<SavedScholarship>(
				ctx,
				`/scholarships/saved?guide_id=${encodeURIComponent(guideId)}&page_size=1`
			);
			savedItem = res.items[0] ?? null;
		} catch {
			savedItem = null;
		}
	}

	let statuses: { value: string; label: string }[] = [];
	try {
		statuses = await apiGet<{ value: string; label: string }[]>(ctx, '/scholarships/statuses');
	} catch {
		statuses = [];
	}

	return {
		scholarship,
		savedItem,
		statuses,
		authed: !!event.locals.user,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
	};
};
