import type { PageServerLoad } from './$types';
import { collectCategories, mergeManagedCategories, parseManagedCategories } from '$lib/docs/catalog';
import { buildMeta } from '$lib/seo/meta';
import { tServer } from '$lib/i18n/dict';

/**
 * Categories index: every doc category across all accessible audiences,
 * derived from the layout's audience-gated nav tree (no extra API calls).
 */
export const load: PageServerLoad = async (event) => {
	const data = await event.parent();
	const categories = mergeManagedCategories(
		collectCategories(data.nav),
		parseManagedCategories(data.siteSettings?.['docs.categories'])
	);

	const locale = data.locale ?? event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const title = `${tServer(locale, 'categories.title', 'Documentation categories')} · ${appName}`;
	const description = tServer(
		locale,
		'categories.sub',
		'Browse all documentation by category and sub-category.'
	);
	const meta = buildMeta({ title, description, path: '/docs/categories', ogType: 'website' });

	return { categories, meta };
};
