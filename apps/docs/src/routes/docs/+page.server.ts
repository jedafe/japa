import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { buildMeta, webSiteLd } from '$lib/seo/meta';
import { tServer } from '$lib/i18n/dict';
import type { DocPageSummary } from '$lib/types';

/**
 * Docs landing: the portal homepage — hero, user-group cards, category
 * cards, popular pages, and the "What's new" changelog column.
 *
 * The nav tree is already fetched by the layout; this load additionally
 * pulls the published-page summary lists for every accessible audience
 * (parallel, fail-soft) so the changelog section can sort by created_at.
 * Each remote call degrades gracefully — a failed list just yields a
 * shorter changelog, never a broken landing.
 */
export const load: PageServerLoad = async (event) => {
	const data = await event.parent();
	const locale = data.locale ?? event.locals.locale;
	const title = tServer(locale, 'landing.hero_title', 'JapaConnect Documentation');
	const description = tServer(
		locale,
		'landing.hero_subtitle',
		'Everything you need to build on, integrate with, and use JapaConnect.'
	);
	const meta = buildMeta({
		title,
		description,
		path: '/docs',
		ogType: 'website',
		jsonLd: webSiteLd({ name: title, description })
	});

	const ctx = apiContext(event);
	const results = await Promise.allSettled(
		event.locals.accessibleAudiences.map((audience) =>
			apiGet<DocPageSummary[]>(ctx, `/docs/${audience}`)
		)
	);
	const summaries: DocPageSummary[] = [];
	for (const r of results) {
		if (r.status === 'fulfilled' && Array.isArray(r.value)) summaries.push(...r.value);
	}

	return { meta, summaries };
};
