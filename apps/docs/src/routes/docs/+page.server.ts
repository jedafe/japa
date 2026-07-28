import type { PageServerLoad } from './$types';
import { buildMeta, webSiteLd } from '$lib/seo/meta';

/**
 * Docs landing: lists the accessible audiences as entry points + popular
 * pages from the nav tree. The nav tree is already fetched by the layout;
 * this load only adds SEO meta (no extra API calls).
 */
export const load: PageServerLoad = async ({ parent }) => {
	const data = await parent();
	const meta = buildMeta({
		title: 'JapaConnect Documentation',
		description:
			'Everything you need to build on, integrate with, and use JapaConnect — developer API reference, user guides, and admin docs.',
		path: '/docs',
		jsonLd: webSiteLd()
	});
	return { meta };
};
