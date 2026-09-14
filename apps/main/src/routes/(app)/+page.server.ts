import type { PageServerLoad } from './$types';
import { apiList, apiGet, apiGetEnvelope, apiContext } from '$lib/server/api';
import { feedQueryString, resolveTab, resolveSort, PAGE_SIZE, type FeedTab, type SortValue } from '$lib/feed/endpoint';
import type {
	FeedItem,
	GeoContext,
	GeoCountry,
	GuestHomeData,
	GuestHomeOpportunity,
	GuestHomeStory,
	GuestHomeTool,
	GuestHomeStat,
	GuestHomeCategory,
	SpaceSuggestion,
	Story
} from '$lib/types';

import {
	resolveGeoCopy,
	resolveString,
	getGeoTokens,
	type LandingPage as LandingPageData
} from '@japaconnect/landing-blocks';

const emptyGuestHome: GuestHomeData = {
	topDestinations: [],
	opportunities: [],
	stories: [],
	tools: [],
	stats: [],
	categories: []
};

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
			items: [] as FeedItem[],
			tab: 'explore' as FeedTab,
			sort: 'top' as SortValue,
			hasMore: false,
			pageSize: PAGE_SIZE,
			loadError: false,
			home: emptyGuestHome
		};
	} catch {
		// No published "homepage" page — render the new default guest homepage.
		// All marketing data is sourced from public settings (Rule 16); live
		// content (stories, destinations) comes from the backend API.
		const parent = await event.parent();
		const siteSettings = (parent?.siteSettings as Record<string, unknown> | undefined) ?? {};

		const stats = asArray<GuestHomeStat>(siteSettings['landing.stats']);
		const categories = asArray<GuestHomeCategory>(siteSettings['landing.categories']);
		const opportunities = asArray<GuestHomeOpportunity>(siteSettings['landing.featured_opportunities']);
		const tools = asArray<GuestHomeTool>(siteSettings['landing.tools']);
		let topDestinations = asArray<SpaceSuggestion>(siteSettings['landing.top_destinations']);

		// If no curated top destinations are configured, try the directory API.
		if (topDestinations.length === 0) {
			try {
				const spaceRes = await apiList<SpaceSuggestion>(
					ctx,
					`/spaces?level=country&sort=members&page_size=4`
				);
				topDestinations = spaceRes.items.slice(0, 4);
			} catch (err) {
				console.error('[HOME] top destinations load failed', err);
			}
		}

		let stories: GuestHomeStory[] = [];
		try {
			const storyRes = await apiList<Story>(ctx, '/stories?limit=3&sort=top');
			stories = storyRes.items.slice(0, 3).map(mapStoryToGuestHome);
		} catch (err) {
			console.error('[HOME] stories load failed', err);
		}

		const home: GuestHomeData = {
			topDestinations,
			opportunities,
			stories,
			tools,
			stats,
			categories
		};

		return {
			authed: false,
			landing: null,
			guestHome: true,
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

function asArray<T>(value: unknown): T[] {
	if (Array.isArray(value)) return value as T[];
	if (value && typeof value === 'object' && !Array.isArray(value)) {
		const maybe = (value as Record<string, unknown>).data;
		if (Array.isArray(maybe)) return maybe as T[];
	}
	return [];
}

function mapStoryToGuestHome(story: Story): GuestHomeStory {
	const quote =
		story.best_advice?.trim() ||
		story.most_useful_resource?.trim() ||
		(story.body ? story.body.replace(/<[^>]+>/g, '').slice(0, 160) : story.title);
	const role = [story.profession, story.destination_space_name].filter(Boolean).join(' • ');

	return {
		id: story.id,
		slug: story.slug,
		title: story.title,
		quote,
		author_name: story.author_display_name ?? story.author_name ?? 'A JapaConnect Member',
		author_role: role || undefined,
		author_avatar_url: story.author_avatar ?? story.featured_image_url ?? null,
		destination: story.destination_space_name
	};
}
