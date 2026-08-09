// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { Space } from '$lib/types';

export const load: PageServerLoad = async (event) => {
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
