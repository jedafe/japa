import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	Scholarship,
	SavedScholarship,
	GeoCountry,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';
import { defaultReminderDays } from '$lib/scholarships/defaultReminder';

/**
 * Scholarship Finder at /scholarships (Section 73, ADR-073).
 * SSR-fetches the scholarship list with query filters so the page is
 * indexable and renders without a client round-trip. For authenticated
 * users, also fetches saved scholarships with their application status
 * so the page can render the application tracker (Phase 33.04).
 *
 * The "Compare in JCE" action creates a private quick comparison from
 * selected scholarships, seeds the matrix with tool-native values, and
 * redirects to the canonical /compare/{slug} preview.
 *
 * Ref: japaconnect-engineering/sections/73-scholarship-finder.md
 */
const PAGE_SIZE = 20;
const MAX_COMPARE = 4;

export interface SavedScholarshipView {
	id: string;
	guide_id: string;
	title: string;
	slug: string;
	summary?: string;
	status: string;
	notes?: string | null;
	remind_before_days?: number | null;
	updated_at?: string;
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['scholarship_finder.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Scholarship Finder is not available' });
	}

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
	const minAmount = event.url.searchParams.get('min_amount');
	const maxAmount = event.url.searchParams.get('max_amount');
	const deadlineFrom = event.url.searchParams.get('deadline_from');
	const deadlineTo = event.url.searchParams.get('deadline_to');
	const includeExpired = event.url.searchParams.get('include_expired');
	if (q) params.set('q', q);
	if (country) params.set('country', country);
	if (level) params.set('level', level);
	if (sort) params.set('sort', sort);
	if (renewable) params.set('renewable', renewable);
	if (minAmount) params.set('min_amount', minAmount);
	if (maxAmount) params.set('max_amount', maxAmount);
	if (deadlineFrom) params.set('deadline_from', deadlineFrom);
	if (deadlineTo) params.set('deadline_to', deadlineTo);
	if (includeExpired === 'true' || includeExpired === '1') params.set('include_expired', 'true');
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

	// Canonical country list for the filter dropdown (ADR-005).
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	// Rule 16: canonical saved-scholarship status options from the API.
	let statuses: string[] = [];
	try {
		const list = await apiGet<{ value: string; label: string }[]>(ctx, '/scholarships/statuses');
		statuses = (list ?? []).map((o) => o.value);
	} catch {
		statuses = [];
	}

	// Rule 16: distinct degree levels across all published scholarships —
	// the level filter's option list must not be limited to the current page.
	let levels: string[] = [];
	try {
		const list = await apiGet<{ value: string; label: string }[]>(ctx, '/scholarships/levels');
		levels = (list ?? []).map((o) => o.value);
	} catch {
		levels = [];
	}

	return {
		scholarships,
		disabled,
		authed,
		pageSize: PAGE_SIZE,
		currentPage: page,
		total,
		savedScholarships,
		savedIds,
		countries,
		statuses,
		levels,
		defaultReminderDays: defaultReminderDays(event.locals.siteSettings)
	};
};

/**
 * Build a private quick JCE comparison from 2-4 selected scholarships.
 * Seeds the matrix with tool-native fields (amount, deadline, eligibility,
 * coverage, country, institution, level, field of study) so the
 * /compare/{slug} preview is useful immediately.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const guideIds = form
			.getAll('guide_id')
			.filter((v): v is string => typeof v === 'string');

		if (guideIds.length < 2 || guideIds.length > MAX_COMPARE) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 scholarships' });
		}

		const ctx = apiContext(event);

		// Fetch the selected scholarships in the order the user picked them.
		let scholarships: Scholarship[];
		try {
			scholarships = await Promise.all(
				guideIds.map((id) => apiGet<Scholarship>(ctx, `/scholarships/${encodeURIComponent(id)}`))
			);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load scholarships';
			return fail(500, { error: 'fetch_failed', message });
		}

		const scholarshipById = new Map(scholarships.map((s) => [s.guide_id, s]));
		const orderedScholarships = guideIds
			.map((id) => scholarshipById.get(id))
			.filter((s): s is Scholarship => !!s);

		if (orderedScholarships.length !== guideIds.length) {
			return fail(500, { error: 'scholarship_not_found', message: 'One or more selected scholarships could not be found' });
		}

		const items = orderedScholarships.map((s, i) => ({
			title: s.title,
			description: s.summary ?? '',
			position: i,
			item_ref: { type: 'scholarship', id: s.guide_id }
		}));

		const dimensionLabels = [
			'Amount',
			'Deadline',
			'Eligibility',
			'Coverage',
			'Country',
			'Institution',
			'Level',
			'Field of Study'
		];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Scholarship comparison',
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

		const itemByPosition = new Map<number, ComparisonItem>();
		for (const item of jce.items) {
			itemByPosition.set(item.position, item);
		}
		const dimByLabel = new Map(jce.dimensions.map((d) => [d.label, d]));

		function getValue(s: Scholarship, label: string): string {
			switch (label) {
				case 'Amount': {
					const raw = s.award_amount_usd ?? s.field_values?.award_amount_usd;
					if (raw == null || raw === '') return '';
					const n = typeof raw === 'number' ? raw : parseFloat(String(raw));
					if (Number.isNaN(n)) return String(raw);
					return new Intl.NumberFormat('en-US', {
						style: 'currency',
						currency: 'USD',
						maximumFractionDigits: 0
					}).format(n);
				}
				case 'Deadline':
					return s.application_deadline ?? s.field_values?.application_deadline ?? '';
				case 'Eligibility':
					return s.eligibility ?? '';
				case 'Coverage': {
					if (s.is_renewable != null) {
						return s.is_renewable ? 'Renewable' : 'One-time';
					}
					const raw = s.field_values?.coverage ?? s.field_values?.is_renewable;
					if (raw == null) return '';
					if (raw.toLowerCase() === 'true') return 'Renewable';
					if (raw.toLowerCase() === 'false') return 'One-time';
					return raw;
				}
				case 'Country':
					return s.country ?? s.field_values?.countries_eligible ?? '';
				case 'Institution':
					return s.institution ?? s.field_values?.provider_institution ?? '';
				case 'Level':
					return s.level_of_study ?? s.field_values?.level_of_study ?? '';
				case 'Field of Study':
					return s.field_values?.field_of_study ?? s.field_values?.fields_of_study ?? '';
				default:
					return '';
			}
		}

		for (const label of dimensionLabels) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orderedScholarships
				.map((s, i) => {
					const item = itemByPosition.get(i);
					if (!item) return null;
					return { item_id: item.id, value: getValue(s, label) };
				})
				.filter((v): v is { item_id: string; value: string } => v !== null);

			if (values.length === 0) continue;

			try {
				await apiPut<unknown>(ctx, `/jce/comparisons/dimensions/${dim.id}/values`, { values });
			} catch (e) {
				// Seed failures are non-fatal; the user can still see the
				// created comparison and fill values in the composer.
				const message = e instanceof ApiError ? e.message : String(e);
				console.error('failed to seed dimension values:', message);
			}
		}

		throw redirect(303, `/compare/${created.slug}`);
	}
};
