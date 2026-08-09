import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type {
	LanguageTest,
	LanguageTestRequirementWithAi,
	GeoSpaceOption
} from '$lib/types';

/**
 * Language Test Hub — §99, ADR-088.
 * SSR loads the destination country list from /spaces?level=country AND all
 * language tests from /language-tests. When a ?dest= param is present, the
 * requirements for that destination are fetched server-side so the page
 * renders with content immediately.
 *
 * API: GET /spaces?level=country — destination countries (geo selector).
 *      GET /language-tests — all tests (IELTS, TOEFL, etc.).
 *      GET /language-tests/requirements?dest={space_id}&visa={category}
 *          — requirements for a destination.
 *      GET /language-tests/{test_slug} — test detail (nested data object).
 *      GET /language-tests/{test_slug}/resources — prep resources.
 *      GET /language-tests/{test_slug}/stories?dest={space_id} — score stories.
 *      POST /language-tests/{test_slug}/stories — submit a score story (auth).
 *
 * Route: /tools/language-test
 * Ref: §99 (Language Test Hub), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Destination list — the geo-aware selector source (Rule 16).
	// A 404 or language_test_hub_disabled code means the tool is turned off.
	let destinations: GeoSpaceOption[] = [];
	let disabled = false;
	try {
		destinations = (await apiList<GeoSpaceOption>(ctx, '/spaces?level=country')).items;
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
