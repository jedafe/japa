// Ref: japaconnect-engineering/sections/89-achievement-feed.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { redirect } from '@sveltejs/kit';
import type { Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/achievements/new');
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
