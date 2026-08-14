import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiGetEnvelope, apiList } from '$lib/server/api';
import type {
	PublicUser,
	ProfilePost,
	ProfileReply,
	ProfileSpace,
	ProfileCertificate,
	ProfileMedia,
	ProfileLike,
	ProfileAnalytics,
	EarnedBadge,
	Badge,
	FollowerGrowthItem,
	TopContentItem,
	PublicJapaStage
} from '$lib/types';

/**
 * Public user profile at /u/[username] — SSR, indexable.
 *
 * Fetches the lean profile header plus the data for every center-column tab
 * (Posts, Replies, Media, Likes, Spaces) and the profile timeline feed in a
 * single load pass. Right-sidebar data (certificates, spaces) is also resolved
 * here so the sidebar renders in SSR.
 *
 * Ref: japaconnect-engineering/sections/33-professional-profiles.md
 *      japaconnect-engineering/sections/23-main-site-frontend.md (Profile tabs)
 *      japaconnect-engineering/sections/32-activity-feed-timeline.md (Profile feed)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const username = event.params.username;

	// Profile header — 404 propagates as a SvelteKit error (banned/suspended/deleted).
	let user: PublicUser;
	let jsonLd: unknown = null;
	try {
		const envelope = await apiGetEnvelope<PublicUser>(ctx, `/users/${username}`);
		user = envelope.data;
		jsonLd = envelope.json_ld ?? null;
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'User not found' : 'Failed to load profile'
		});
	}

	const userId = user.id;
	const isOwnProfile = !!event.locals.user && event.locals.user.id === userId;

	// C2: hydrate the follow button's initial state on SSR. Only fetch when
	// the viewer is authenticated and viewing someone else's profile — own
	// profile has no follow button, and guests can't follow. Best-effort:
	// a failure degrades to `false` (not following).
	const initiallyFollowing =
		!isOwnProfile && event.locals.user
			? apiGet<{ following: boolean }>(ctx, `/users/${userId}/follow-status`)
					.then((r) => r.following)
					.catch(() => false)
			: Promise.resolve(false);

	// BUG-007: hydrate the block button's initial state on SSR. Fetches the
	// viewer's blocked-users list and checks whether the profile user is in
	// it. Only fetch when the viewer is authenticated and viewing someone
	// else's profile. Best-effort: a failure degrades to `false` (not blocked).
	const initiallyBlocked =
		!isOwnProfile && event.locals.user
			? apiList<{ blocked_id: string }>(ctx, '/me/blocked-users')
					.then((r) => r.items.some((b) => b.blocked_id === userId))
					.catch(() => false)
			: Promise.resolve(false);

	// FE-1: hydrate the mute button's initial state on SSR. Fetches the
	// per-user mute-status endpoint. Only fetch when the viewer is
	// authenticated and viewing someone else's profile. Best-effort: a
	// failure degrades to `false` (not muted). Ref: Section 52.
	const initiallyMuted =
		!isOwnProfile && event.locals.user
			? apiGet<{ muted: boolean }>(ctx, `/users/${userId}/mute-status`)
				.then((r) => r.muted)
				.catch(() => false)
			: Promise.resolve(false);

	// Tab data — each degrades to an empty list on failure (a missing tab must
	// never blank the whole profile). Each tab fetches its dedicated
	// /users/{id}/<tab> endpoint (JEOS api-routes-catalog §334–340).
	// Phase 18.01a — analytics time-series (follower growth + top content) for
	// the LayerChart visualizations on the owner-only analytics tab. Ref: ADR-059.
	const [posts, replies, media, spaces, certificates, likes, analytics, isFollowing, badges, allBadges, followerGrowth, topContent, isBlocked, isMuted, japaStage] = await Promise.all([
		apiList<ProfilePost>(ctx, `/users/${userId}/content`).then((r) => r.items).catch(() => []),
		apiList<ProfileReply>(ctx, `/users/${userId}/replies`).then((r) => r.items).catch(() => []),
		apiList<ProfileMedia>(ctx, `/users/${userId}/media`).then((r) => r.items).catch(() => []),
		apiList<ProfileSpace>(ctx, `/users/${userId}/spaces`).then((r) => r.items).catch(() => []),
		apiList<ProfileCertificate>(ctx, `/users/${userId}/certificates`).then((r) => r.items).catch(() => []),
		apiList<ProfileLike>(ctx, `/users/${userId}/likes`).then((r) => r.items).catch(() => []),
		// Phase 11.07 — analytics dashboard, owner-only per JEOS §342 [auth,
		// self]. Only fetch for the profile owner; for guests and other users
		// the tab is hidden and the route would 403, so skip the request.
		isOwnProfile
			? apiGet<ProfileAnalytics>(ctx, `/users/${userId}/analytics`).catch(() => null)
			: Promise.resolve(null),
		initiallyFollowing,
		// Phase 34.03 — earned badges (Section 64, ADR-080). Public route.
		apiList<EarnedBadge>(ctx, `/users/${username}/badges`).then((r) => r.items).catch(() => []),
		// Phase 34.03 — full badge catalog, owner-only. Used to surface
		// unearned badges with "how to earn" hints on the own profile.
		isOwnProfile
			? apiList<Badge>(ctx, '/gamification/badges').then((r) => r.items).catch(() => [])
			: Promise.resolve([] as Badge[]),
		// Follower growth time-series for the analytics LineChart (owner-only).
		isOwnProfile
			? apiList<FollowerGrowthItem>(ctx, '/me/analytics/followers?days=30').then((r) => r.items).catch(() => [])
			: Promise.resolve([] as FollowerGrowthItem[]),
		// Top content by views for the analytics BarChart (owner-only).
		isOwnProfile
			? apiList<TopContentItem>(ctx, '/me/analytics/content?page_size=10').then((r) => r.items).catch(() => [])
			: Promise.resolve([] as TopContentItem[]),
		initiallyBlocked,
		initiallyMuted,
		// §87 — public japa stage. Best-effort: degrades to null when the
		// feature flag is off, the profile is private, or the user has no
		// stage set. The route returns an empty DTO (no stage field) when
		// is_public is false, so we treat a missing stage as "no badge".
		apiGet<PublicJapaStage>(ctx, `/users/${username}/japa-stage`).catch(() => null)
	]);

	return {
		user,
		isOwnProfile,
		initiallyFollowing: isFollowing,
		initiallyBlocked: isBlocked,
		initiallyMuted: isMuted,
		posts,
		replies,
		media,
		spaces,
		certificates,
		likes,
		analytics,
		badges,
		allBadges,
		followerGrowth,
		topContent,
		japaStage,
		jsonLd
	};
};
