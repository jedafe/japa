import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import type { SavedScholarship } from '$lib/types';

/**
 * Saved Scholarships tracker — Section 73, ADR-073.
 * Shows all scholarships the authenticated user has saved, with their
 * application status, notes, and reminder configuration.
 *
 * Anonymous users are redirected to the login page.
 *
 * API: GET /scholarships/saved — user's saved list (auth only).
 *      GET /scholarships/statuses — canonical status options.
 *
 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['scholarship_finder.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scholarship Finder is not available' });
	}

	if (!event.locals.user) {
		throw redirect(302, '/login?redirect=/scholarships/saved');
	}

	const ctx = apiContext(event);

	let saved: SavedScholarship[] = [];
	try {
		const res = await apiList<SavedScholarship>(ctx, '/scholarships/saved?page_size=100');
		saved = res.items;
	} catch {
		saved = [];
	}

	let statuses: { value: string; label: string }[] = [];
	try {
		statuses = await apiGet<{ value: string; label: string }[]>(ctx, '/scholarships/statuses');
	} catch {
		statuses = [];
	}

	return { saved, statuses };
};
