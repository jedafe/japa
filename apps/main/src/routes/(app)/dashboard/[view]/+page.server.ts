import { error, redirect } from '@sveltejs/kit';
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
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const meta = resolveView(event.params.view);
	if (!meta || !meta.backendReady || !meta.component) {
		throw error(404, { code: 'not_found', message: 'Unknown dashboard view' });
	}

	const loader = DASHBOARD_LOADERS[meta.slug];
	// Pass URL search params to the loader so views like content can filter
	// by status/type server-side (Published/Drafts/Scheduled tabs).
	const params: Record<string, string | undefined> = {};
	for (const [k, v] of event.url.searchParams) params[k] = v;
	const viewData = loader ? await loader(apiContext(event), params) : null;
	return { view: meta.slug, viewData };
};
