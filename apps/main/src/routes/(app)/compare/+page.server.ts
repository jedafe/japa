import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { University } from '$lib/types';

/**
 * Compare at /compare — public university comparison tool. SSR-fetches the
 * full university list so the picker renders immediately; the comparison
 * table is built client-side by fetching /universities/{id}/compare for each
 * selected university.
 *
 * Ref: japaconnect-engineering/sections/39-universities.md
 */
const PAGE_SIZE = 50;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let universities: University[] = [];
	try {
		universities = (await apiList<University>(ctx, `/universities?page_size=${PAGE_SIZE}`)).items;
	} catch {
		universities = [];
	}

	return { universities };
};
