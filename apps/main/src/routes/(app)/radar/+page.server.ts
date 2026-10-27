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

	let countriesError = false;
	let intelUnavailable = false;

	// Geo-filtered feed — when ?space_id= or ?dest= is present, fetch the
	// radar feed filtered to that country/space. This is public (no auth
	// required) so guests can browse a specific country's opportunities.
	// `dest` carries a geo_code, slug, or country name — resolve it to the
	// country space id so the backend's space-keyed filter applies (the raw
	// `country=` param only matches exact stored text or geo_code). When no
	// country matches, fall back to the raw value normalized for casing.
	const fetchGeoFeed = (countryList: GeoCountry[]) =>
		apiRadarList<RadarItem>(
			ctx,
			`/intel/radar?${(() => {
				const params = new URLSearchParams();
				params.set('page_size', String(PAGE_SIZE));
				params.set('page_offset', String(offset));
				if (itemType) params.set('item_type', itemType);
				const resolvedSpace = spaceId
					? null
					: (countryList.find((c) => {
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
				return params.toString();
			})()}`
		).catch((err) => {
			if (err instanceof ApiError && err.status === 503) {
				intelUnavailable = true;
			}
			return null;
		});

	// Wave 1 — countries, followed feed (auth), and the geo feed when
	// ?space_id= is given (no country resolution needed). A ?dest=-only
	// filter still waits on the country list to resolve the space id.
	const [countries, followedRes, geoRes] = await Promise.all([
		apiList<GeoCountry>(ctx, '/geo/countries')
			.then((r) => r.items)
			.catch(() => {
				countriesError = true;
				return [] as GeoCountry[];
			}),
		// Followed-countries feed — auth-gated. Guests get an empty feed; the
		// page renders the browse list only. Best-effort: any backend error
		// degrades to an empty list rather than failing the whole page.
		authed
			? apiRadarList<RadarItem>(
					ctx,
					`/intel/radar/followed?${(() => {
						const params = new URLSearchParams();
						params.set('page_size', String(PAGE_SIZE));
						params.set('page_offset', String(offset));
						if (itemType) params.set('item_type', itemType);
						return params.toString();
					})()}`
				).catch((err) => {
					if (err instanceof ApiError && err.status === 503) {
						intelUnavailable = true;
					}
					// 401 (token revoked mid-session) or any other error → empty feed.
					return null;
				})
			: Promise.resolve(null),
		hasGeoFilter && spaceId ? fetchGeoFeed([]) : Promise.resolve(undefined)
	]);

	// Followed-countries feed — auth-gated. Guests get an empty feed; the
	// page renders the browse list only. Best-effort: any backend error
	// degrades to an empty list rather than failing the whole page.
	const followedMeta = followedRes?.meta;
	const followedItems = followedRes?.items ?? [];
	// The followed feed already emits followed_space_ids in its meta —
	// reuse it for the browse-list badges instead of a second 1-row
	// fetch of /intel/radar just for the sidecar.
	const followedSpaceIds = (followedMeta?.['followed_space_ids'] as string[] | undefined) ?? [];
	const followedHasMore = followedMeta?.['has_more'] === true;
	// Spec §739 sidecars: keyset cursor + per-type/per-country counts.
	const followedNextCursor = (followedMeta?.['next_cursor'] as string | undefined) ?? '';
	const followedCountsByType = (followedMeta?.['counts_by_type'] as Record<string, number> | undefined) ?? {};
	const followedCountsByCountry = (followedMeta?.['counts_by_country'] as Record<string, number> | undefined) ?? {};

	// ?dest=-only filter — resolve the country space id now that the list
	// has landed (genuine dependency), unless intel already reported 503.
	const geoFeed =
		geoRes !== undefined
			? geoRes
			: hasGeoFilter && !intelUnavailable
				? await fetchGeoFeed(countries)
				: null;

	const geoItems = geoFeed?.items ?? [];
	const geoHasMore = geoFeed?.meta?.['has_more'] === true;
	const geoNextCursor = (geoFeed?.meta?.['next_cursor'] as string | undefined) ?? '';
	const geoCountsByType = (geoFeed?.meta?.['counts_by_type'] as Record<string, number> | undefined) ?? {};

	return {
		countries,
		countriesError,
		followedItems,
		followedHasMore,
		followedNextCursor,
		followedCountsByType,
		followedCountsByCountry,
		followedSpaceIds,
		intelUnavailable,
		authed,
		pageSize: PAGE_SIZE,
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
