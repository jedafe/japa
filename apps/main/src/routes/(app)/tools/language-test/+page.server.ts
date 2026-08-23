// Ref: japaconnect-engineering/sections/99-language-test-hub.md
// Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, apiPut, ApiError } from '$lib/server/api';
import type {
	LanguageTest,
	LanguageTestHubPage,
	LanguageTestRequirementWithAi,
	JCEComparison,
	ComparisonCreated,
	ComparisonItem,
	ComparisonDimension,
	GeoCountry
} from '$lib/types';

/**
 * Language Test Hub — §99, ADR-088.
 * SSR loads the full 250 primary geo space countries from GET /geo/countries AND all
 * language tests from /language-tests. When a ?dest= param is present, the
 * requirements for that destination are fetched server-side so the page
 * renders with content immediately.
 *
 * The "Compare in JCE" action creates a private quick comparison from
 * selected language tests, seeds the matrix with tool-native values, and
 * redirects to the canonical /compare/{slug} preview.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /language-tests — all tests (IELTS, TOEFL, etc.).
 *      GET /language-tests/requirements?dest={space_id}&visa={category}
 *          — requirements for a destination.
 *      GET /language-tests/{test_slug} — test detail (nested data object).
 *      GET /language-tests/{test_slug}/resources — prep resources.
 *      GET /language-tests/{test_slug}/stories?dest={space_id} — score stories.
 *      POST /language-tests/{test_slug}/stories — submit a score story (auth).
 *      POST /jce/comparisons — JCE quick comparison creation.
 *      PUT /jce/comparisons/dimensions/{id}/values — seed cell values.
 *
 * Route: /tools/language-test
 * Ref: §99 (Language Test Hub), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['language_test_hub.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Language Test Hub is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or language_test_hub_disabled code means the tool is turned off.
	let destinations: GeoCountry[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch (e) {
		if (e instanceof ApiError && (e.status === 404 || e.code === 'language_test_hub_disabled')) {
			disabled = true;
		}
		destinations = [];
	}

	// All language tests — fetched once on SSR (public, not destination-specific).
	let tests: LanguageTest[] = [];
	if (!disabled) {
		try {
			tests = (await apiList<LanguageTest>(ctx, '/language-tests')).items;
		} catch {
			tests = [];
		}
	}

	// If ?dest= is present, fetch requirements for that destination.
	const destParam = event.url.searchParams.get('dest');
	const initialDestId =
		destParam && destinations.some((d) => d.id === destParam) ? destParam : null;

	let requirements: LanguageTestRequirementWithAi[] = [];
	if (initialDestId) {
		try {
			requirements = (
				await apiList<LanguageTestRequirementWithAi>(
					ctx,
					`/language-tests/requirements?dest=${initialDestId}`
				)
			).items;
		} catch {
			requirements = [];
		}
	}

	return {
		destinations,
		tests,
		requirements,
		initialDestId,
		disabled,
		authed
	};
};

/**
 * Build a private quick JCE comparison from 2-4 selected language tests.
 * The matrix is pre-seeded with tool-native fields (validity, official URL,
 * minimum score, and visa category when a destination is selected) so the
 * /compare/{slug} preview is immediately useful.
 */
export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(401, { error: 'unauthenticated', message: 'Sign in to compare' });
		}

		const form = await event.request.formData();
		const testSlugs = form
			.getAll('test_slug')
			.filter((v): v is string => typeof v === 'string');
		const destId = form.get('dest_id');

		if (testSlugs.length < 2 || testSlugs.length > 4) {
			return fail(400, { error: 'invalid_selection', message: 'Select 2 to 4 language tests' });
		}

		const ctx = apiContext(event);

		// Fetch the selected tests in the order the user picked them.
		let pages: LanguageTestHubPage[] = [];
		try {
			pages = await Promise.all(
				testSlugs.map((slug) =>
					apiGet<LanguageTestHubPage>(ctx, `/language-tests/${encodeURIComponent(slug)}`)
				)
			);
		} catch (e) {
			const message = e instanceof ApiError ? e.message : 'Could not load language tests';
			return fail(500, { error: 'fetch_failed', message });
		}

		const tests = pages.map((p) => p.test).filter((t): t is LanguageTest => !!t?.slug);

		if (tests.length !== testSlugs.length) {
			return fail(500, { error: 'test_not_found', message: 'One or more selected tests could not be resolved' });
		}

		// If a destination was supplied, load its requirements to seed
		// destination-specific dimension values.
		let requirements: LanguageTestRequirementWithAi[] = [];
		if (destId && typeof destId === 'string') {
			try {
				requirements = (
					await apiList<LanguageTestRequirementWithAi>(
						ctx,
						`/language-tests/requirements?dest=${encodeURIComponent(destId)}`
					)
				).items;
			} catch {
				requirements = [];
			}
		}

		const reqBySlug = new Map<string, LanguageTestRequirementWithAi>();
		for (const req of requirements) {
			if (!reqBySlug.has(req.test_slug)) {
				reqBySlug.set(req.test_slug, req);
			}
		}

		const items = tests.map((t, i) => ({
			title: t.name,
			description: '',
			position: i,
			item_ref: { type: 'languagetest', slug: t.slug }
		}));

		const dimensionLabels = ['Validity', 'Official URL', 'Minimum score', 'Visa category'];

		let created: ComparisonCreated;
		try {
			created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', {
				title: 'Language test comparison',
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
		const dimByLabel = new Map(jce.dimensions.map((d: ComparisonDimension) => [d.label, d]));

		const fieldMap: Record<string, (test: LanguageTest) => string> = {
			Validity: (test) => (test.validity_months ? `${test.validity_months} months` : '—'),
			'Official URL': (test) => (test.official_url ?? '—'),
			'Minimum score': (test) => (reqBySlug.get(test.slug ?? '')?.minimum_score ?? '—'),
			'Visa category': (test) => (reqBySlug.get(test.slug ?? '')?.visa_category ?? '—')
		};

		for (const [label, getter] of Object.entries(fieldMap)) {
			const dim = dimByLabel.get(label);
			if (!dim) continue;

			const values = tests
				.map((test) => {
					const item = itemByTitle.get(test.name);
					if (!item) return null;
					return { item_id: item.id, value: getter(test) };
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
