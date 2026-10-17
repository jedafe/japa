import type { PageServerLoad, Actions } from './$types';
import { apiContext } from '$lib/server/api';
import { assertDocEditor, fetchAuthoringData, createDocPageAction } from '$lib/server/docs-author';
import { buildMeta } from '$lib/seo/meta';
import { tServer, DEFAULT_SITE_NAME } from '$lib/i18n/dict';

/**
 * /docs/api/new — the api audience's create route. A dedicated static route
 * because docs/api/[slug] would otherwise capture "new" as a page slug (§44
 * amendment 2; "new" is also a reserved slug backend-side).
 */
export const load: PageServerLoad = async (event) => {
	const site = (((await event.parent())?.siteSettings ?? {}) as Record<string, string>)['general.site_name'] ?? DEFAULT_SITE_NAME;
	assertDocEditor(event, 'api');

	const { statuses, existingPages } = await fetchAuthoringData(apiContext(event), 'api');

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', '{site} Docs', undefined, site);
	const audienceLabel = tServer(locale, 'audience.api', 'API', undefined, site);
	const title = `${tServer(locale, 'new_page.title', 'New page', undefined, site)} — ${audienceLabel} · ${appName}`;

	const meta = buildMeta({
		title,
		description: tServer(locale, 'new_page.description', 'Create a new documentation page.', undefined, site),
		path: '/docs/api/new'
	});

	return {
		audience: 'api' as const,
		labelKey: 'audience.api',
		audienceLabel,
		statuses,
		existingPages,
		nextPosition: existingPages.reduce((m, p) => Math.max(m, p.position ?? 0), -1) + 1,
		meta,
		isGated: true
	};
};

export const actions: Actions = {
	create: async (event) => createDocPageAction(event, 'api')
};
