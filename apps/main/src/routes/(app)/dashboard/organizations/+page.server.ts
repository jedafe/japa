// Ref: japaconnect-engineering/sections/07-organizations.md (my organizations)
// Lists the orgs the current user is a member of. Auth-gated; the backend
// returns the org row + the caller's member_role.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, OrganizationAdmin, OrgMeta } from '$lib/types';

/** GET /me/organizations row — org + member_role. */
export interface MyOrganizationRow extends OrganizationAdmin {
	member_role: string;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const organizations = (await apiList<MyOrganizationRow>(ctx, '/me/organizations')).items;

	// BUG-022: Fetch org type options from the API (Rule 16).
	let orgMeta: OrgMeta | null = null;
	try {
		orgMeta = await apiGet<OrgMeta>(ctx, '/organizations/meta');
	} catch {
		// Non-fatal — selects are disabled when meta is unavailable (Rule 16).
	}

	// Country list sourced from the spaces country table (/geo/countries)
	// so the country_code picker is never hardcoded (Rule 16).
	let countries: GeoCountry[] = [];
	try {
		countries = (await apiList<GeoCountry>(ctx, '/geo/countries')).items;
	} catch {
		countries = [];
	}

	return { organizations, orgMeta, countries };
};
