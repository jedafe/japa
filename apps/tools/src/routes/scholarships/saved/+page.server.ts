import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { SavedScholarship } from '$lib/types';

/**
 * Saved Scholarships tracker — §73, ADR-073.
 * Shows all scholarships the authenticated user has saved, with their
 * application status, notes, and reminder configuration. Links to each
 * scholarship's detail page for editing.
 *
 * Anonymous users are redirected to the Main app login (the saved tracker
 * is an auth-only feature — there is no anonymous state to show).
 *
 * API: GET /scholarships/saved — user's saved list + statuses (auth only).
 *
 * Ref: §73 (Scholarship Finder), ROADMAP-TOOLS 02.05.
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(302, '/scholarships');
	}

	const ctx = apiContext(event);

	let saved: SavedScholarship[] = [];
	try {
		const res = await apiList<SavedScholarship>(ctx, '/scholarships/saved');
		saved = res.items;
	} catch {
		saved = [];
	}

	return {
		saved,
		user: event.locals.user
	};
};
