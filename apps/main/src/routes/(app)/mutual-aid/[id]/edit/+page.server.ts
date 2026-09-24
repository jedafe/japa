// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { MutualAidPost, MutualAidPostType, MutualAidCategory, GeoCountry } from '$lib/types';

/**
 * Edit mutual aid post at /mutual-aid/[id]/edit — loads the post and
 * country list for the edit form. Only the post author can edit (the
 * backend enforces ownership on PATCH).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const ctx = apiContext(event);
	const id = event.params.id;

	let post: MutualAidPost;
	try {
		post = await apiGet<MutualAidPost>(ctx, `/mutual-aid/${id}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Mutual aid post not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load mutual aid post' });
	}

	// Client-side ownership hint — the backend enforces this on PATCH.
	if (post.author_id && post.author_id !== event.locals.user?.id) {
		throw error(403, { code: 'forbidden', message: 'You can only edit your own posts' });
	}

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	let postTypes: MutualAidPostType[] = [];
	let categories: MutualAidCategory[] = [];
	try {
		const pt = await apiGet<{ value: string; label: string }[]>(ctx, '/mutual-aid/post-types');
		postTypes = (pt ?? []).map((o) => o.value as MutualAidPostType);
		const cat = await apiGet<{ value: string; label: string }[]>(ctx, '/mutual-aid/categories');
		categories = (cat ?? []).map((o) => o.value as MutualAidCategory);
	} catch {
		postTypes = [];
		categories = [];
	}

	return { post, countries, postTypes, categories };
};
