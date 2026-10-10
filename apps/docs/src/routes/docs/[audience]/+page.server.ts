import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import { buildMeta, techArticleLd, DOCS_ORIGIN } from '$lib/seo/meta';
import type { DocPageSummary, DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';
import { tServer } from '$lib/i18n/dict';

/**
 * Audience landing: lists published pages for the given audience. Gated
 * audiences (anything other than 'api' for guests) are enforced server-side:
 * if the audience isn't in accessibleAudiences, redirect to /docs/access-denied
 * (ADR-060 — never client checks).
 */
export const load: PageServerLoad = async (event) => {
	const audience = event.params.audience as DocAudience;

	// An audience slug outside the canonical list isn't gated — it doesn't
	// exist. 404 it; access-denied is only for real-but-gated audiences.
	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

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

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const labelKey = AUDIENCE_LABEL_KEY[audience];
	const audienceLabel = tServer(locale, labelKey, audience);
	const title = `${audienceLabel} · ${appName}`;
	const description = tServer(
		locale,
		'page.audience_description',
		'Documentation for the {audience} audience.',
		{ audience: audienceLabel }
	);

	// Gated audiences get noindex (Ref: §26 ROADMAP 07.02). Presence in the
	// backend gates map is the source of truth — a new public audience added
	// upstream must not silently become noindex here.
	const isGated = event.locals.audienceGates[audience] != null;
	const meta = buildMeta({
		title,
		description,
		path: `/docs/${audience}`,
		jsonLd: techArticleLd({
			headline: title,
			dateModified: new Date().toISOString(),
			url: `${DOCS_ORIGIN}/docs/${audience}`,
			description
		})
	});

	// docs.manage holders get the "New page" affordance (§44 in-app authoring).
	const canEditDocs = event.locals.user?.permissions?.includes('docs.manage') ?? false;

	return { audience, pages, labelKey, meta, isGated, canEditDocs };
};
