import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiList, apiGet, apiGetEnvelope, apiContext, ApiError } from '#lib/server/api.js';
import { buildGuestHome, emptyGuestHome } from '#lib/server/guest-home.js';
import { publishedPagePath } from '#lib/server/published-page.js';
import { feedQueryString, resolveTab, resolveSort, PAGE_SIZE, type FeedTab, type SortValue } from '#lib/feed/endpoint.js';
import type {
	FeedItem,
	GeoContext,
	GeoCountry,
	RemoteTagSubscription,
	RemoteTimelineItem,
	RemoteTimelinePage,
} from '#lib/types/index.js';

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
	// ADR-127: on the roadmap host the root path is the roadmap board.
	if (event.locals.isRoadmapHost) redirect(302, '/roadmap');

	// The home "Thread" tab links out to the /thread index — it is not a
	// feed of its own. ?tab=thread is the legacy alias for that surface;
	// redirect so home never renders a second thread list. Every param
	// /thread understands survives the hop: sort (top|featured — latest is
	// the default) plus the forum filters status/tag/author_id. Feed-only
	// params (relevant|rising sorts, geo) have no meaning there and drop.
	if (event.url.searchParams.get('tab') === 'thread') {
		const params = new URLSearchParams();
		const sort = event.url.searchParams.get('sort');
		if (sort === 'top' || sort === 'featured') params.set('sort', sort);
		const status = event.url.searchParams.get('status');
		if (status === 'solved' || status === 'open') params.set('status', status);
		for (const key of ['tag', 'author_id']) {
			const v = event.url.searchParams.get(key);
			if (v) params.set(key, v);
		}
		const qs = params.toString();
		redirect(302, `/thread${qs ? `?${qs}` : ''}`);
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	// Authenticated users get the timeline in the shell.
	if (authed) {
		const tab = resolveTab(event.url.searchParams.get('tab'), authed);
		const sort = resolveSort(event.url.searchParams.get('sort'), tab);

		// §126 M5 — the Global tab is the consume-side fediverse timeline:
		// keyset-paginated remote posts, not the offset-paginated local feeds.
		// ?tag= narrows to one hashtag and also drives the live remote
		// search + subscription UI client-side.
		if (tab === 'global') {
			const globalTag = (event.url.searchParams.get('tag') ?? '').replace(/^#/, '').trim();
			// §126 tag discovery — ?domain= pins the live remote search to one
			// instance (search input accepts tag@domain syntax).
			const globalDomain = (event.url.searchParams.get('domain') ?? '').trim().toLowerCase();
			const tagQs = globalTag ? `&tag=${encodeURIComponent(globalTag)}` : '';
			const subsPromise = apiGet<{ subscriptions: RemoteTagSubscription[] }>(
				ctx, '/me/federation/tag-subscriptions'
			).then((r) => r?.subscriptions ?? []).catch(() => [] as RemoteTagSubscription[]);
			try {
				const page = await apiGet<RemoteTimelinePage>(
					ctx,
					`/me/federation/timeline?limit=${PAGE_SIZE}${tagQs}`
				);
				return {
					items: [] as FeedItem[],
					globalItems: page?.items ?? [],
					globalNextCursor: page?.next_cursor ?? null,
					globalNotFederated: false,
					globalTag,
					globalDomain,
					globalSubscriptions: await subsPromise,
					tab,
					sort,
					authed,
					pageSize: PAGE_SIZE,
					loadError: false,
					landing: null,
					guestHome: false,
					hasGuestHomeBlock: false,
					hasMore: false,
					home: emptyGuestHome,
					contentCountry: undefined
				};
			} catch (e) {
				// 404 not_federated → member hasn't opted in (or the flag is
				// off): the tab shows the enable CTA instead of an error.
				const notFederated = e instanceof ApiError && e.status === 404;
				if (!notFederated) console.error('[HOME GLOBAL FEED ERROR]', e);
				return {
					items: [] as FeedItem[],
					globalItems: [] as RemoteTimelineItem[],
					globalNextCursor: null,
					globalNotFederated: notFederated,
					globalTag,
					globalDomain,
					globalSubscriptions: [] as RemoteTagSubscription[],
					tab,
					sort,
					authed,
					pageSize: PAGE_SIZE,
					loadError: !notFederated,
					landing: null,
					guestHome: false,
					hasGuestHomeBlock: false,
					hasMore: false,
					home: emptyGuestHome,
					contentCountry: undefined
				};
			}
		}

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
	// The param is validated like the backend's explicitGeoParam: 'all' or a
	// 2-letter code overrides; an empty/malformed value is absent (preference
	// applies) rather than rendering "1A"-style tokens.
	const geoParamRaw = event.url.searchParams.get('geo')?.trim();
	const geoParam =
		geoParamRaw && (/^[a-z]{2}$/i.test(geoParamRaw) || geoParamRaw.toLowerCase() === 'all')
			? geoParamRaw
			: null;
	let geoCountry = geoParam ?? event.locals.user?.content_country_preference ?? null;
	let geoCountryName: string | undefined;

	// ?geo=all is the documented explicit-global override (ADR-064 §2) —
	// normalize it to the 'global' sentinel so getGeoTokens doesn't render
	// "all" as a literal country name.
	if (geoCountry?.toLowerCase() === 'all') geoCountry = 'global';

	// Geo must resolve before the landing fetch fires: the backend resolves
	// geo literals from the request's geo context, so a bare /pages/homepage
	// response is per-IP and unsafe to share. publishedPagePath carries the
	// resolved destination plus the visitor's detected source country, making
	// the payload cacheable per (geo, source) for anonymous callers — the api
	// layer bypasses the cache for auth/journey context it cannot key.
	const countriesPromise = apiGet<GeoCountry[]>(ctx, '/geo/countries').catch(() => null);
	const homePromise = buildGuestHome(ctx, event).catch(() => emptyGuestHome);
	// IP-cached, so repeat visitors pay ~nothing; it is also the source
	// country for the page-cache key even when ?geo= or a preference
	// overrides the destination.
	const geoMePromise = apiGet<GeoContext>(ctx, '/geo/me').catch(() => null);

	if (!geoCountry) {
		const geo = await geoMePromise;
		if (geo?.country_code) {
			geoCountry = geo.country_code;
			geoCountryName = geo.country_name || undefined;
		}
	}
	if (geoCountry && geoCountry !== 'global' && !geoCountryName) {
		// Countries fetch was already in flight — usually a cache hit anyway.
		const countries = await countriesPromise;
		const match = countries?.find((c) => c.geo_code === geoCountry!.toUpperCase());
		if (match) geoCountryName = match.name;
	}
	if (!geoCountry) geoCountry = 'global';
	const srcGeo = (await geoMePromise)?.country_code ?? '';

	const landingResult = await apiGetEnvelope<LandingPageData>(
		ctx,
		publishedPagePath('homepage', geoCountry, srcGeo)
	).then(
		(envelope) => ({ ok: true as const, envelope }),
		(err) => ({ ok: false as const, err })
	);
	if (landingResult.ok) {
		const envelope = landingResult.envelope;
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
		// the guest_home wrapper. The prefetch is already in flight — await it
		// only when the landing actually contains guest_* blocks.
		const home = containsGuestBlocks(landing.blocks as Block[])
			? await homePromise
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
	}

	// No published "homepage" page — render the new default guest homepage.
	// All marketing data is sourced from public settings (Rule 16); live
	// content (stories, destinations) comes from the backend API.
	const home = await homePromise;

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
};
