// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { AMAThread, ForumReply } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;
	const authed = !!event.locals.user;

	let session: AMAThread;
	try {
		session = await apiGet<AMAThread>(ctx, `/forum/ama/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'AMA not found' });
		}
		return { session: null, replies: [], authed, isHost: false, loadError: true };
	}

	let replies: ForumReply[] = [];
	try {
		const { items } = await apiList<ForumReply>(ctx, `/forum/${session.id}/replies`);
		replies = items;
	} catch {
		replies = [];
	}

	const isHost = false;

	return { session, replies, authed, isHost, loadError: false };
};
