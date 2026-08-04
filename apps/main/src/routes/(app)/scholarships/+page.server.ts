import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Scholarship } from '$lib/types';

/**
 * Scholarship Finder at /scholarships (Section 73, ADR-073).
 * SSR-fetches the scholarship list with query filters so the page is
 * indexable and renders without a client round-trip.
 *
 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

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

	let scholarships: Scholarship[] = [];
	let disabled = false;
	try {
		const res = await apiList<Scholarship>(ctx, `/scholarships?${params.toString()}`);
		scholarships = res.items;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'scholarship_finder_disabled' || status === 404) {
			disabled = true;
		}
	}

	return { scholarships, disabled, authed, pageSize: PAGE_SIZE };
};
