import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type {
	SuggestedUser,
	SuggestedSpace,
	SuggestedTag,
	SuggestedContent,
	PublicOrgListItem,
	University
} from '$lib/types';

/**
 * Discover at /discover — auth-gated frontend route (ADR-058). Consumes the
 * existing /suggestions/* and /organizations + /universities endpoints; no
 * new backend route is added for Discover.
 *
 * Tabs: Spaces · People · Organizations · Tags · Content · Universities. The
 * active tab is selected via ?tab=; unknown values coerce to "spaces" so the
 * tab bar and the content pane can never disagree. Each tab's data degrades
 * to an empty list on failure (a failing endpoint must never blank Discover).
 */
const PAGE_SIZE = 24;
const VALID_TABS = new Set(['spaces', 'people', 'organizations', 'tags', 'content', 'universities']);

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
	let universities: University[] = [];
	let orgsTotal = 0;
	let unisTotal = 0;
	let followedSpaceIds: string[] = [];
	let followedOrgIds: string[] = [];

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
				const res = await apiList<PublicOrgListItem>(
					ctx,
					`/organizations?page_size=${PAGE_SIZE}`
				);
				organizations = res.items;
				orgsTotal = res.meta?.total ?? res.items.length;
				const followed = await apiList<{ entity_id: string }>(
					ctx,
					'/me/following?type=organization&page_size=100'
				);
				followedOrgIds = followed.items.map((f) => f.entity_id);
				break;
			}
			case 'tags':
				tags = (await apiList<SuggestedTag>(ctx, `/suggestions/tags?limit=${PAGE_SIZE}`)).items;
				break;
			case 'content':
				content = (
					await apiList<SuggestedContent>(ctx, `/suggestions/content?limit=${PAGE_SIZE}`)
				).items;
				break;
			case 'universities': {
				// GET /universities paginates with ?page=&limit= — it does NOT read
				// page_size (the param is ignored and the default limit applies).
				const res = await apiList<University>(
					ctx,
					`/universities?page=1&limit=${PAGE_SIZE}`
				);
				universities = res.items;
				unisTotal = res.meta?.total ?? res.items.length;
				break;
			}
			default: {
				spaces = (await apiList<SuggestedSpace>(ctx, `/suggestions/spaces?limit=${PAGE_SIZE}`)).items;
				const followed = await apiList<{ entity_id: string }>(
					ctx,
					'/me/following?type=space&page_size=100'
				);
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
		universities,
		orgsTotal,
		unisTotal,
		followedSpaceIds,
		followedOrgIds,
		pageSize: PAGE_SIZE
	};
};
