import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { BlogSeries } from '$lib/types';
import type { SeriesPost } from '$lib/types/content';

/**
 * Series detail at /series/[slug] — public, SSR, indexable.
 * Fetches the series (GET /series/{slug}) and its ordered posts
 * (GET /series/{id}/posts — each row carries content_type so part links
 * route to their own family via contentHref). On 404, throws a SvelteKit 404.
 *
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 *      japaconnect-engineering/sections/24-blog-extension.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Series is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let series: BlogSeries;
	try {
		series = await apiGet<BlogSeries>(ctx, `/series/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Series not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load series' });
	}

	// Posts in the series — degrades to an empty list on failure so the
	// series header still renders.
	let posts: SeriesPost[] = [];
	try {
		posts = (await apiList<SeriesPost>(ctx, `/series/${series.id}/posts`)).items;
	} catch {
		posts = [];
	}

	return { series, posts };
};
