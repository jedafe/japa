// Ref: japaconnect-engineering/sections/102-mentorship-matching.md
import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiGet, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	Mentor,
	GeoCountry,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension
} from '$lib/types';

/**
 * Mentor discovery at /mentors — lists mentors with optional destination and
 * areas filters and a sort (match | newest | capacity). Passes through all
 * query params to GET /mentors so the backend filters/sorts server-side.
 *
 * Feature-flagged via `mentorship.enabled` (§102): returns 404 (not 403)
 * when the feature is disabled, matching the backend features.Gate behavior.
 *
 * The "Compare in JCE" action creates a private quick comparison from
 * selected mentors, seeds the matrix with tool-native values, and redirects
 * to the canonical /compare/{slug} preview.
 */
const PAGE_SIZE = 20;
const MAX_COMPARE = 4;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['mentorship.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Mentorship is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const dest = event.url.searchParams.get('dest') ?? '';
	const areas = event.url.searchParams.get('areas') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'match';
	const page = Number(event.url.searchParams.get('page') ?? '1');

	const filters = { dest, areas, sort, page };

	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	if (page > 1) params.set('page', String(page));
	if (dest) params.set('dest', dest);
	if (areas) params.set('areas', areas);
	if (sort) params.set('sort', sort);

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	try {
		const { items, meta } = await apiList<Mentor>(ctx, `/mentors?${params.toString()}`);
		return { mentors: items, meta, authed, pageSize: PAGE_SIZE, filters, countries, loadError: false };
	} catch {
		return {
			mentors: [] as Mentor[],
			meta: undefined,
			authed,
			pageSize: PAGE_SIZE,
			filters,
			countries,
			loadError: true
		};
	}
};

/**
 * Build a private quick JCE comparison from 2-4 selected mentors.
 * Seeds the matrix with tool-native fields (areas, capacity, active mentees,
 * available slots, bio) so the /compare/{slug} preview is useful immediately.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to save a comparison' });
		}

		const form = await event.request.formData();
		const mentorIds = form
			.getAll('mentor_id')
			.filter((v): v is string => typeof v === 'string');

		if (mentorIds.length < 2 || mentorIds.length > MAX_COMPARE) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 mentors' });
		}

		const ctx = apiContext(event);

		// Fetch the selected mentors in the order the user picked them.
		let mentors: Mentor[];
		try {
			mentors = await apiGet<Mentor[]>(
				ctx,
				`/compare/mentors?ids=${mentorIds.map(encodeURIComponent).join(',')}`
			);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load mentors';
			return fail(500, { error: 'fetch_failed', message });
		}

		const mentorById = new Map(mentors.map((m) => [m.user_id, m]));
		const orderedMentors = mentorIds
			.map((id) => mentorById.get(id))
			.filter((m): m is Mentor => !!m);

		if (orderedMentors.length !== mentorIds.length) {
			return fail(500, {
				error: 'mentor_not_found',
				message: 'One or more selected mentors could not be resolved'
			});
		}

		const items = orderedMentors.map((m, i) => ({
			title: m.username,
			description: m.mentor_bio ?? '',
			position: i,
			item_ref: { type: 'mentor', id: m.user_id }
		}));

		const dimensionLabels = ['Areas', 'Capacity', 'Active mentees', 'Available slots', 'Bio'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Mentor comparison',
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

		const itemByTitle = new Map<string, ComparisonItem>();
		for (const item of jce.items) {
			itemByTitle.set(item.title, item);
		}
		const dimByLabel = new Map(jce.dimensions.map((d) => [d.label, d]));

		const fieldMap: Record<string, (m: Mentor) => string> = {
			Areas: (m) => (m.mentor_areas ?? []).join(', '),
			Capacity: (m) => (m.mentor_capacity == null ? '' : String(m.mentor_capacity)),
			'Active mentees': (m) =>
				m.active_mentee_count == null ? '' : String(m.active_mentee_count),
			'Available slots': (m) => String(m.available_slots ?? ''),
			Bio: (m) => m.mentor_bio ?? ''
		};

		for (const [label, getter] of Object.entries(fieldMap)) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = orderedMentors
				.map((mentor) => {
					const item = itemByTitle.get(mentor.username);
					if (!item) return null;
					const value = getter(mentor);
					return { item_id: item.id, value };
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
