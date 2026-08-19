import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { BlogSeries } from '$lib/types';

/**
 * A post belonging to a blog series (GET /blog/series/{id}/posts).
 * Ordered by position; each row links to /blog/{slug}.
 */
export interface BlogSeriesPost {
	content_id: string;
	title: string;
	slug: string;
	position: number;
	status: string;
	created_at: string;
}

/**
 * Blog series detail at /blog/series/[slug] — public, SSR, indexable.
 * Fetches the series (GET /blog/series/{slug}) and its ordered posts
 * (GET /blog/series/{id}/posts). On 404, throws a SvelteKit 404.
 *
 * Ref: japaconnect-engineering/sections/14-content-type-system.md
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.blog']) {
		throw error(404, { code: 'feature_disabled', message: 'Blog is not available' });
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let series: BlogSeries;
	try {
		series = await apiGet<BlogSeries>(ctx, `/blog/series/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Series not found' });
		}
		throw error(500, { code: 'internal_error', message: 'Failed to load series' });
	}

	// Posts in the series — degrades to an empty list on failure so the
	// series header still renders.
	let posts: BlogSeriesPost[] = [];
	try {
		posts = (await apiList<BlogSeriesPost>(ctx, `/blog/series/${series.id}/posts`)).items;
	} catch {
		posts = [];
	}

	return { series, posts };
};
