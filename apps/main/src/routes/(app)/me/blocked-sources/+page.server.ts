import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Source } from '$lib/types/sources';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/me/blocked-sources');
	}

	const ctx = apiContext(event);
	const { items: sources } = await apiList<Source>(ctx, '/me/blocked-sources');

	return { sources };
};
