import { api } from '$lib/api/client';
import type { VisitorContextSummary } from '$lib/types';

export interface JourneyOption {
	value: string;
	label: string;
}

export interface JourneyOptions {
	purposes: JourneyOption[];
	stages: JourneyOption[];
	visa_routes: JourneyOption[];
}

export interface JourneyContextInput {
	origin_country: string;
	destination_country: string;
	japa_stage: string;
	visa_route: string;
	purpose: string;
	travel_date: string;
}

export const getFeedContext = () => api.get<VisitorContextSummary>('/feed/context');

export const getJourneyOptions = () => api.get<JourneyOptions>('/feed/journey-options');

// ---------------------------------------------------------------------------
// Shared client-side caches. JourneyContextBar and JourneyOnboardingModal are
// mounted together on feed pages and each fetched /feed/context,
// /feed/journey-options and /geo/countries independently — the memoized
// getters below collapse those duplicate requests to one in-flight promise
// per resource per session. Module state only lives in the browser (all call
// sites run inside $effect), so there is no cross-request leakage.
// ---------------------------------------------------------------------------

let feedContextPromise: ReturnType<typeof getFeedContext> | null = null;
let journeyOptionsPromise: ReturnType<typeof getJourneyOptions> | null = null;

export function getFeedContextCached() {
	feedContextPromise ??= getFeedContext();
	return feedContextPromise;
}

export function getJourneyOptionsCached() {
	journeyOptionsPromise ??= getJourneyOptions();
	return journeyOptionsPromise;
}

/** Drop the cached context so the next read re-fetches (e.g. after a save). */
export function invalidateJourneyCaches() {
	feedContextPromise = null;
}

let geoCountriesPromise: ReturnType<typeof api.get<import('$lib/types').GeoCountry[]>> | null = null;

/** Memoized GET /geo/countries — one fetch per session across all consumers. */
export function getGeoCountriesCached() {
	geoCountriesPromise ??= api.get<import('$lib/types').GeoCountry[]>('/geo/countries');
	return geoCountriesPromise;
}

// The guest journey token is delivered via the jc_journey_token Set-Cookie
// header and a top-level envelope sidecar — it is not part of `data`.
export const setFeedContext = (input: Partial<JourneyContextInput>) => {
	invalidateJourneyCaches();
	return api.post<VisitorContextSummary>('/feed/context', input);
};
