import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { contentHref } from '$lib/utils/contentLink';

/**
 * Guide single page at /guide/{slug} — the canonical detail route for
 * content_type 'guide' (§14, ADR-058 route families). Shares the generic
 * content loader; a non-guide slug under /guide is redirected to its own
 * canonical route.
 */
export const load: PageServerLoad = async (event) => {
	const data = await loadContentArticle(event);

	if (data.article.content_type !== 'guide') {
		throw redirect(301, `${contentHref(data.article)}${event.url.search}`);
	}

	return data;
};
