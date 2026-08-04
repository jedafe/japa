import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type {
	SuggestedUser,
	SuggestedSpace,
	SuggestedTag,
	PublicOrgListItem,
	University
} from '$lib/types';

/**
 * Discover at /discover — auth-gated frontend route (ADR-058). Consumes the
 * existing /suggestions/* and /organizations + /universities endpoints; no
 * new backend route is added for Discover.
 *
 * Tabs: Spaces · People · Organizations · Tags · Universities. The active tab
 * is selected via ?tab= and defaults to "spaces". Each tab's data degrades to
 * an empty list on failure (a failing endpoint must never blank Discover).
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/discover');
	}

	const ctx = apiContext(event);
	const tab = event.url.searchParams.get('tab') ?? 'spaces';

	// Only fetch the active tab's data server-side; the others lazy-load
	// client-side via the api client to keep the SSR payload lean.
	let spaces: SuggestedSpace[] = [];
	let people: SuggestedUser[] = [];
	let organizations: PublicOrgListItem[] = [];
	let tags: SuggestedTag[] = [];
	let universities: University[] = [];

	try {
		switch (tab) {
			case 'people':
				people = (await apiList<SuggestedUser>(ctx, `/suggestions/users?limit=${PAGE_SIZE}`)).items;
				break;
			case 'organizations':
				organizations = (
					await apiList<PublicOrgListItem>(ctx, `/organizations?page_size=${PAGE_SIZE}`)
				).items;
				break;
			case 'tags':
				tags = (await apiList<SuggestedTag>(ctx, `/suggestions/tags?limit=${PAGE_SIZE}`)).items;
				break;
			case 'universities':
				universities = (await apiList<University>(ctx, `/universities?page_size=${PAGE_SIZE}`)).items;
				break;
			default:
				spaces = (await apiList<SuggestedSpace>(ctx, `/suggestions/spaces?limit=${PAGE_SIZE}`)).items;
				break;
		}
	} catch {
		// degrades to empty — the page renders an EmptyState.
	}

	return { tab, spaces, people, organizations, tags, universities, pageSize: PAGE_SIZE };
};
