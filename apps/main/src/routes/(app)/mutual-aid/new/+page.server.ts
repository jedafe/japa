// Ref: japaconnect-engineering/sections/85-mutual-aid-board.md
import type { PageServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { GeoCountry, MutualAidPostType, MutualAidCategory } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}

	const ctx = apiContext(event);

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

	return { countries, postTypes, categories };
};
