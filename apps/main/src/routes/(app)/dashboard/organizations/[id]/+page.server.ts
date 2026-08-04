// Ref: japaconnect-engineering/sections/07-organizations.md (org admin dashboard)
// Loads the full org row (by ID) + members + invitations + content + analytics
// for the admin view. Auth-gated; the backend self-403s non-members. The {id}
// param is a UUID; GET /organizations/{id} is served by the slug handler's
// UUID-detection path.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { OrganizationAdmin, OrgMember, ProfilePost } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const id = event.params.id;

	let org: OrganizationAdmin | null = null;
	try {
		org = await apiGet<OrganizationAdmin>(ctx, `/organizations/${id}`);
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		throw error(status, { code, message: 'Organization not found' });
	}

	// Members + invitations + content + analytics load best-effort.
	const [membersRes, invitationsRes, contentRes, analyticsRes] = await Promise.all([
		apiList<OrgMember>(ctx, `/organizations/${id}/members`).catch(() => ({ items: [] as OrgMember[] })),
		apiList<unknown>(ctx, `/organizations/${id}/invitations`).catch(() => ({ items: [] as unknown[] })),
		apiList<ProfilePost>(ctx, `/organizations/${id}/content?page_size=20`).catch(() => ({ items: [] as ProfilePost[] })),
		apiGet<unknown>(ctx, `/organizations/${id}/analytics`).catch(() => null)
	]);

	return {
		org,
		members: membersRes.items,
		invitations: invitationsRes.items,
		content: contentRes.items,
		analytics: analyticsRes
	};
};
