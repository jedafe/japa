import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Scholarship } from '$lib/types';

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

export interface SavedScholarship {
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

	const page = Math.max(0, parseInt(event.url.searchParams.get('page') ?? '0', 10) || 0);

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
	params.set('limit', String(PAGE_SIZE));
	params.set('page_offset', String(page * PAGE_SIZE));

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
	// their application status so the page can render the tracker. Best-effort:
	// guests and any backend error degrade to an empty list.
	let savedScholarships: SavedScholarship[] = [];
	let savedIds: string[] = [];
	if (authed) {
		try {
			const res = await apiGet<{ scholarships: SavedScholarship[]; total: number }>(
				ctx,
				'/scholarships/saved?limit=100'
			);
			savedScholarships = res?.scholarships ?? [];
			savedIds = savedScholarships.map((s) => s.guide_id);
		} catch {
			savedScholarships = [];
		}
	}

	return { scholarships, disabled, authed, pageSize: PAGE_SIZE, currentPage: page, total, savedScholarships, savedIds };
};
