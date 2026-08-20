import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { JobListing, GeoCountry } from '$lib/types';

/**
 * Jobs Board finder at /jobs (Section 63, ADR-081).
 * SSR-fetches the job listings with query filters so the page is
 * indexable and renders without a client round-trip.
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const params = new URLSearchParams();
	const q = event.url.searchParams.get('q');
	const country = event.url.searchParams.get('country');
	const jobType = event.url.searchParams.get('job_type');
	const remote = event.url.searchParams.get('remote');
	const page = event.url.searchParams.get('page') ?? '1';
	if (q) params.set('q', q);
	if (country) params.set('country', country);
	if (jobType) params.set('job_type', jobType);
	if (remote) params.set('remote', remote);
	params.set('page', page);
	params.set('limit', String(PAGE_SIZE));

	let jobs: JobListing[] = [];
	let total = 0;
	let disabled = false;
	try {
		const res = await apiList<JobListing>(ctx, `/jobs?${params.toString()}`);
		jobs = res.items;
		total = res.meta?.total ?? 0;
	} catch (e: unknown) {
		const code = (e as { code?: string })?.code;
		const status = (e as { status?: number })?.status;
		if (code === 'jobs_disabled' || status === 404) {
			disabled = true;
		}
	}

	// Canonical country list for the filter dropdown (ADR-005).
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	return { jobs, total, disabled, authed, pageSize: PAGE_SIZE, currentPage: parseInt(page, 10), countries };
};
