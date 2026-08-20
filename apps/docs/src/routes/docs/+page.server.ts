import type { PageServerLoad } from './$types';
import { buildMeta, webSiteLd } from '$lib/seo/meta';
import { tServer } from '$lib/i18n/dict';

/**
 * Docs landing: lists the accessible audiences as entry points + popular
 * pages from the nav tree. The nav tree is already fetched by the layout;
 * this load only adds SEO meta (no extra API calls).
 */
export const load: PageServerLoad = async ({ parent, locals }) => {
	const data = await parent();
	const locale = data.locale ?? locals.locale;
	const title = tServer(locale, 'landing.title', 'JapaConnect Documentation');
	const description = tServer(
		locale,
		'landing.subtitle',
		'Everything you need to build on, integrate with, and use JapaConnect.'
	);
	const meta = buildMeta({
		title,
		description,
		path: '/docs',
		jsonLd: webSiteLd({ name: title, description })
	});
	return { meta };
};
