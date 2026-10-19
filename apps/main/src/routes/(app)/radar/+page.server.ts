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

	// Explicit invalidation dependencies: SSR fetches go through ctx.fetch
	// against the backend origin, so client-side invalidate('/api/v1/...')
	// never matches a recorded fetch URL. Declaring the keys makes the
	// follow/unfollow toggle's invalidate() calls re-run this load.
	event.depends('/api/v1/intel/radar/followed');
	event.depends('/api/v1/intel/radar');

	// 1-based page index for the followed feed (matches the rest of the app).
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const offset = (page - 1) * PAGE_SIZE;
	// Domain filter — ?item_type= per catalog; ?type= is the backend alias.
	const itemType =
		event.url.searchParams.get('item_type') ?? event.url.searchParams.get('type') ?? '';

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
	let followedTotal = 0;
	let followedHasMore = false;
	let followedNextCursor = '';
	let followedCountsByType: Record<string, number> = {};
	let followedCountsByCountry: Record<string, number> = {};
	let followedSpaceIds: string[] = [];
	let intelUnavailable = false;
	if (authed) {
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(PAGE_SIZE));
			params.set('page_offset', String(offset));
			if (itemType) params.set('item_type', itemType);
			const res = await apiRadarList<RadarItem>(ctx, `/intel/radar/followed?${params.toString()}`);
			followedItems = res.items;
			const meta = res.meta;
			// The followed feed already emits followed_space_ids in its meta —
			// reuse it for the browse-list badges instead of a second 1-row
			// fetch of /intel/radar just for the sidecar.
			followedSpaceIds = (meta?.['followed_space_ids'] as string[] | undefined) ?? [];
			followedTotal = followedSpaceIds.length;
			followedHasMore = meta?.['has_more'] === true;
			// Spec §739 sidecars: keyset cursor + per-type/per-country counts.
			followedNextCursor = (meta?.['next_cursor'] as string | undefined) ?? '';
			followedCountsByType = (meta?.['counts_by_type'] as Record<string, number> | undefined) ?? {};
			followedCountsByCountry = (meta?.['counts_by_country'] as Record<string, number> | undefined) ?? {};
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
	// `dest` carries a geo_code, slug, or country name — resolve it to the
	// country space id so the backend's space-keyed filter applies (the raw
	// `country=` param only matches exact stored text or geo_code). When no
	// country matches, fall back to the raw value normalized for casing.
	let geoItems: RadarItem[] = [];
	let geoHasMore = false;
	let geoNextCursor = '';
	let geoCountsByType: Record<string, number> = {};
	if (hasGeoFilter && !intelUnavailable) {
		try {
			const params = new URLSearchParams();
			params.set('page_size', String(PAGE_SIZE));
			params.set('page_offset', String(offset));
			if (itemType) params.set('item_type', itemType);
			const resolvedSpace = spaceId
				? null
				: (countries.find((c) => {
						const d = dest.trim().toLowerCase();
						return (
							c.geo_code.toLowerCase() === d ||
							c.slug === d ||
							c.name.toLowerCase() === d
						);
					}) ?? null);
			if (spaceId) params.set('space_id', spaceId);
			else if (resolvedSpace) params.set('space_id', resolvedSpace.id);
			else if (dest) {
				const d = dest.trim();
				params.set(
					'country',
					d.length <= 4
						? d.toUpperCase()
						: d
								.split(/[\s-]+/)
								.map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase())
								.join(' ')
				);
			}
			const res = await apiRadarList<RadarItem>(ctx, `/intel/radar?${params.toString()}`);
			geoItems = res.items;
			geoHasMore = res.meta?.['has_more'] === true;
			geoNextCursor = (res.meta?.['next_cursor'] as string | undefined) ?? '';
			geoCountsByType = (res.meta?.['counts_by_type'] as Record<string, number> | undefined) ?? {};
		} catch (err) {
			if (err instanceof ApiError && err.status === 503) {
				intelUnavailable = true;
			}
			geoItems = [];
		}
	}

	return {
		countries,
		countriesError,
		followedItems,
		followedTotal,
		followedHasMore,
		followedNextCursor,
		followedCountsByType,
		followedCountsByCountry,
		followedSpaceIds,
		intelUnavailable,
		authed,
		pageSize: PAGE_SIZE,
		currentPage: page,
		// Geo-aware filter state
		spaceId,
		dest,
		itemType,
		hasGeoFilter,
		geoItems,
		geoHasMore,
		geoNextCursor,
		geoCountsByType
	};
};
