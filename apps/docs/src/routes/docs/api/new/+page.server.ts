import type { PageServerLoad, Actions } from './$types';
import { apiContext } from '$lib/server/api';
import { assertDocEditor, fetchAuthoringData, createDocPageAction } from '$lib/server/docs-author';
import { buildMeta } from '$lib/seo/meta';
import { tServer } from '$lib/i18n/dict';

/**
 * /docs/api/new — the api audience's create route. A dedicated static route
 * because docs/api/[slug] would otherwise capture "new" as a page slug (§44
 * amendment 2; "new" is also a reserved slug backend-side).
 */
export const load: PageServerLoad = async (event) => {
	assertDocEditor(event, 'api');

	const { statuses, existingPages } = await fetchAuthoringData(apiContext(event), 'api');

	const locale = event.locals.locale;
	const appName = tServer(locale, 'app.name', 'JapaConnect Docs');
	const audienceLabel = tServer(locale, 'audience.api', 'API');
	const title = `${tServer(locale, 'new_page.title', 'New page')} — ${audienceLabel} · ${appName}`;

	const meta = buildMeta({
		title,
		description: tServer(locale, 'new_page.description', 'Create a new documentation page.'),
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
