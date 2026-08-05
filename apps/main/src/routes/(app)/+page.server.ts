import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';
import { feedPath, resolveTab, resolveSort, PAGE_SIZE } from '$lib/feed/endpoint';
import type { FeedItem } from '$lib/types';
import type { ContentArticle } from '$lib/types/content';
import type { ForumThread } from '$lib/types';
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
	const geoCountry =
		event.url.searchParams.get('geo') ??
		event.locals.user?.content_country_preference ??
		'global';
	const geoParam = geoCountry !== 'global' ? `&geo=${encodeURIComponent(geoCountry)}` : '';
	try {
		const landing = await apiGet<LandingPageData>(ctx, '/pages/homepage');
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
		// No published "homepage" page — hydrate the default seed and
		// pre-fetch entity data server-side to inject into entity blocks.
		const tokens = getGeoTokens(geoCountry, 'en');

		// Pre-fetch entity data for the seed's entity_collection blocks.
		// Each entity_type maps to its canonical source so the data matches the
		// section heading: "Trending discussions" (thread) → /forum threads,
		// NOT the mixed /explore feed (which returns stories/guides/posts).
		// "Popular destinations" (space) → /spaces (the /trending/spaces endpoint
		// is cataloged but currently returns null — a backend stub bug; /spaces
		// returns real destination data so the heading is not left empty).
		const [exploreItems, blogPosts, destinations, jobs, forumThreads] = await Promise.all([
			apiList<FeedItem>(ctx, `/explore?page_size=5&page_offset=0${geoParam}`)
				.then((r) => r.items).catch(() => [] as FeedItem[]),
			apiList<ContentArticle>(ctx, '/blog?page_size=6&page_offset=0')
				.then((r) => r.items).catch(() => [] as ContentArticle[]),
			apiList<unknown>(ctx, `/spaces?page_size=6&page_offset=0${geoParam ? `&geo=${encodeURIComponent(geoCountry)}` : ''}`)
				.then((r) => r.items).catch(() => [] as unknown[]),
			apiList<unknown>(ctx, `/jobs?page_size=6&page_offset=0${geoParam}`)
				.then((r) => r.items).catch(() => [] as unknown[]),
			apiList<ForumThread>(ctx, '/forum?page_size=5')
				.then((r) => r.items).catch(() => [] as ForumThread[]),
		]);

		const seed = landingPalettes['homepage']?.defaultSeed ?? [];
		const seedBlocks = seed.map((entry, i) => {
			const props = { ...entry.props } as Record<string, unknown>;
			// Inject pre-fetched data into entity_collection blocks.
			if (entry.type === 'entity_collection') {
				const entityType = props.entity_type as string;
				if (entityType === 'space') props._items = destinations;
				else if (entityType === 'thread') props._items = forumThreads;
				else if (entityType === 'post') props._items = exploreItems;
				else if (entityType === 'blog') props._items = blogPosts;
				else if (entityType === 'job') props._items = jobs;
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
			guestLanding: null,
			contentCountry: geoCountry,
		};
	}
};
