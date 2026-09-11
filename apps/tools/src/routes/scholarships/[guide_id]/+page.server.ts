import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetOrNull, apiList } from '$lib/server/api';
import type { ScholarshipDetail, SavedScholarship, CatalogOption } from '$lib/types';

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
	const ctx = apiContext(event);
	const guideId = event.params.guide_id;

	let scholarship: ScholarshipDetail | null = null;
	try {
		scholarship = await apiGet<ScholarshipDetail>(ctx, `/scholarships/${guideId}`);
	} catch (err) {
		// A 404 means the scholarship doesn't exist — surface a proper 404.
		if (err instanceof Error && 'status' in err && (err as { status: number }).status === 404) {
			throw error(404, { code: 'scholarship_not_found', message: 'Scholarship not found' });
		}
		scholarship = null;
	}

	// For authenticated users, find the saved record for this scholarship
	// (if any) so the detail page can show the save status + notes editor.
	let savedItem: SavedScholarship | null = null;
	if (event.locals.user && scholarship) {
		try {
			const res = await apiGetOrNull<SavedScholarship[]>(ctx, '/scholarships/saved?page_size=100');
			if (res) {
				savedItem = res.find((s) => s.scholarship.guide_id === guideId) ?? null;
			}
		} catch {
			savedItem = null;
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

	return {
		scholarship,
		savedItem,
		statusOptions,
		user: event.locals.user
	};
};
