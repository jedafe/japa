import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { sanitizePostLoginTarget } from '$lib/server/auth';
import type { OnboardingStatus, TagSuggestion, OnboardingRecommendation, GeoContext, GeoCountry, Achievement, OnboardingOptions } from '$lib/types';
import type { Source } from '$lib/types/sources';

/**
 * Onboarding wizard (Section 09). Seven-step post-login flow that collects
 * core identity, destination, interests, recommended connections, and
 * followed sources.
 * Redirects to / if already completed; to /login if unauthenticated.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	if (!event.locals.user) {
		throw redirect(302, '/login');
	}

	// §120: a ?redirect= target carries the post-onboarding destination (the
	// MCP OAuth authorize URL for agent connect flows). Sanitized with the
	// same allowlist as post-login redirects — never an open redirect.
	const postOnboardingRedirect = sanitizePostLoginTarget(event.url.searchParams.get('redirect'));

	// Fetch onboarding status — redirect to / if already completed.
	let status: OnboardingStatus | null = null;
	try {
		status = await apiGet<OnboardingStatus>(ctx, '/onboarding/status');
	} catch {
		// If the endpoint fails, let the user through to the wizard.
	}
	if (status?.completed) {
		throw redirect(302, postOnboardingRedirect);
	}

	// Preload tags for step 3 (interests) — Rule 16: from API, not hardcoded.
	// NOTE: /suggestions/tags is intentionally NOT used here — it only
	// returns tags attached to the caller's own published content, which is
	// always empty for a new user. The public tag directory is the correct
	// source for topic selection.
	const tagsPromise = apiList<TagSuggestion>(ctx, '/tags?sort=popular&page_size=30')
		.then((r) => r.items)
		.catch(() => [] as TagSuggestion[]);

	// Preload recommendations for steps 4 (users) and 5 (spaces).
	const recommendationsPromise = apiGet<OnboardingRecommendation[]>(ctx, '/onboarding/recommendations')
		.then((r) => r ?? [])
		.catch(() => [] as OnboardingRecommendation[]);

	// Preload popular sources for step 6 — the public directory, not the
	// recommendation engine (Rule 16: from API, not hardcoded).
	const sourcesPromise = apiList<Source>(ctx, '/sources?sort=popular&page_size=12')
		.then((r) => r.items)
		.catch(() => [] as Source[]);

	// Geo detection (ADR-037): pre-fill the Step 1 "Current location" field
	// from the user's GeoIP context so they don't have to type it manually.
	// Best-effort — degrades to null when the GeoIP DB is unavailable.
	const geoContextPromise = apiGet<GeoContext>(ctx, '/geo/me').catch(() => null);

	// Preload countries for the Step 1 location picker (ADR-005 geo spaces).
	const countriesPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then((r) => r.items)
		.catch(() => [] as GeoCountry[]);

	// Preload onboarding option lists (japa status, primary goal) — Rule 16:
	// from API, not hardcoded in the component.
	const onboardingOptionsPromise = apiGet<OnboardingOptions>(ctx, '/onboarding/options').catch(
		() => null
	);

	// §89 Community Achievement Feed — preload latest achievements for the
	// final-step social-proof preview. Best-effort: the feature may be disabled
	// (returns 404), in which case the preview section is hidden.
	const achievementsPromise = apiList<Achievement>(ctx, '/achievements?sort=latest&limit=4')
		.then((r) => r.items)
		.catch(() => [] as Achievement[]);

	// All seven preloads are independent — run them concurrently so the
	// wizard pays one round-trip, not seven.
	const [tags, recommendations, sources, geoContext, countries, onboardingOptions, achievements] =
		await Promise.all([
			tagsPromise,
			recommendationsPromise,
			sourcesPromise,
			geoContextPromise,
			countriesPromise,
			onboardingOptionsPromise,
			achievementsPromise
		]);

	return {
		user: event.locals.user,
		status,
		postOnboardingRedirect,
		tags,
		recommendations,
		sources,
		geoContext,
		countries,
		onboardingOptions,
		achievements
	};
};
