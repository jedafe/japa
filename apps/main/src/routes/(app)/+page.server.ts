import type { PageServerLoad } from './$types';
import { apiList, apiGet, apiGetEnvelope, apiContext } from '$lib/server/api';
import { buildGuestHome, emptyGuestHome } from '$lib/server/guest-home';
import { feedQueryString, resolveTab, resolveSort, PAGE_SIZE, type FeedTab, type SortValue } from '$lib/feed/endpoint';
import type {
	FeedItem,
	GeoContext,
	GeoCountry,
} from '$lib/types';

import {
	resolveGeoCopy,
	resolveString,
	getGeoTokens,
	containsGuestBlocks,
	type LandingPage as LandingPageData,
	type Block
} from '@japaconnect/landing-blocks';

/**
 * Home route (/). Behavior splits by auth state per ADR-058 §5 Exception #3:
 *
 * - Authenticated users: For You timeline in the app shell (default tab).
 * - Guests with an assigned homepage Page Builder page (slug "homepage"):
 *   render the landing page OUTSIDE the app shell.
 * - Guests without an assigned landing: render the new default guest homepage
 *   from public settings and API-driven data (Rule 16) — NOT a redirect.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Authenticated users get the timeline in the shell.
	if (authed) {
		const tab = resolveTab(event.url.searchParams.get('tab'), authed);
		const sort = resolveSort(event.url.searchParams.get('sort'));
		const geo =
			event.url.searchParams.get('geo') ??
			event.locals.user?.content_country_preference ??
			undefined;
		try {
			const qs = feedQueryString(tab, sort, PAGE_SIZE + 1, 0, geo);
			const feedRes = await (tab === 'following'
				? apiList<FeedItem>(ctx, `/feed?${qs}`)
				: tab === 'for_you'
					? apiList<FeedItem>(ctx, `/feed/for-you?${qs}`)
					: apiList<FeedItem>(ctx, `/explore?${qs}`));
			const items = feedRes.items;
			const hasMore = items.length > PAGE_SIZE;
			return {
				items: items.slice(0, PAGE_SIZE),
				hasMore,
				tab,
				sort,
				authed,
				pageSize: PAGE_SIZE,
				loadError: false,
				landing: null,
				guestHome: false,
				hasGuestHomeBlock: false,
				home: emptyGuestHome,
				contentCountry: undefined
			};
		} catch (e) {
			console.error('[HOME LOAD ERROR]', tab, sort, e);
			return {
				items: [] as FeedItem[],
				tab,
				sort,
				authed,
				pageSize: PAGE_SIZE,
				loadError: true,
				landing: null,
				guestHome: false,
				hasGuestHomeBlock: false,
				hasMore: false,
				home: emptyGuestHome,
				contentCountry: undefined
			};
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
		const envelope = await apiGetEnvelope<LandingPageData>(ctx, '/pages/homepage');
		const landing = envelope.data;
		const literals = envelope.literals ?? {};
		const tokens = getGeoTokens(geoCountry, 'en', geoCountryName);
		const resolvedBlocks = resolveGeoCopy(landing.blocks, {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
			literals
		});
		const hasGuestHome = (landing.blocks as Block[]).some((b) => b.type === 'guest_home');
		// Guest-home context data is needed by ANY guest_* block on the page
		// (standalone blocks consume stats/categories/etc. too), not just
		// the guest_home wrapper.
		const home = containsGuestBlocks(landing.blocks as Block[])
			? await buildGuestHome(ctx, event)
			: emptyGuestHome;
		return {
			authed: false,
			landing: {
				...landing,
				title: landing.title ? resolveString(landing.title, tokens, literals) : landing.title,
				description: landing.description ? resolveString(landing.description, tokens, literals) : landing.description,
				blocks: resolvedBlocks
			},
			contentCountry: geoCountry,
			guestHome: false,
			hasGuestHomeBlock: hasGuestHome,
			items: [] as FeedItem[],
			tab: 'explore' as FeedTab,
			sort: 'top' as SortValue,
			hasMore: false,
			pageSize: PAGE_SIZE,
			loadError: false,
			home
		};
	} catch {
		// No published "homepage" page — render the new default guest homepage.
		// All marketing data is sourced from public settings (Rule 16); live
		// content (stories, destinations) comes from the backend API.
		const home = await buildGuestHome(ctx, event);

		return {
			authed: false,
			landing: null,
			guestHome: true,
			hasGuestHomeBlock: true,
			home,
			contentCountry: geoCountry,
			items: [] as FeedItem[],
			tab: 'explore' as FeedTab,
			sort: 'top' as SortValue,
			hasMore: false,
			pageSize: PAGE_SIZE,
			loadError: false
		};
	}
};
