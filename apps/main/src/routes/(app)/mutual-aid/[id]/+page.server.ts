// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { MutualAidPost } from '$lib/types';

/**
 * Single mutual aid post at /mutual-aid/[id] — rendered inside the app shell.
 * Fetches GET /mutual-aid/{id}. The backend's response envelope includes the
 * author join; ownership is derived from the session user for action buttons.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	let post: MutualAidPost;
	try {
		post = await apiGet<MutualAidPost>(ctx, `/mutual-aid/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Mutual aid post not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load mutual aid post' });
	}

	return { post, authed, currentUserId, loadError: false };
};
