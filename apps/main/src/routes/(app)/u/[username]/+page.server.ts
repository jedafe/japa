import { error, redirect } from '@sveltejs/kit';
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
	PublicJapaStage,
	ProfileFollower,
	ProfileFollowing,
	VideoSource
} from '$lib/types';

// Raw shape returned by GET /users/{id}/following (FollowingInfo). The backend
// emits entity-scoped fields rather than a PublicUser, so we map it into the
// PublicUser shape the template expects below.
type RawFollowing = {
	entity_id: string;
	entity_slug: string;
	entity_name: string;
	entity_display_name?: string;
	entity_avatar_url?: string;
	is_following?: boolean;
	entity_type?: string;
	created_at?: string;
};

// Raw shape returned by GET /users/{id}/followers (FollowerInfo). `id` is the
// follow row's id — `follower_id` is the actual user id the FollowButton and
// each-key need.
type RawFollower = PublicUser & { follower_id?: string; is_following?: boolean };

/**
 * Public user profile at /u/[username] — SSR, indexable.
 *
 * Fetches the lean profile header plus the data for every center-column tab
 * (Posts, Replies, Media, Likes, Spaces) and the profile timeline feed in a
 * single load pass. Right-sidebar data (certificates, spaces) is also resolved
 * here so the sidebar renders in SSR.
 *
 * Ref: docs/superpowers/specs/2026-09-03-unified-user-tiers-and-professional-profile-removal-design.md
 *      japaconnect-engineering/sections/23-main-site-frontend.md (Profile tabs)
 *      japaconnect-engineering/sections/32-activity-feed-timeline.md (Profile feed)
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const username = event.params.username;

	// Declare an explicit invalidation dependency so follow/unfollow toggles
	// on the client can refresh the profile header counts with
	// invalidate('/api/v1/users/{username}').
	event.depends(`/api/v1/users/${username}`);

	// apiGet/apiList go through ctx.fetch with an absolute backend URL, so
	// SvelteKit never matches client-side invalidate() calls that use the
	// relative /api/v1/... path. Declaring the dependency keys here makes the
	// block/mute toggles' invalidate() calls actually re-run this load and
	// refresh initiallyBlocked/initiallyMuted authoritatively.
	event.depends('/api/v1/me/blocked-users');

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

	// §106: video-channel synthetic users have no public profile — the
	// canonical surface is their /sources/{slug} directory page (a source,
	// not a profile, per the sources-directory model). Resolve one of the
	// channel's videos → its intel source → the canonical slug.
	if (user.is_synthetic) {
		const channelSourceId = await apiList<VideoSource>(ctx, `/u/${username}/videos?limit=1`)
			.then((r) => r.items[0]?.source_id ?? null)
			.catch(() => null);
		let channelSlug: string | null = null;
		if (channelSourceId) {
			channelSlug = await apiGet<{ slug: string }>(ctx, `/sources/${channelSourceId}`)
				.then((s) => s.slug ?? null)
				.catch(() => null);
		}
		if (channelSlug) {
			throw redirect(301, `/sources/${channelSlug}`);
		}
	}

	const userId = user.id;
	const isOwnProfile = !!event.locals.user && event.locals.user.id === userId;

	// See the depends comment above — the mute toggle invalidates
	// /api/v1/users/{id}/mute-status, which must be declared here for the
	// relative-path invalidate to match this load.
	event.depends(`/api/v1/users/${userId}/mute-status`);

	// C2: hydrate the follow button's initial state on SSR. Only fetch when
	// the viewer is authenticated and viewing someone else's profile — own
	// profile has no follow button, and guests can't follow. Best-effort:
	// a failure degrades to `false` (not following).
	const initiallyFollowing =
		!isOwnProfile && event.locals.user
			? apiGet<{ following: boolean }>(ctx, `/users/${userId}/follow-status`)
					.then((r) => r.following)
					.catch(catchFalse('follow-status'))
			: Promise.resolve(false);

	// BUG-007: hydrate the block button's initial state on SSR. Fetches the
	// viewer's blocked-users list and checks whether the profile user is in
	// it. Only fetch when the viewer is authenticated and viewing someone
	// else's profile. Best-effort: a failure degrades to `false` (not blocked).
	const initiallyBlocked =
		!isOwnProfile && event.locals.user
			? apiList<{ blocked_id: string }>(ctx, '/me/blocked-users')
					.then((r) => r.items.some((b) => b.blocked_id === userId))
					.catch(catchFalse('blocked-users'))
			: Promise.resolve(false);

	// FE-1: hydrate the mute button's initial state on SSR. Fetches the
	// per-user mute-status endpoint. Only fetch when the viewer is
	// authenticated and viewing someone else's profile. Best-effort: a
	// failure degrades to `false` (not muted). Ref: Section 52.
	const initiallyMuted =
		!isOwnProfile && event.locals.user
			? apiGet<{ muted: boolean }>(ctx, `/users/${userId}/mute-status`)
				.then((r) => r.muted)
				.catch(catchFalse('mute-status'))
			: Promise.resolve(false);

	// Tab data — each degrades to an empty list on failure (a missing tab must
	// never blank the whole profile). Each tab fetches its dedicated
	// /users/{id}/<tab> endpoint (JEOS api-routes-catalog §334–340).
	// Phase 18.01a — analytics time-series (follower growth + top content) for
	// the LayerChart visualizations on the owner-only analytics tab. Ref: ADR-059.
	// §31 — Follower and following lists are public (the endpoint includes an
	// `is_following` boolean for the caller). Degrade to empty on failure.
	const FOLLOW_LIST_PAGE_SIZE = 100;
	// Tabs filter/search/paginate client-side, so they need the full list —
	// not just the first page. Loop pages (page_size=100 is the backend cap)
	// until a short page or the reported total is reached. Max 5 pages
	// (500 items) bounds worst-case load time for prolific users.
	const MAX_LIST_PAGES = 5;

	async function listAllPages<T>(
		path: string,
		opts: { sizeParam?: string; offsetParam?: string } = {}
	): Promise<T[]> {
		const sizeParam = opts.sizeParam ?? 'page_size';
		const offsetParam = opts.offsetParam ?? 'page_offset';
		const sep = path.includes('?') ? '&' : '?';
		const all: T[] = [];
		const seen = new Set<string>();
		for (let p = 0; p < MAX_LIST_PAGES; p++) {
			const url = `${path}${sep}${sizeParam}=${FOLLOW_LIST_PAGE_SIZE}&${offsetParam}=${p * FOLLOW_LIST_PAGE_SIZE}`;
			const r = await apiList<T>(ctx, url);
			let added = 0;
			for (const item of r.items) {
				const rec = item as { id?: string; entity_id?: string };
				const key = rec.id ?? rec.entity_id ?? JSON.stringify(item);
				if (!seen.has(key)) {
					seen.add(key);
					all.push(item);
					added++;
				}
			}
			// Nothing new — the list is exhausted or the endpoint ignores
			// pagination params (e.g. certificates return all rows).
			if (added === 0) break;
			if (r.items.length < FOLLOW_LIST_PAGE_SIZE) break; // short page = end
			if (r.meta?.total !== undefined && all.length >= r.meta.total) break;
		}
		return all;
	}

	// Best-effort loaders log failures before degrading, so transient API
	// errors do not silently hide content from the profile sidebar or tabs.
	function logLoadError(label: string) {
		return (err: unknown) => {
			console.error(`[profile /u/${username}] ${label} load failed:`, err);
		};
	}
	function catchEmpty<T>(label: string) {
		return (err: unknown): T[] => {
			logLoadError(label)(err);
			return [] as T[];
		};
	}
	function catchNull<T>(label: string) {
		return (err: unknown): T | null => {
			logLoadError(label)(err);
			return null;
		};
	}
	function catchFalse(label: string) {
		return (err: unknown): boolean => {
			logLoadError(label)(err);
			return false;
		};
	}

	const [posts, replies, media, spaces, certificates, likes, analytics, isFollowing, badges, allBadges, followerGrowth, topContent, isBlocked, isMuted, japaStage, followers, following, videos] = await Promise.all([
		// Tabs paginate client-side over the loaded list — fetch every page so
		// totals, search, and paging reflect the whole dataset (capped at
		// MAX_LIST_PAGES × 100).
		listAllPages<ProfilePost>(`/users/${userId}/content`).catch(catchEmpty<ProfilePost>('posts')),
		listAllPages<ProfileReply>(`/users/${userId}/replies`).catch(catchEmpty<ProfileReply>('replies')),
		listAllPages<ProfileMedia>(`/users/${userId}/media`).catch(catchEmpty<ProfileMedia>('media')),
		listAllPages<ProfileSpace>(`/users/${userId}/spaces`).catch(catchEmpty<ProfileSpace>('spaces')),
		listAllPages<ProfileCertificate>(`/users/${userId}/certificates`).catch(catchEmpty<ProfileCertificate>('certificates')),
		listAllPages<ProfileLike>(`/users/${userId}/likes`).catch(catchEmpty<ProfileLike>('likes')),
		// Phase 11.07 — analytics dashboard, owner-only per JEOS §342 [auth,
		// self]. Only fetch for the profile owner; for guests and other users
		// the tab is hidden and the route would 403, so skip the request.
		isOwnProfile
			? apiGet<ProfileAnalytics>(ctx, `/users/${userId}/analytics`).catch(catchNull<ProfileAnalytics>('analytics'))
			: Promise.resolve(null),
		initiallyFollowing,
		// Phase 34.03 — earned badges (Section 64, ADR-080). Own profile uses the
		// authenticated /me/badges endpoint; other profiles use the public
		// username-scoped route. Both return the same BadgeWithDetails shape.
		isOwnProfile
			? apiList<EarnedBadge>(ctx, '/me/badges').then((r) => r.items).catch(catchEmpty<EarnedBadge>('badges'))
			: apiList<EarnedBadge>(ctx, `/users/${username}/badges`).then((r) => r.items).catch(catchEmpty<EarnedBadge>('badges')),
		// Phase 34.03 — full badge catalog, owner-only. Used to surface
		// unearned badges with "how to earn" hints on the own profile.
		isOwnProfile
			? apiList<Badge>(ctx, '/gamification/badges').then((r) => r.items).catch(catchEmpty<Badge>('all-badges'))
			: Promise.resolve([] as Badge[]),
		// Follower growth time-series for the analytics LineChart (owner-only).
		isOwnProfile
			? apiList<FollowerGrowthItem>(ctx, '/me/analytics/followers?days=30').then((r) => r.items).catch(catchEmpty<FollowerGrowthItem>('follower-growth'))
			: Promise.resolve([] as FollowerGrowthItem[]),
		// Top content by views for the analytics BarChart (owner-only).
		isOwnProfile
			? apiList<TopContentItem>(ctx, '/me/analytics/content?page_size=10').then((r) => r.items).catch(catchEmpty<TopContentItem>('top-content'))
			: Promise.resolve([] as TopContentItem[]),
		initiallyBlocked,
		initiallyMuted,
		// §87 — public japa stage. Best-effort: degrades to null when the
		// feature flag is off, the profile is private, or the user has no
		// stage set. The route returns an empty DTO (no stage field) when
		// is_public is false, so we treat a missing stage as "no badge".
		apiGet<PublicJapaStage>(ctx, `/users/${username}/japa-stage`).catch(catchNull<PublicJapaStage>('japa-stage')),
		listAllPages<RawFollower>(`/users/${userId}/followers`)
			.then((items) => items.map((u) => ({ ...u, id: u.follower_id ?? u.id, entity_type: 'user' as const })))
			.catch(catchEmpty<ProfileFollower>('followers')),
		listAllPages<RawFollowing>(`/users/${userId}/following`)
			.then((items) =>
				items.map((f) => ({
					id: f.entity_id, // real followed entity id (fixes /u/ link + FollowButton target)
					username: f.entity_slug, // user-type: == username; else slug
					display_name: f.entity_display_name || f.entity_name,
					avatar_url: f.entity_avatar_url || null,
					is_following: f.is_following,
					entity_type: f.entity_type, // tells the template how to link + which FollowButton entityType to use
					created_at: f.created_at
				}))
			)
			.catch(catchEmpty<ProfileFollowing>('following')),
		// §106 video — channel/uploaded video sources for the profile Videos
		// tab (GET /u/{username}/videos returns VideoSource rows; it paginates
		// with limit/offset rather than page_size/page_offset). Best-effort:
		// a failure degrades to an empty list so the tab never blanks the page.
		listAllPages<VideoSource>(`/u/${username}/videos`, { sizeParam: 'limit', offsetParam: 'offset' }).catch(catchEmpty<VideoSource>('videos'))
	]);

	return {
		user,
		isOwnProfile,
		isAuthenticated: !!event.locals.user,
		currentUser: event.locals.user ?? null,
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
		followers,
		following,
		videos,
		jsonLd
	};
};
