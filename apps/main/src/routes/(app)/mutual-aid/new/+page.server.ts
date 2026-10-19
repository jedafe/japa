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

	const countriesPromise = apiList<GeoCountry>(ctx, '/geo/countries')
		.then((r) => r.items)
		.catch(() => [] as GeoCountry[]);

	let postTypes: MutualAidPostType[] = [];
	let categories: MutualAidCategory[] = [];
	try {
		const [pt, cat] = await Promise.all([
			apiGet<{ value: string; label: string }[]>(ctx, '/mutual-aid/post-types'),
			apiGet<{ value: string; label: string }[]>(ctx, '/mutual-aid/categories')
		]);
		postTypes = (pt ?? []).map((o) => o.value as MutualAidPostType);
		categories = (cat ?? []).map((o) => o.value as MutualAidCategory);
	} catch {
		postTypes = [];
		categories = [];
	}

	const countries = await countriesPromise;

	return { countries, postTypes, categories };
};
