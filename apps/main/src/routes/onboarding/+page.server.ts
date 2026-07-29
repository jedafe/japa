import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { OnboardingStatus, TagSuggestion, OnboardingRecommendation, GeoContext, GeoCountry } from '$lib/types';

/**
 * Onboarding wizard (Section 09). Five-step post-login flow that collects
 * core identity, destination, interests, and recommended connections.
 * Redirects to / if already completed; to /login if unauthenticated.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	if (!event.locals.user) {
		throw redirect(302, '/login');
	}

	// Fetch onboarding status — redirect to / if already completed.
	let status: OnboardingStatus | null = null;
	try {
		status = await apiGet<OnboardingStatus>(ctx, '/onboarding/status');
	} catch {
		// If the endpoint fails, let the user through to the wizard.
	}
	if (status?.completed) {
		throw redirect(302, '/');
	}

	// Preload tags for step 3 (interests) — Rule 16: from API, not hardcoded.
	let tags: TagSuggestion[] = [];
	try {
		tags = (await apiList<TagSuggestion>(ctx, '/suggestions/tags?limit=30')).items;
	} catch {
		tags = [];
	}

	// Preload recommendations for step 4 (spaces & connections).
	let recommendations: OnboardingRecommendation[] = [];
	try {
		recommendations = (await apiGet<OnboardingRecommendation[]>(ctx, '/onboarding/recommendations')) ?? [];
	} catch {
		recommendations = [];
	}

	// Geo detection (ADR-037): pre-fill the Step 1 "Current location" field
	// from the user's GeoIP context so they don't have to type it manually.
	// Best-effort — degrades to null when the GeoIP DB is unavailable.
	let geoContext: GeoContext | null = null;
	try {
		geoContext = await apiGet<GeoContext>(ctx, '/geo/me');
	} catch {
		geoContext = null;
	}

	// Preload countries for the Step 1 location picker (ADR-005 geo spaces).
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	return {
		user: event.locals.user,
		status,
		tags,
		recommendations,
		geoContext,
		countries
	};
};
