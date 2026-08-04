// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
// Lists the spaces the current user has joined. Auth-gated.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MeSpace } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	return { spaces };
};
