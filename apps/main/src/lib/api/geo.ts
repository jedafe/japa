import { api } from '$lib/api/client';
import { listSpaceTypes, createSpace } from '$lib/api/create';
import type {
	GeoContext,
	GeoDetect,
	GeoCountry,
	GeoState,
	GeoCity,
	ContentCountryPreference,
	Space
} from '$lib/types';

/**
 * Geo API client (ADR-037 / ADR-064 / ADR-064a). Wraps the /geo/*,
 * /me/preferences/content-country, /me/location, and /recommendations
 * endpoints. All functions degrade gracefully — returning null on failure
 * so callers can fall back to free-text or the system default.
 */

/** GET /geo/me — GeoIP context for the current request IP. */
export async function getGeoMe(): Promise<GeoContext | null> {
	const res = await api.get<GeoContext>('/geo/me');
	return res.data ?? null;
}

/** GET /geo/detect — { country, state, city } projection for locale detection. */
export async function getGeoDetect(): Promise<GeoDetect | null> {
	const res = await api.get<GeoDetect>('/geo/detect');
	return res.data ?? null;
}

/** GET /geo/countries — all seeded country-level geo spaces. */
export async function listCountries(): Promise<GeoCountry[]> {
	const res = await api.get<GeoCountry[]>('/geo/countries');
	return res.data ?? [];
}

/** GET /geo/countries/{code}/states — states/provinces for a country. */
export async function listStates(countryCode: string): Promise<GeoState[]> {
	const res = await api.get<GeoState[]>(`/geo/countries/${countryCode}/states`);
	return res.data ?? [];
}

/** GET /geo/states/{id}/cities — cities for a state. */
export async function listCities(stateId: string): Promise<GeoCity[]> {
	const res = await api.get<GeoCity[]>(`/geo/states/${stateId}/cities`);
	return res.data ?? [];
}

function slugify(name: string): string {
	return name
		.toLowerCase()
		.trim()
		.replace(/\s+/g, '-')
		.replace(/[^\w-]+/g, '')
		.replace(/--+/g, '-')
		.replace(/^-|-$/g, '');
}

/**
 * POST /spaces — create a new city-level geo space under the given state.
 * Used by the location settings picker when a city is not yet seeded.
 * Returns the created city or null on failure.
 */
export async function createCity(stateId: string, name: string): Promise<GeoCity | null> {
	const typesRes = await listSpaceTypes();
	const cityType = typesRes.data?.find(
		(t) => t.slug === 'city' || t.name.toLowerCase() === 'city'
	);
	if (!cityType) {
		return null;
	}

	const res = await createSpace({
		name: name.trim(),
		slug: slugify(name),
		space_type_id: cityType.id,
		level: 'city',
		parent_id: stateId,
		parent_space_ids: [stateId]
	});
	if (!res.data) {
		return null;
	}
	const space = res.data;
	return spaceToCity(space);
}

function spaceToCity(space: Space): GeoCity {
	return {
		id: space.id,
		name: space.name,
		slug: space.slug,
		geo_code: space.geo_code,
		level: 'city',
		parent_id: space.parent_id ?? '',
		currency_code: null,
		member_count: space.member_count
	};
}

/**
 * PATCH /me/preferences/content-country — set the user's content country
 * override. Empty string clears the preference (falls back to GeoIP).
 * The backend invalidates the VisitorContext cache on success.
 */
export async function setContentCountry(country: string): Promise<ContentCountryPreference | null> {
	const res = await api.patch<ContentCountryPreference>('/me/preferences/content-country', {
		country
	});
	return res.data ?? null;
}

/**
 * POST /me/location — set the user's current geo space from browser
 * geolocation coordinates or a direct space_id selection (ADR-064a).
 * The backend reverse-geocodes lat/lng via Nominatim and matches to a
 * seeded city-level Space, falling back to country-level. Invalidates
 * the VisitorContext cache on success.
 */
export async function setLocation(
	coords: { latitude: number; longitude: number } | { space_id: string }
): Promise<{ status: string; space_id: string } | null> {
	const res = await api.post<{ status: string; space_id: string }>('/me/location', coords);
	return res.data ?? null;
}

/** GET /me/location — returns the user's current geo space and resolved location. */
export async function getMyLocation(): Promise<{
	space_id: string | null;
	name: string | null;
	city: string | null;
	city_id: string | null;
	state: string | null;
	state_id: string | null;
	country: string | null;
	country_code: string | null;
} | null> {
	const res = await api.get<{
		space_id: string | null;
		name: string | null;
		city: string | null;
		city_id: string | null;
		state: string | null;
		state_id: string | null;
		country: string | null;
		country_code: string | null;
	}>('/me/location');
	return res.data ?? null;
}

/**
 * GET /recommendations — geo-aware tool/feature recommendations based on
 * the VisitorContext (ADR-064a). Works for both guests and authenticated
 * users. Returns ranked tools, features, and a context summary.
 */
export async function getRecommendations(): Promise<RecommendationResponse | null> {
	const res = await api.get<RecommendationResponse>('/recommendations');
	return res.data ?? null;
}

export interface RecommendationItem {
	slug: string;
	label: string;
	url: string;
	reason: string;
}

/** A recommended content item sourced from trending content (ADR-064a §5). */
export interface RecommendationContentItem {
	id: string;
	title: string;
	url: string;
	content_type: string;
}

/** A recommended space sourced from trending spaces (ADR-064a §5). */
export interface RecommendationSpaceItem {
	id: string;
	name: string;
	slug: string;
	level: string;
}

export interface RecommendationResponse {
	tools: RecommendationItem[];
	features: RecommendationItem[];
	content: RecommendationContentItem[];
	spaces: RecommendationSpaceItem[];
	context: {
		archetype: string;
		content_country: string;
		origin_country: string;
		current_country: string;
		destination_country: string;
		japa_stage: string;
		current_city: string;
	};
}
