import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { ProfessionalProfile, ProfilePost } from '$lib/types';

/**
 * Professional profile at /profiles/[slug] — auth-gated. Fetches the profile
 * header, the profile's published content (Posts tab), and renders name,
 * tagline, verified badge, description, website, follower count, custom
 * field_values, and a follow button.
 *
 * Ref: japaconnect-engineering/sections/33-professional-profiles.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

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

	// Hydrate the follow button's initial state. Best-effort: a failure
	// degrades to `false` (not following). The viewer is always authenticated
	// here (the page is auth-gated above).
	const initiallyFollowing = await apiGet<{ following: boolean }>(
		ctx,
		`/users/${profile.id}/follow-status?entity_type=profile`
	)
		.then((r) => r.following)
		.catch(() => false);

	return { profile, posts, initiallyFollowing };
};
