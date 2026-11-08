import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiRadarList } from '#lib/server/api.js';
import type {
	SuggestedUser,
	SuggestedSpace,
	SuggestedTag,
	SuggestedContent,
	PublicOrgListItem
} from '#lib/types/index.js';
import type { Source } from '#lib/types/sources.js';
import type { RadarItem } from '#lib/types/index.js';

/**
 * Discover at /discover — auth-gated frontend route (ADR-058). Consumes the
 * existing /suggestions/* and /organizations endpoints; no
 * new backend route is added for Discover.
 *
 * Tabs: Spaces · People · Organizations · Tags · Content · Sources ·
 * Opportunities. The active tab is selected via ?tab=; unknown values
 * coerce to "spaces" so the tab bar and the content pane can never
 * disagree. Each tab's data degrades to an empty list on failure (a
 * failing endpoint must never blank Discover).
 */
const PAGE_SIZE = 24;
const VALID_TABS = new Set(['spaces', 'people', 'organizations', 'tags', 'content', 'sources', 'opportunities']);

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/discover');
	}

	const ctx = apiContext(event);
	const rawTab = event.url.searchParams.get('tab') ?? 'spaces';
	const tab = VALID_TABS.has(rawTab) ? rawTab : 'spaces';

	let spaces: SuggestedSpace[] = [];
	let people: SuggestedUser[] = [];
	let organizations: PublicOrgListItem[] = [];
	let tags: SuggestedTag[] = [];
	let content: SuggestedContent[] = [];
	let sources: Source[] = [];
	let orgsTotal = 0;
	let followedSpaceIds: string[] = [];
	let followedOrgIds: string[] = [];
	let followedSourceIds: string[] = [];
	let opportunities: RadarItem[] = [];

	// Follow endpoints are toggles, so the follow/join buttons need the
	// caller's current follow state — a wrong initial state would turn a
	// "Follow" click into an unfollow. The suggestion queries exclude
	// already-followed users/tags but NOT spaces (member-only exclusion) or
	// organizations (cached public list), so the followed IDs for those two
	// types are fetched via GET /me/following?type=… and intersected in the
	// page.
	try {
		switch (tab) {
			case 'people':
				people = (await apiList<SuggestedUser>(ctx, `/suggestions/users?limit=${PAGE_SIZE}`)).items;
				break;
			case 'organizations': {
				const [res, followed] = await Promise.all([
					apiList<PublicOrgListItem>(ctx, `/organizations?page_size=${PAGE_SIZE}`),
					apiList<{ entity_id: string }>(ctx, '/me/following?type=organization&page_size=100')
				]);
				organizations = res.items;
				orgsTotal = res.meta?.total ?? res.items.length;
				followedOrgIds = followed.items.map((f) => f.entity_id);
				break;
			}
			case 'tags':
				tags = (await apiList<SuggestedTag>(ctx, `/suggestions/tags?limit=${PAGE_SIZE}`)).items;
				break;
			case 'content':
				// 7d window, not the 24h default — a discovery surface should
				// not blank out on days with no freshly published content.
				content = (
					await apiList<SuggestedContent>(ctx, `/suggestions/content?window=7d&limit=${PAGE_SIZE}`)
				).items;
				break;
			case 'sources': {
				const [res, followed] = await Promise.all([
					apiList<Source>(ctx, `/sources?sort=popular&page_size=${PAGE_SIZE}&page_offset=0`),
					apiList<{ entity_id: string }>(ctx, '/me/following?type=source&page_size=100')
				]);
				sources = res.items;
				followedSourceIds = followed.items.map((f) => f.entity_id);
				break;
			}
			case 'opportunities': {
				// Followed-countries feed (/intel/radar/followed), not the
				// global /intel/radar — Discover surfaces opportunities for the
				// spaces the caller follows, matching the dashboard widget.
				// Both endpoints emit the §739 radar envelope {items, ...},
				// not {data: [...]} — apiList would silently read an empty list.
				opportunities = (
					await apiRadarList<RadarItem>(ctx, `/intel/radar/followed?page_size=${PAGE_SIZE}`)
				).items;
				break;
			}
			default: {
				const [res, followed] = await Promise.all([
					apiList<SuggestedSpace>(ctx, `/suggestions/spaces?limit=${PAGE_SIZE}`),
					apiList<{ entity_id: string }>(ctx, '/me/following?type=space&page_size=100')
				]);
				spaces = res.items;
				followedSpaceIds = followed.items.map((f) => f.entity_id);
				break;
			}
		}
	} catch {
		// degrades to empty — the page renders an EmptyState.
	}

	return {
		tab,
		spaces,
		people,
		organizations,
		tags,
		content,
		sources,
		opportunities,
		orgsTotal,
		followedSpaceIds,
		followedOrgIds,
		followedSourceIds,
		pageSize: PAGE_SIZE
	};
};
