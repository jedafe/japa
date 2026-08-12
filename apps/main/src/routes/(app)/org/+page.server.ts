import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { OrgMeta, PublicOrganization } from '$lib/types';

/**
 * Organizations index at /org — public, SSR, indexable. Lists every
 * tenant organization with member/follower counts and verification state.
 * Each card links to the org profile at /org/[slug]. Search filters by
 * name or slug. org_type and verified filters are optional.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      ADR-058 (renders inside the app shell)
 */
const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const q = (event.url.searchParams.get('q') ?? '').trim();
	const orgType = (event.url.searchParams.get('org_type') ?? '').trim();
	const verified = (event.url.searchParams.get('verified') ?? '').trim();
	const pageParam = parseInt(event.url.searchParams.get('page') ?? '0', 10);
	const page = isNaN(pageParam) || pageParam < 0 ? 0 : pageParam;
	const offset = page * PAGE_SIZE;

	let orgs: PublicOrganization[] = [];
	let total = 0;
	let totalPages = 0;
	let loadError = false;
	let forbidden = false;

	// BUG-022: Fetch org type options from the API (Rule 16).
	let orgMeta: OrgMeta | null = null;
	try {
		orgMeta = await apiGet<OrgMeta>(ctx, '/organizations/meta');
	} catch {
		// Non-fatal — the page falls back to an empty filter.
	}

	try {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		params.set('page_offset', String(offset));
		if (q) params.set('q', q);
		if (orgType) params.set('org_type', orgType);
		if (verified) params.set('verified', verified);

		const result = await apiList<PublicOrganization>(ctx, `/organizations?${params.toString()}`);
		orgs = result.items;
		total = result.meta?.total ?? 0;
		totalPages = result.meta?.total_pages ?? 0;
	} catch (err) {
		if (err instanceof ApiError && err.status === 403) {
			forbidden = true;
		} else {
			loadError = true;
		}
	}

	return { orgs, q, orgType, verified, page, totalPages, total, pageSize: PAGE_SIZE, forbidden, loadError, orgMeta };
};
