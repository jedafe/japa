// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
// Lists the spaces the current user has joined. Auth-gated.
import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MeSpace } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required to view spaces' });
	}
	const ctx = apiContext(event);
	const spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	return { spaces };
};
