import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { BlogSeries } from '$lib/types';

/**
 * Blog series listing at /blog/series — public, SSR, indexable.
 * Lists all blog series (GET /blog/series). May return an empty array.
 *
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);

	let series: BlogSeries[] = [];
	try {
		series = (await apiList<BlogSeries>(ctx, '/blog/series')).items;
	} catch {
		series = [];
	}

	return { series };
};
