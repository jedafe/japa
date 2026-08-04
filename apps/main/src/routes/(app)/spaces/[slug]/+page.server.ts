import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	SpaceDetail,
	Space,
	SpaceMember,
	SpaceModerator,
	SpaceContentItem,
	SpacePermissionPreset,
	ForumThread
} from '$lib/types';

/**
 * Space hub at /spaces/[slug] — SSR, indexable. Center column swaps between
 * Feed / Forum / Members / About tabs; the right sidebar shows the space
 * card (member/follower counts, join + follow actions, rules).
 *
 * Ref: japaconnect-engineering/sections/22-spaces.md
 *      ADR-058 (Space: Feed/Forum/Members/About tabs inside the shell)
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Space detail bundle: { space, rules, keywords, logo_url, header_url }.
	// 404 propagates as a SvelteKit error.
	let detail: SpaceDetail;
	try {
		detail = await apiGet<SpaceDetail>(ctx, `/spaces/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Space not found' : 'Failed to load space'
		});
	}

	const space = detail.space;
	const spaceId = space.id;

	// Viewer-scoped management flags from the detail bundle (Ref: §05 lines
	// 198-224). is_owner gates the moderator assignment UI; is_moderator is
	// surfaced for future moderator-only controls. Both default to false for
	// guests — the mutating routes re-check server-side.
	const isOwner = detail.is_owner ?? false;
	const isModerator = detail.is_moderator ?? false;

	// Hydrate the follow button's initial state. Best-effort: a failure
	// degrades to `false` (not following). Only fetch when the viewer is
	// authenticated — guests can't follow.
	const initiallyFollowing = event.locals.user
		? apiGet<{ following: boolean }>(ctx, `/users/${spaceId}/follow-status?entity_type=space`)
				.then((r) => r.following)
				.catch(() => false)
		: Promise.resolve(false);

	// Mute status — only fetched for authed viewers (guests always get false).
	const muteStatus = event.locals.user
		? apiGet<{ muted: boolean }>(ctx, `/spaces/${slug}/mute-status`)
				.then((r) => r.muted)
				.catch(() => false)
		: Promise.resolve(false);

	// Tab data — each degrades to an empty list on failure so a missing tab
	// never blanks the whole space view.
	const canManage = isOwner || isModerator;

	const [
		content, members, forumThreads, moderators, isFollowing,
		isMuted, presets, parentSpaces
	] = await Promise.all([
		apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?page_size=${PAGE_SIZE}`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<SpaceMember>(ctx, `/spaces/${slug}/members?page_size=${PAGE_SIZE}`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<ForumThread>(ctx, `/forum?space_id=${spaceId}&page_size=50`)
			.then((r) => r.items)
			.catch(() => []),
		// Moderators list — only fetched for the owner/moderator so the
		// assignment panel has data to render. The GET route is public, but
		// we avoid the extra request for viewers who can't manage moderators.
		canManage
			? apiList<SpaceModerator>(ctx, `/spaces/${slug}/moderators`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as SpaceModerator[]),
		initiallyFollowing,
		muteStatus,
		// Permission presets — only fetched for authed viewers (the route
		// requires auth). Used in the Settings tab to show preset shortcuts.
		event.locals.user
			? apiList<SpacePermissionPreset>(ctx, `/space-permission-presets`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as SpacePermissionPreset[]),
		// Parent spaces — only fetched for custom spaces (primary spaces have
		// no parents) and only when the viewer is the owner (who can manage
		// parents). Non-owners don't need this data.
		(isOwner && space.level === 'custom')
			? apiList<Space>(ctx, `/spaces/${slug}/parents`)
					.then((r) => r.items)
					.catch(() => [])
			: Promise.resolve([] as Space[])
	]);

	// Phase 08.10 — space analytics dashboard, space-admin only per JEOS
	// §215 [auth, space-admin]. Only fetch for authenticated viewers; the
	// route 403s for non-admins (degrades to null → tab hidden). Guests
	// never see the tab and never trigger the request.
	type AnalyticsResult = { data: unknown } | { error: string };
	const analyticsResult: AnalyticsResult = event.locals.user
		? await apiGet<unknown>(ctx, `/spaces/${spaceId}/analytics`)
				.then((data): AnalyticsResult => ({ data }))
				.catch((err: any): AnalyticsResult => {
					if (err?.status === 403 || err?.code === 'forbidden') {
						return { error: 'permission_denied' };
					}
					return { error: 'unavailable' };
				})
		: { data: null };

	return {
		space,
		rules: detail.rules,
		keywords: detail.keywords ?? [],
		logoUrl: detail.logo_url ?? '',
		headerUrl: detail.header_url ?? '',
		isOwner,
		isModerator,
		moderators,
		presets,
		parentSpaces,
		content,
		members,
		forumThreads,
		analytics: 'data' in analyticsResult ? analyticsResult.data : null,
		analyticsError: 'error' in analyticsResult ? analyticsResult.error : null,
		isMuted,
		pageSize: PAGE_SIZE,
		initiallyFollowing: isFollowing
	};
};
