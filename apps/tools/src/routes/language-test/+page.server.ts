import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { loadCountries } from '$lib/server/geo';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type {
	LanguageTest,
	LanguageTestRequirementWithAi,
	GeoSpaceOption
} from '$lib/types';

/**
 * Language Test Hub — §99, ADR-088.
 * SSR loads the destination country list from GET /geo/countries AND all
 * language tests from /language-tests. When a ?dest= param is present, the
 * requirements for that destination are fetched server-side so the page
 * renders with content immediately.
 *
 * API: GET /geo/countries — the full 250 primary geo space countries (geo selector).
 *      GET /language-tests — all tests (IELTS, TOEFL, etc.).
 *      GET /language-tests/requirements?dest={space_id}&visa={category}
 *          — requirements for a destination.
 *      GET /language-tests/{test_slug} — test detail (nested data object).
 *      GET /language-tests/{test_slug}/resources — prep resources.
 *      GET /language-tests/{test_slug}/stories?dest={space_id} — score stories.
 *      POST /language-tests/{test_slug}/stories — submit a score story (auth).
 *
 * Ref: §99 (Language Test Hub), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when language_test_hub.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('language-test')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			destinations: [],
			tests: [],
			requirements: [],
			initialDestId: null,
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// Destination list — the geo-aware selector source (Rule 16).
	let destinations: GeoSpaceOption[] = [];
	try {
		destinations = await loadCountries(ctx);
	} catch {
		destinations = [];
	}

	// All language tests — fetched once on SSR (public, not destination-specific).
	let tests: LanguageTest[] = [];
	try {
		tests = (await apiList<LanguageTest>(ctx, '/language-tests')).items;
	} catch {
		tests = [];
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
		enabled: true,
		tool,
		destinations,
		tests,
		requirements,
		initialDestId,
		user: event.locals.user
	};
};
