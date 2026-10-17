import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import {
	categorySlug,
	findCategory,
	parseManagedCategories,
	type DocsCategory,
	type DocsCategoryItem
} from '$lib/docs/catalog';
import { buildMeta, collectionPageLd, DOCS_ORIGIN } from '$lib/seo/meta';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';
import type { DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';

export interface CategoryAudienceGroup {
	audience: DocAudience;
	labelKey: string;
	/** '' = items with no subcategory (rendered first, no header). */
	subgroups: { name: string; items: DocsCategoryItem[] }[];
}

/**
 * Category page: every published page in a category across all accessible
 * audiences, grouped audience → subcategory. Derived entirely from the
 * layout's audience-gated nav tree — no extra API calls, no leakage of
 * gated pages (ADR-060). Unknown category slugs 404.
 */
export const load: PageServerLoad = async (event) => {
	const data = await event.parent();
	const site = (data.siteSettings?.['general.site_name'] as string | undefined) ?? DEFAULT_SITE_NAME;
	let category: DocsCategory | null = findCategory(data.nav, event.params.name);
	if (!category) {
		// Managed-catalog categories with no published pages yet still
		// resolve — the page renders an empty state instead of a 404.
		const decoded = decodeURIComponent(event.params.name).toLowerCase();
		const managed = parseManagedCategories(data.siteSettings?.['docs.categories']).find(
			(m) => m.name.toLowerCase() === decoded || categorySlug(m.name) === categorySlug(decoded)
		);
		if (managed) {
			category = {
				name: managed.name,
				slug: categorySlug(managed.name),
				subcategories: managed.subcategories,
				items: [],
				description: managed.description,
				icon: managed.icon
			};
		}
	}
	if (!category) {
		throw error(404, { code: 'not_found', message: 'Category not found' });
	}

	const audienceOrder: DocAudience[] = data.audienceOrder ?? [];
	const byAudience = new Map<DocAudience, DocsCategoryItem[]>();
	for (const item of category.items) {
		const list = byAudience.get(item.audience) ?? [];
		list.push(item);
		byAudience.set(item.audience, list);
	}

	const groups: CategoryAudienceGroup[] = [...byAudience.entries()]
		.sort(
			([a], [b]) =>
				(audienceOrder.indexOf(a) === -1 ? 99 : audienceOrder.indexOf(a)) -
				(audienceOrder.indexOf(b) === -1 ? 99 : audienceOrder.indexOf(b))
		)
		.map(([audience, items]) => {
			const subs = new Map<string, DocsCategoryItem[]>();
			for (const item of items) {
				const key = item.subcategory?.trim() ?? '';
				const list = subs.get(key) ?? [];
				list.push(item);
				subs.set(key, list);
			}
			return {
				audience,
				labelKey: AUDIENCE_LABEL_KEY[audience],
				subgroups: [...subs.entries()].map(([name, subItems]) => ({ name, items: subItems }))
			};
		});

	const locale = data.locale ?? event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const title = `${category.name} · ${appName}`;
	const description = tServer(
		locale,
		'categories.sub',
		'Browse all documentation by category and sub-category.',
		undefined,
		site
	);
	const meta = buildMeta({
		title,
		description,
		path: `/docs/category/${category.slug}`,
		ogType: 'website',
		jsonLd: collectionPageLd({
				siteName: site,
				name: title,
			url: `${DOCS_ORIGIN}/docs/category/${category.slug}`,
			description
		})
	});

	return { category, groups, meta };
};
