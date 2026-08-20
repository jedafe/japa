import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiRadarList, ApiError } from '$lib/server/api';
import type { GeoCountry, RadarItem } from '$lib/types';

/**
 * Japa Radar at /radar — the country-grouped intelligence feed (Phase 6,
 * Task 25). SSR-fetches the caller's followed-countries feed plus the full
 * 250 primary geo-space countries from GET /api/v1/geo/countries so the page
 * renders without a client round-trip and is indexable. Followed-countries
 * feed is auth-gated (401 for guests → empty); the country browse list is public.
 *
 * Ref: japaconnect-engineering — Japa Intelligence System spec §7
 *      ADR-005 / ADR-058 (renders inside the unified app shell)
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// 1-based page index for the followed feed (matches the rest of the app).
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const offset = (page - 1) * PAGE_SIZE;

	// Geo-aware filter: ?space_id= or ?dest= (country code). When present,
	// the radar feed is filtered to that country/space instead of showing
	// the followed-countries feed. This matches the geo-aware pattern used
	// by other surfaces (§31 spaces, §32 feed).
	const spaceId = event.url.searchParams.get('space_id') ?? '';
	const dest = event.url.searchParams.get('dest') ?? '';
	const hasGeoFilter = spaceId !== '' || dest !== '';

	// Country list — public. Consumes the canonical /geo/countries endpoint
	// (ADR-005), which returns all 250 primary country geo spaces.
	let countries: GeoCountry[] = [];
	let countriesError = false;
	try {
		const res = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = res.items;
	} catch {
		countriesError = true;
	}

	// Followed-countries feed — auth-gated. Guests get an empty feed; the
	// page renders the browse list only. Best-effort: any backend error
	// degrades to an empty list rather than failing the whole page.
	let followedItems: RadarItem[] = [];
	let followedCount = 0;
	let followedTotal = 0;
	let intelUnavailable = false;
	if (authed) {
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(PAGE_SIZE));
			params.set('page_offset', String(offset));
			const res = await apiRadarList<RadarItem>(ctx, `/intel/radar/followed?${params.toString()}`);
			followedItems = res.items;
			const meta = res.meta;
			const countsByType = meta?.['counts_by_type'] as Record<string, number> | undefined;
			followedCount = countsByType
				? Object.values(countsByType).reduce((a, b) => a + b, 0)
				: followedItems.length;
			followedTotal = (meta?.['followed_space_ids'] as string[] | undefined)?.length ?? 0;
		} catch (err) {
			if (err instanceof ApiError && err.status === 503) {
				intelUnavailable = true;
			}
			// 401 (token revoked mid-session) or any other error → empty feed.
			followedItems = [];
		}
	}

	// Geo-filtered feed — when ?space_id= or ?dest= is present, fetch the
	// radar feed filtered to that country/space. This is public (no auth
	// required) so guests can browse a specific country's opportunities.
	let geoItems: RadarItem[] = [];
	let geoCount = 0;
	if (hasGeoFilter && !intelUnavailable) {
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(PAGE_SIZE));
			params.set('page_offset', String(offset));
			if (spaceId) params.set('space_id', spaceId);
			else if (dest) params.set('country', dest);
			const res = await apiRadarList<RadarItem>(ctx, `/intel/radar?${params.toString()}`);
			geoItems = res.items;
			const meta = res.meta;
			const countsByType = meta?.['counts_by_type'] as Record<string, number> | undefined;
			geoCount = countsByType
				? Object.values(countsByType).reduce((a, b) => a + b, 0)
				: geoItems.length;
		} catch (err) {
			if (err instanceof ApiError && err.status === 503) {
				intelUnavailable = true;
			}
			geoItems = [];
		}
	}

	// Resolve the caller's followed space IDs so the browse list can badge
	// followed countries. The public radar feed emits `followed_space_ids` in
	// its meta; we fetch a single-row page just to read that sidecar. Best-
	// effort: any error degrades to an empty set (the page falls back to an
	// optimistic toggle for the follow button state).
	let followedSpaceIds: string[] = [];
	if (authed && !intelUnavailable) {
		try {
			const res = await apiRadarList<RadarItem>(ctx, `/intel/radar?page_size=1&page_offset=0`);
			followedSpaceIds = (res.meta?.['followed_space_ids'] as string[] | undefined) ?? [];
		} catch {
			followedSpaceIds = [];
		}
	}

	return {
		countries,
		countriesError,
		followedItems,
		followedCount,
		followedTotal,
		followedSpaceIds,
		intelUnavailable,
		authed,
		pageSize: PAGE_SIZE,
		currentPage: page,
		// Geo-aware filter state
		spaceId,
		dest,
		hasGeoFilter,
		geoItems,
		geoCount
	};
};
