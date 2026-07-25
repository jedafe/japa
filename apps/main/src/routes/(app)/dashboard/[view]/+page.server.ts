import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext } from '$lib/server/api';
import { resolveView } from '$lib/components/hub/dashboard/dashboardViews';
import { DASHBOARD_LOADERS } from '$lib/components/hub/dashboard/dashboardLoaders.server';

/**
 * Dispatch to the view's server loader. 404s on unknown slugs, views flagged
 * backendReady:false (e.g. `analytics`), and views whose component is not yet
 * built — the rail shows those disabled.
 *
 * Ref: japaconnect-engineering/sections/23-main-site-frontend.md (Dashboard hub)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required to view dashboard' });
	}

	const meta = resolveView(event.params.view);
	if (!meta || !meta.backendReady || !meta.component) {
		throw error(404, { code: 'not_found', message: 'Unknown dashboard view' });
	}

	const loader = DASHBOARD_LOADERS[meta.slug];
	const viewData = loader ? await loader(apiContext(event)) : null;
	return { view: meta.slug, viewData };
};
