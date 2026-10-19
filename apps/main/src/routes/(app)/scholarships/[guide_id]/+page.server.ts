import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
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

	const [scholarshipRes, savedItem, statuses] = await Promise.all([
		apiGet<Scholarship>(ctx, `/scholarships/${guideId}`).catch(() => null),
		// Server-side guide_id filter — correct even when the user has more
		// saved scholarships than a single page would return.
		event.locals.user
			? apiList<SavedScholarship>(
					ctx,
					`/scholarships/saved?guide_id=${encodeURIComponent(guideId)}&page_size=1`
				)
					.then((r) => r.items[0] ?? null)
					.catch(() => null)
			: Promise.resolve(null),
		apiGet<{ value: string; label: string }[]>(ctx, '/scholarships/statuses').catch(
			() => [] as { value: string; label: string }[]
		)
	]);
	const scholarship = scholarshipRes;

	if (!scholarship) {
		throw error(404, { code: 'scholarship_not_found', message: 'Scholarship not found' });
	}

	return {
		scholarship,
		savedItem,
		statuses,
		authed: !!event.locals.user,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
	};
};
