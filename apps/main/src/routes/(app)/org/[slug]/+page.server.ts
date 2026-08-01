import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { PublicOrganization, OrgMember, ProfilePost, OrgMedia } from '$lib/types';

/**
 * Public organization profile at /org/[slug] — SSR, indexable.
 *
 * Fetches the org header, members, the org's published content (Posts tab),
 * and the org's media (Media tab). The right-sidebar (members list + follow
 * button) is also resolved here for SSR.
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let org: PublicOrganization;
	try {
		org = await apiGet<PublicOrganization>(ctx, `/organizations/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Organization not found' : 'Failed to load organization'
		});
	}

	const [members, posts, media, isFollowing] = await Promise.all([
		apiList<OrgMember>(ctx, `/organizations/${org.id}/members`).then((r) => r.items).catch(() => []),
		apiList<ProfilePost>(ctx, `/organizations/${org.id}/content`).then((r) => r.items).catch(() => []),
		apiList<OrgMedia>(ctx, `/organizations/${org.id}/media`).then((r) => r.items).catch(() => []),
		// Hydrate the follow button's initial state. Best-effort: a failure
		// degrades to `false` (not following). Only fetch when the viewer is
		// authenticated — guests can't follow.
		event.locals.user
			? apiGet<{ following: boolean }>(ctx, `/users/${org.id}/follow-status?entity_type=organization`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false)
	]);

	// Owner = the current user appears in the member list with role 'owner'.
	const isOwner = !!event.locals.user && members.some((m) => m.user.id === event.locals.user!.id && m.role === 'owner');

	return { org, isOwner, members, posts, media, initiallyFollowing: isFollowing };
};
