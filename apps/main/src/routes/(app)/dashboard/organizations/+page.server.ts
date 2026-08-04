// Ref: japaconnect-engineering/sections/07-organizations.md (my organizations)
// Lists the orgs the current user is a member of. Auth-gated; the backend
// returns the org row + the caller's member_role.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { OrganizationAdmin } from '$lib/types';

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
	return { organizations };
};
