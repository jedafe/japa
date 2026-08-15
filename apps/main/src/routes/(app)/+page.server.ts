import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet, API_BASE } from '$lib/server/api';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem, GeoContext, GeoCountry } from '$lib/types';
import { resolveGeoCopy, getGeoTokens, landingPalettes, fetchEntityCollection, type Block, type LandingPage as LandingPageData, type EntityType, type FetchContext } from '@japaconnect/landing-blocks';
/**
 * Home route (/). Behavior splits by auth state per ADR-058 §5 Exception #3:
 *
 * - Authenticated users: For You timeline in the app shell (default tab).
 * - Guests with an assigned homepage Page Builder page (slug "homepage"):
 *   render the landing page OUTSIDE the app shell.
 * - Guests without an assigned landing: render the default seed landing
 *   (Hashnode-style marketing page) using API data — NOT a redirect.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	// Authenticated users get the timeline in the shell.
	if (authed) {
		const tab = resolveTab(event.url.searchParams.get('tab'), authed);
		const sort = resolveSort(event.url.searchParams.get('sort'));
		// Forward the user's content_country_preference as a geo re-weight
		// (ADR-064). The ?geo= query param (if present) overrides the preference.
		const geo =
			event.url.searchParams.get('geo') ??
			event.locals.user?.content_country_preference ??
			undefined;
		try {
			const { items } = await apiList<FeedItem>(ctx, feedPath(tab, sort, PAGE_SIZE, 0, geo));
			return { items, tab, sort, authed, pageSize: PAGE_SIZE, loadError: false, landing: null };
		} catch (e) {
			console.error('[HOME LOAD ERROR]', tab, sort, e);
			return { items: [] as FeedItem[], tab, sort, authed, pageSize: PAGE_SIZE, loadError: true, landing: null };
		}
	}
	// Guests: check for an assigned homepage Page Builder landing page.
	// Convention: the landing page has slug "homepage" (ADR-058 §5).
	// Geo resolution priority: ?geo= param → GeoIP detection → 'global'.
	let geoCountry =
		event.url.searchParams.get('geo') ??
		event.locals.user?.content_country_preference ??
		null;
	let geoCountryName: string | undefined;
	if (!geoCountry) {
		// Use GeoIP to detect the visitor's country for geo-aware copy
		// (ADR-064). The backend resolves the IP from X-Forwarded-For
		// (forwarded by apiContext) via the MaxMind/DB-IP database.
		try {
			const geo = await apiGet<GeoContext>(ctx, '/geo/me');
			if (geo?.country_code) {
				geoCountry = geo.country_code;
				geoCountryName = geo.country_name || undefined;
			}
		} catch {
			// GeoIP not available (no database, localhost, etc.) — fall back.
		}
	}
	// When geo was set via ?geo= param (not GeoIP), resolve the country
	// name from the backend's /geo/countries endpoint so any of the 250
	// seeded countries gets its real name in the copy tokens — not just
	// the curated ones in the geoCopy registry (Rule 16: no hardcoded data).
	if (geoCountry && geoCountry !== 'global' && !geoCountryName) {
		try {
			const countries = await apiGet<GeoCountry[]>(ctx, '/geo/countries');
			const match = countries.find((c) => c.geo_code === geoCountry!.toUpperCase());
			if (match) geoCountryName = match.name;
		} catch {
			// Countries endpoint unavailable — getGeoTokens will use the
			// country code as a fallback name.
		}
	}
	if (!geoCountry) geoCountry = 'global';
	try {
		const landing = await apiGet<LandingPageData>(ctx, '/pages/homepage');
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);
		const resolvedBlocks = resolveGeoCopy(landing.blocks as Block[], {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
		});
		return {
			authed: false,
			landing: { ...landing, blocks: resolvedBlocks },
			contentCountry: geoCountry,
		};
	} catch (err) {
		// No published "homepage" page — hydrate the default seed and
		// pre-fetch entity data server-side to inject into entity blocks.
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);

		const seed = landingPalettes['homepage']?.defaultSeed ?? [];

		// Data-driven pre-fetch: scan the seed for entity_collection blocks,
		// collect the unique entity_types, and fetch each via the shared
		// fetchEntityCollection helper from @japaconnect/landing-blocks.
		// This replaces the previous hardcoded 5-type Promise.all and
		// automatically supports any entity type the seed declares.
		const fetchCtx: FetchContext = {
			fetch: ctx.fetch,
			apiBase: API_BASE,
			token: ctx.token,
			host: ctx.host,
		};

		const entityTypes = new Set<EntityType>();
		for (const entry of seed) {
			if (entry.type === 'entity_collection') {
				const et = (entry.props as Record<string, unknown>).entity_type as EntityType;
				if (et) entityTypes.add(et);
			}
		}

		// Fetch all unique entity types in parallel. Each fetch handles its
		// own errors (logs + returns []) so one failing type doesn't break
		// the others.
		const entityData = new Map<EntityType, unknown[]>();
		await Promise.all(
			Array.from(entityTypes).map(async (et) => {
				const items = await fetchEntityCollection(
					et,
					{ limit: 6, geo: true },
					fetchCtx,
					geoCountry !== 'global' ? geoCountry : undefined,
				);
				entityData.set(et, items);
			}),
		);

		const seedBlocks = seed.map((entry, i) => {
			const props = { ...entry.props } as Record<string, unknown>;
			// Inject pre-fetched data into entity_collection blocks.
			if (entry.type === 'entity_collection') {
				const entityType = props.entity_type as EntityType;
				if (entityType && entityData.has(entityType)) {
					props._items = entityData.get(entityType);
				}
			}
			return {
				id: `seed-${i}`,
				type: entry.type,
				visible: true,
				label: entry.label,
				props: props as unknown as Block['props'],
				motion: entry.motion,
			};
		}) as Block[];

		const resolvedBlocks = resolveGeoCopy(seedBlocks, {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
		});
		return {
			authed: false,
			landing: {
				id: 'seed-homepage',
				title: 'JapaConnect — The Nigerian Diaspora Community',
				slug: 'homepage',
				page_type: 'landing',
				status: 'seed',
				blocks: resolvedBlocks,
			} as LandingPageData,
			contentCountry: geoCountry,
		};
	}
};
