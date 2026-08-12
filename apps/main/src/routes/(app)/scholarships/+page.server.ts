import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Scholarship, SavedScholarship } from '$lib/types';

/**
 * Scholarship Finder at /scholarships (Section 73, ADR-073).
 * SSR-fetches the scholarship list with query filters so the page is
 * indexable and renders without a client round-trip. For authenticated
 * users, also fetches saved scholarships with their application status
 * so the page can render the application tracker (Phase 33.04).
 *
 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
 */
const PAGE_SIZE = 20;

export interface SavedScholarshipView {
	id: string;
	guide_id: string;
	title: string;
	slug: string;
	summary?: string;
	status: string;
	notes?: string;
	remind_before_days?: number | null;
	updated_at?: string;
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// 1-based page index (matches PaginationControls + the /jobs page
	// convention). Default 1; clamp negative/NaN to 1.
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);

	const params = new URLSearchParams();
	const q = event.url.searchParams.get('q');
	const country = event.url.searchParams.get('country');
	const level = event.url.searchParams.get('level');
	const sort = event.url.searchParams.get('sort');
	const renewable = event.url.searchParams.get('renewable');
	if (q) params.set('q', q);
	if (country) params.set('country', country);
	if (level) params.set('level', level);
	if (sort) params.set('sort', sort);
	if (renewable) params.set('renewable', renewable);
	params.set('page_size', String(PAGE_SIZE));
	params.set('page_offset', String((page - 1) * PAGE_SIZE));

	let scholarships: Scholarship[] = [];
	let disabled = false;
	let total = 0;
	try {
		const res = await apiList<Scholarship>(ctx, `/scholarships?${params.toString()}`);
		scholarships = res.items;
		total = res.meta?.total ?? scholarships.length;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'scholarship_finder_disabled' || status === 404) {
			disabled = true;
		}
	}

	// Phase 33.04 — fetch the authenticated user's saved scholarships with
	// their application status so the page can render the tracker. The
	// backend returns SavedScholarship rows with a nested `scholarship`
	// object; we project a flat view for the tracker list. Best-effort:
	// guests and any backend error degrade to an empty list.
	let savedScholarships: SavedScholarshipView[] = [];
	let savedIds: string[] = [];
	if (authed) {
		try {
			const res = await apiList<SavedScholarship>(ctx, '/scholarships/saved?page_size=100');
			savedScholarships = (res.items ?? []).map((s) => ({
				id: s.id,
				guide_id: s.scholarship?.guide_id ?? '',
				title: s.scholarship?.title ?? '',
				slug: s.scholarship?.slug ?? '',
				summary: s.scholarship?.summary,
				status: s.status,
				notes: s.notes,
				remind_before_days: s.remind_before_days,
				updated_at: s.updated_at
			}));
			savedIds = savedScholarships.map((s) => s.guide_id);
		} catch {
			savedScholarships = [];
		}
	}

	return { scholarships, disabled, authed, pageSize: PAGE_SIZE, currentPage: page, total, savedScholarships, savedIds };
};
