import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList } from '$lib/server/api';
import type { PublicOrganization, OrgMember, ProfilePost, OrgMedia, ComparisonListItem, JobListing, ComposerOrganization } from '$lib/types';

// Job editors = owner/admin/editor members — the same roles the
// GET/PATCH /org/{id}/jobs endpoints enforce (JOBS-012).
const JOB_EDITOR_ROLES = new Set(['owner', 'admin', 'editor']);

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
		// Public Members tab — ListPublicMembers (ACTIVE members, lean rows).
		// The full member list (invited_by) is auth + member-gated server-side.
		apiList<OrgMember>(ctx, `/organizations/${org.id}/members`).then((r) => r.items).catch(() => []),
		// page_size 60 — this feed backs the Posts tab and the public Jobs tab
		// (job_listing items filtered client-side); the default page would cap it.
		apiList<ProfilePost>(ctx, `/organizations/${org.id}/content?page_size=60`)
			.then((r) => r.items)
			.catch(() => []),
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
		// comparisons of the 'universities' type (the seeded comparison-type
		// slug is plural — 'university' 404s and silently empties the list)
		// to show on the Compare tab. Degrades to [] for guests or if the
		// endpoint is unavailable.
		org.org_type === 'university'
			? apiList<ComparisonListItem>(ctx, `/jce/comparisons?type=universities&page_size=10`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([])
	]);

	// Managers = owner or admin members (Section 07: PATCH is owner/admin).
	// They get the "Manage" button linking to the org admin dashboard.
	const canManage =
		!!event.locals.user &&
		members.some((m) => m.user.id === event.locals.user!.id && (m.role === 'owner' || m.role === 'admin'));

	// Section 63 — job editors (owner/admin/editor) get a Jobs tab listing
	// the org's listings via GET /org/{id}/jobs. The caller's membership
	// comes from /me/organizations (authoritative for member_role); the
	// public members list is display-only and may not include the viewer.
	const myMembership = event.locals.user
		? await apiList<ComposerOrganization>(ctx, '/me/organizations')
				.then((r) => r.items.find((o) => o.id === org.id) ?? null)
				.catch(() => null)
		: null;
	const canManageJobs = !!myMembership && JOB_EDITOR_ROLES.has(myMembership.member_role);
	const isJobAdmin = !!myMembership && (myMembership.member_role === 'owner' || myMembership.member_role === 'admin');
	const orgJobs = canManageJobs
		? await apiList<JobListing>(ctx, `/org/${org.id}/jobs?limit=50`)
				.then((r) => r.items)
				.catch(() => [])
		: [];

	return {
		org,
		canManage,
		canManageJobs,
		isJobAdmin,
		orgJobs,
		members,
		posts,
		media,
		initiallyFollowing: isFollowing,
		jsonLd,
		comparisons
	};
};
