// Ref: japaconnect-engineering/sections/82-japa-planning-groups.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);

	let countries: Space[] = [];
	try {
		const { items } = await apiList<Space>(ctx, '/spaces?level=country');
		countries = items;
	} catch {
		countries = [];
	}

	return { countries };
};
