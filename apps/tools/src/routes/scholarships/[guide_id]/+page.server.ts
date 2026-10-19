import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { ScholarshipDetail, SavedScholarship, CatalogOption } from '$lib/types';
import { defaultReminderDays } from '$lib/scholarships/defaultReminder';

/**
 * Scholarship detail page — §73, ADR-073.
 * SSR loads the full scholarship detail (eligibility, how-to-apply, contact)
 * and the user's saved record if authenticated.
 *
 * API: GET /scholarships/:guide_id — full detail.
 *      GET /scholarships/saved — user's saved list (auth only, used to
 *      determine if this scholarship is already saved and its status).
 *
 * Ref: §73 (Scholarship Finder), ROADMAP-TOOLS 02.05.
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('scholarships')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			scholarship: null,
			savedItem: null,
			statusOptions: [],
			user: null,
			defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
		};
	}

	const ctx = apiContext(event);
	const guideId = event.params.guide_id;

	// Detail, saved list, and canonical status options (Rule 16 — no
	// hardcoded list) are independent — one wave. The saved lookup only
	// needs the user session; the find narrows it to this guide.
	const [scholarshipRes, savedList, statusOptions] = await Promise.all([
		apiGet<ScholarshipDetail>(ctx, `/scholarships/${guideId}`).catch((err) => {
			// A 404 means the scholarship doesn't exist — surface a proper 404.
			if (err instanceof Error && 'status' in err && (err as { status: number }).status === 404) {
				throw error(404, { code: 'scholarship_not_found', message: 'Scholarship not found' });
			}
			return null;
		}),
		event.locals.user
			? apiGetOrNull<SavedScholarship[]>(ctx, '/scholarships/saved?page_size=100').catch(() => null)
			: Promise.resolve(null),
		apiList<CatalogOption>(ctx, '/scholarships/statuses')
			.then((r) => r.items)
			.catch(() => [] as CatalogOption[])
	]);
	const scholarship = scholarshipRes;

	// For authenticated users, find the saved record for this scholarship
	// (if any) so the detail page can show the save status + notes editor.
	const savedItem =
		scholarship && savedList
			? (savedList.find((s) => s.scholarship.guide_id === guideId) ?? null)
			: null;

	return {
		enabled: true,
		tool,
		scholarship,
		savedItem,
		statusOptions,
		user: event.locals.user,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
	};
};
