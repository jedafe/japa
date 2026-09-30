import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiGet, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	JobListing,
	JobDetail,
	GeoCountry,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';

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
	const remote = event.url.searchParams.get('remote') ?? event.url.searchParams.get('location_type');
	const visaSponsorship = event.url.searchParams.get('visa_sponsorship');
	const sort = event.url.searchParams.get('sort');
	const page = event.url.searchParams.get('page') ?? '1';
	if (q) params.set('q', q);
	if (country) params.set('country', country);
	if (jobType) params.set('job_type', jobType);
	if (remote) params.set('remote', remote);
	if (visaSponsorship) params.set('visa_sponsorship', visaSponsorship);
	if (sort) params.set('sort', sort);
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

/**
 * Build a private quick JCE comparison from 2-4 selected job listings.
 * The matrix is pre-seeded with job custom fields so the /compare/{slug}
 * preview is immediately useful.
 *
 * Ref: japaconnect-engineering/sections/63-jobs-board.md (Integration)
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const jobSlugs = form
			.getAll('job_slug')
			.filter((v): v is string => typeof v === 'string');

		if (jobSlugs.length < 2 || jobSlugs.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 jobs' });
		}

		const ctx = apiContext(event);

		let jobs: JobDetail[];
		try {
			jobs = await Promise.all(
				jobSlugs.map((slug) => apiGet<JobDetail>(ctx, `/jobs/${encodeURIComponent(slug)}`))
			);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load job details';
			return fail(500, { error: 'fetch_failed', message });
		}

		const items = jobs.map((j, i) => ({
			title: j.title,
			description: j.summary ?? '',
			position: i,
			item_ref: { type: 'job_listing', id: j.id, slug: j.slug }
		}));

		const dimensionLabels = [
			'Company',
			'Job Type',
			'Location Type',
			'Country',
			'City',
			'Salary Min',
			'Salary Max',
			'Salary Currency',
			'Experience Level',
			'Application Deadline',
			'Visa Sponsorship',
			'Relocation Assistance'
		];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Job comparison',
				depth: 'quick',
				body: '',
				items,
				dimensions: dimensionLabels
			});
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not create comparison';
			return fail(500, { error: 'create_failed', message });
		}

		let jce: JCEComparison;
		try {
			jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${created.id}`);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load created comparison';
			return fail(500, { error: 'detail_failed', message });
		}

		// §58 — managed currency list so the "Salary Currency" dimension shows
		// "USD — US Dollar" labels instead of bare ISO codes.
		let currencyNameByCode = new Map<string, string>();
		try {
			const currencies = await apiGet<{ code: string; name?: string }[]>(ctx, '/currencies');
			currencyNameByCode = new Map(
				(currencies ?? []).map((c) => [c.code.toUpperCase(), c.name || c.code])
			);
		} catch {
			// degrade to raw codes
		}

		const itemByPosition = new Map<number, ComparisonItem>();
		for (const item of jce.items) {
			itemByPosition.set(item.position, item);
		}
		const dimByLabel = new Map<string, ComparisonDimension>(
			jce.dimensions.map((d) => [d.label, d])
		);

		function getValue(j: JobDetail, label: string): string {
			const v = j.field_values ?? {};
			switch (label) {
				case 'Company':
					return v.company_name ?? '';
				case 'Job Type':
					return v.job_type ?? '';
				case 'Location Type':
					return v.location_type ?? '';
				case 'Country':
					return v.country ?? '';
				case 'City':
					return v.city ?? '';
				case 'Salary Min':
					return v.salary_min ?? '';
				case 'Salary Max':
					return v.salary_max ?? '';
				case 'Salary Currency': {
					const code = v.salary_currency ?? '';
					if (!code) return '';
					const name = currencyNameByCode.get(code.toUpperCase());
					return name && name !== code ? `${code} — ${name}` : code;
				}
				case 'Experience Level':
					return v.experience_level ?? '';
				case 'Application Deadline':
					return v.application_deadline ?? '';
				case 'Visa Sponsorship':
					return v.visa_sponsorship ?? '';
				case 'Relocation Assistance':
					return v.relocation_assistance ?? '';
				default:
					return '';
			}
		}

		for (const label of dimensionLabels) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = jobs
				.map((j, i) => {
					const item = itemByPosition.get(i);
					if (!item) return null;
					return { item_id: item.id, value: getValue(j, label) };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
