import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { PublicOrganization } from '$lib/types';

/**
 * Organizations index at /organizations — public, SSR, indexable. Lists every
 * tenant organization with member/follower counts and verification state. Each
 * card links to the org profile at /org/[slug].
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 48;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();

	let orgs: PublicOrganization[] = [];
	let loadError = false;
	try {
		const path = q
			? `/organizations?q=${encodeURIComponent(q)}&page_size=${PAGE_SIZE}`
			: `/organizations?page_size=${PAGE_SIZE}`;
		orgs = (await apiList<PublicOrganization>(ctx, path)).items;
	} catch {
		loadError = true;
	}

	return { orgs, q, pageSize: PAGE_SIZE, loadError };
};
