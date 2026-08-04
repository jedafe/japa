import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPageSummary, DocAudience } from '$lib/types';
import { AUDIENCE_ORDER, AUDIENCE_LABEL_KEY } from '$lib/types';

/**
 * Audience landing: lists published pages for the given audience. Gated
 * audiences (anything other than 'api' for guests) are enforced server-side:
 * if the audience isn't in accessibleAudiences, redirect to /docs/access-denied
 * (ADR-060 — never client checks).
 */
export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;

	// 'api' is fully public; other audiences require membership.
	if (!event.locals.accessibleAudiences.includes(audience)) {
		throw redirect(308, '/docs/access-denied');
	}

	const ctx = apiContext(event);
	let pages: DocPageSummary[] = [];
	try {
		pages = await apiGet<DocPageSummary[]>(ctx, `/docs/${audience}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Audience not found' });
		}
		// Degrade to empty list — never blank the shell.
		pages = [];
	}

	const labelKey = AUDIENCE_LABEL_KEY[audience];
	const title = `${audience[0].toUpperCase()}${audience.slice(1)} · JapaConnect Docs`;
	// Gated audiences get noindex (Ref: §26 ROADMAP 07.02). Public audiences
	// (users, api) are indexable.
	const isGated = audience !== 'users' && audience !== 'api';
	const meta = buildMeta({
		title,
		description: `Documentation for the ${audience} audience.`,
		path: `/docs/${audience}`,
		jsonLd: techArticleLd({
			headline: title,
			dateModified: new Date().toISOString(),
			url: `${DOCS_ORIGIN}/docs/${audience}`,
			description: `Documentation for the ${audience} audience.`
		})
	});

	return { audience, pages, labelKey, meta, isGated };
};
