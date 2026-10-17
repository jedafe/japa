import { error } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext } from '$lib/server/api';
import { assertDocEditor, fetchAuthoringData, createDocPageAction } from '$lib/server/docs-author';
import { buildMeta } from '$lib/seo/meta';
import type { DocAudience } from '$lib/types';
import { AUDIENCE_LABEL_KEY } from '$lib/types';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

/**
 * /docs/{audience}/new — in-app page authoring (§44 amendment 2).
 * Server-side gated: requires both read access to the audience and the
 * docs.manage permission node. The shared docs-author module owns the form
 * contract so /docs/api/new stays identical.
 */
export const load: PageServerLoad = async (event) => {
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	const audience = event.params.audience as DocAudience;

	if (!event.locals.audienceOrder.includes(audience)) {
		throw error(404, { code: 'not_found', message: 'Audience not found' });
	}

	assertDocEditor(event, audience);

	const { statuses, existingPages } = await fetchAuthoringData(apiContext(event), audience);

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const audienceLabel = tServer(locale, AUDIENCE_LABEL_KEY[audience], audience, undefined, site);
	const title = `${tServer(locale, 'new_page.title', 'New page', undefined, site)} — ${audienceLabel} · ${appName}`;

	const meta = buildMeta({
		title,
		description: tServer(locale, 'new_page.description', 'Create a new documentation page.', undefined, site),
		path: `/docs/${audience}/new`
	});

	return {
		audience,
		labelKey: AUDIENCE_LABEL_KEY[audience],
		audienceLabel,
		statuses,
		existingPages,
		nextPosition: existingPages.reduce((m, p) => Math.max(m, p.position ?? 0), -1) + 1,
		meta,
		isGated: true // authoring UI is never indexable
	};
};

export const actions: Actions = {
	create: async (event) => createDocPageAction(event, event.params.audience as DocAudience)
};
