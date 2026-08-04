import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ProfessionalProfile, ProfilePost } from '$lib/types';

/**
 * Professional profile at /profiles/[slug] — public SSR (Phase 12.08). Guests
 * can view the profile header and published content; the follow button and
 * follow-status are only hydrated for authenticated viewers.
 *
 * Ref: japaconnect-engineering/sections/33-professional-profiles.md
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;
	const authed = !!event.locals.user;

	let profile: ProfessionalProfile;
	try {
		profile = await apiGet<ProfessionalProfile>(ctx, `/profiles/${slug}`);
	} catch (e) {
		const status = e instanceof ApiError ? e.status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Profile not found' : 'Failed to load profile'
		});
	}

	// Best-effort fetch of the profile's published content for the Posts tab.
	let posts: ProfilePost[] = [];
	try {
		posts = (await apiList<ProfilePost>(ctx, `/profiles/${profile.id}/content`)).items;
	} catch {
		posts = [];
	}

	// Hydrate the follow button's initial state only for authenticated
	// viewers. Guests see the follow button but it redirects to login.
	let initiallyFollowing = false;
	if (authed) {
		initiallyFollowing = await apiGet<{ following: boolean }>(
			ctx,
			`/users/${profile.id}/follow-status?entity_type=profile`
		)
			.then((r) => r.following)
			.catch(() => false);
	}

	return { profile, posts, initiallyFollowing, authed };
};
