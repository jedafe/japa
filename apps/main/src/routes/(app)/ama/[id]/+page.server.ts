// Ref: japaconnect-engineering/sections/88-ask-an-expat.md
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { AMAThread, ForumReply, GeoCountry } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['forum.ama.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Ask an Expat AMA is not available' });
	}

	const ctx = apiContext(event);
	const id = event.params.id;
	const authed = !!event.locals.user;
	const currentUserId = event.locals.user?.id ?? null;

	// Countries resolve host_dest_country_id to a name (BUG-007) — independent
	// of the session, so it rides the same wave.
	const countriesPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then((r) => r.items)
		.catch(() => [] as GeoCountry[]);

	let session: AMAThread;
	try {
		session = await apiGet<AMAThread>(ctx, `/forum/ama/${id}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'AMA not found' });
		}
		return { session: null, replies: [], countries: [], authed, isHost: false, loadError: true };
	}

	const [fetchedReplies, countries] = await Promise.all([
		apiList<ForumReply>(ctx, `/forum/${session.id}/replies`)
			.then((r) => r.items)
			.catch(() => [] as ForumReply[]),
		countriesPromise
	]);
	let replies = fetchedReplies;

	const isHost = authed && !!session.author_user_id && session.author_user_id === currentUserId;

	// BUG-003: filter out pending AMA questions for non-host viewers. The
	// host sees all questions (including pending) so they can approve them.
	if (!isHost) {
		replies = replies.filter((r) => !r.ama_pending);
	}

	return { session, replies, countries, authed, isHost, loadError: false };
};
