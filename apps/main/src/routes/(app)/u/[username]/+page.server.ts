import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	PublicUser,
	ProfilePost,
	ProfileReply,
	ProfileSpace,
	ProfileCertificate,
	ProfileMedia,
	FeedItem
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
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const username = event.params.username;

	// Profile header — 404 propagates as a SvelteKit error (banned/suspended/deleted).
	let user: PublicUser;
	try {
		user = await apiGet<PublicUser>(ctx, `/users/${username}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'User not found' : 'Failed to load profile'
		});
	}

	const userId = user.id;
	const isOwnProfile = !!event.locals.user && event.locals.user.id === userId;

	// Tab data — each degrades to an empty list on failure (a missing tab must
	// never blank the whole profile).
	const [posts, replies, media, spaces, certificates, feed, analytics] = await Promise.all([
		apiList<ProfilePost>(ctx, `/users/${userId}/content`).then((r) => r.items).catch(() => []),
		apiList<ProfileReply>(ctx, `/users/${userId}/replies`).then((r) => r.items).catch(() => []),
		apiList<ProfileMedia>(ctx, `/users/${userId}/media`).then((r) => r.items).catch(() => []),
		apiList<ProfileSpace>(ctx, `/users/${userId}/spaces`).then((r) => r.items).catch(() => []),
		apiList<ProfileCertificate>(ctx, `/users/${userId}/certificates`).then((r) => r.items).catch(() => []),
		apiList<FeedItem>(ctx, `/feed/profile/${userId}?page_size=10`).then((r) => r.items).catch(() => []),
		// Phase 11.07 — public analytics summary (best-effort).
		apiGet<unknown>(ctx, `/users/${userId}/analytics`).catch(() => null)
	]);

	return {
		user,
		isOwnProfile,
		posts,
		replies,
		media,
		spaces,
		certificates,
		feed,
		analytics
	};
};
