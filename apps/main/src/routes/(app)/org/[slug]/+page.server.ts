import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList } from '$lib/server/api';
import type { PublicOrganization, OrgMember, ProfilePost, OrgMedia, ComparisonListItem } from '$lib/types';

/**
 * Public organization profile at /org/[slug] — SSR, indexable.
 *
 * Fetches the org header, members, the org's published content (Posts tab),
 * and the org's media (Media tab). The right-sidebar (members list + follow
 * button) is also resolved here for SSR. For university-type orgs, also
 * best-effort fetches existing comparisons of the 'university' type for the
 * Compare tab (Section 39).
 *
 * Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md
 *      japaconnect-engineering/sections/39-space-types-university.md (Compare tab)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let org: PublicOrganization;
	let jsonLd: unknown = null;
	try {
		const envelope = await apiGetEnvelope<PublicOrganization>(ctx, `/organizations/${slug}`);
		org = envelope.data;
		jsonLd = envelope.json_ld ?? null;
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Organization not found' : 'Failed to load organization'
		});
	}

	const [members, posts, media, isFollowing, comparisons] = await Promise.all([
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
			: Promise.resolve(false),
		// Section 39 — for university-type orgs, best-effort fetch existing
		// comparisons of the 'university' type to show on the Compare tab.
		// Degrades to [] for guests or if the endpoint is unavailable.
		org.org_type === 'university'
			? apiList<ComparisonListItem>(ctx, `/jce/comparisons?type=university&page_size=10`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([])
	]);

	// Owner = the current user appears in the member list with role 'owner'.
	const isOwner = !!event.locals.user && members.some((m) => m.user.id === event.locals.user!.id && m.role === 'owner');

	return { org, isOwner, members, posts, media, initiallyFollowing: isFollowing, jsonLd, comparisons };
};
