import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem } from '$lib/types';
import { resolveGeoCopy, getGeoTokens, landingPalettes, type Block, type LandingPage as LandingPageData } from '@japaconnect/landing-blocks';

/**
 * Home route (/). Behavior splits by auth state per ADR-058 §5 Exception #3:
 *
 * - Authenticated users: For You timeline in the app shell (default tab).
 * - Guests with an assigned homepage Page Builder page (slug "homepage"):
 *   render the landing page OUTSIDE the app shell.
 * - Guests without an assigned landing: render the default GuestLanding
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
			return { items, tab, sort, authed, pageSize: PAGE_SIZE, loadError: false, landing: null, guestLanding: null };
		} catch (e) {
			console.error('[HOME LOAD ERROR]', tab, sort, e);
			return { items: [] as FeedItem[], tab, sort, authed, pageSize: PAGE_SIZE, loadError: true, landing: null, guestLanding: null };
		}
	}

	// Guests: check for an assigned homepage Page Builder landing page.
	// Convention: the landing page has slug "homepage" (ADR-058 §5).
	try {
		const landing = await apiGet<LandingPageData>(ctx, '/pages/homepage');
		// Resolve geo-aware copy server-side (ADR-064 — no client flicker).
		const geoCountry =
			event.url.searchParams.get('geo') ??
			event.locals.user?.content_country_preference ??
			'global';
		const tokens = getGeoTokens(geoCountry, 'en');
		const resolvedBlocks = resolveGeoCopy(landing.blocks as Block[], {
			contentCountry: geoCountry,
			tokens,
			locale: 'en',
		});
		return {
			authed: false,
			landing: { ...landing, blocks: resolvedBlocks },
			guestLanding: null,
			contentCountry: geoCountry,
		};
	} catch (err) {
		// No published "homepage" page — hydrate the default seed from the
		// homepage palette so the new LandingRenderer shows immediately.
		// Admins can later publish a custom page (slug "homepage") to override.
		const geoCountry =
			event.url.searchParams.get('geo') ??
			event.locals.user?.content_country_preference ??
			'global';
		const tokens = getGeoTokens(geoCountry, 'en');
		const seed = landingPalettes['homepage']?.defaultSeed ?? [];
		const seedBlocks = seed.map((entry, i) => ({
			id: `seed-${i}`,
			type: entry.type,
			visible: true,
			label: entry.label,
			props: entry.props as unknown as Block['props'],
		})) as Block[];
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
			guestLanding: null,
			contentCountry: geoCountry,
		};
	}
};
