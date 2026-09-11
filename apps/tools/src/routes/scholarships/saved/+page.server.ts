import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { SavedScholarship, CatalogOption } from '$lib/types';
import { defaultReminderDays } from '$lib/scholarships/defaultReminder';

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
	const tool = getTool('scholarships')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			saved: [],
			statusOptions: [],
			user: null,
			defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
		};
	}

	if (!event.locals.user) {
		throw redirect(302, '/scholarships');
	}

	const ctx = apiContext(event);

	let saved: SavedScholarship[] = [];
	try {
		const res = await apiList<SavedScholarship>(ctx, '/scholarships/saved?page_size=100');
		saved = res.items;
	} catch {
		saved = [];
	}

	// Canonical status options from the backend (Rule 16 — no hardcoded list).
	let statusOptions: CatalogOption[] = [];
	try {
		const res = await apiList<CatalogOption>(ctx, '/scholarships/statuses');
		statusOptions = res.items;
	} catch {
		statusOptions = [];
	}

	return {
		enabled: true,
		tool,
		saved,
		statusOptions,
		user: event.locals.user,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
	};
};
