import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { loadContentArticle } from '$lib/server/content-article';
import { contentHref } from '$lib/utils/contentLink';

/**
 * Generic content single page (CPTs without a dedicated route family).
 * ADR-058: types with a canonical route (guide → /guide/{slug},
 * story → /story/{slug}, blog_post → /blog/{slug}, …) are 301-redirected
 * so every content item has exactly one canonical URL.
 */
export const load: PageServerLoad = async (event) => {
	const data = await loadContentArticle(event);

	const canonical = contentHref(data.article);
	if (!canonical.startsWith('/content/')) {
		throw redirect(301, `${canonical}${event.url.search}`);
	}

	return data;
};
