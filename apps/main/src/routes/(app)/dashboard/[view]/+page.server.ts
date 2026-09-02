import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext } from '$lib/server/api';
import { resolveView, DASHBOARD_COMPONENT_SLUGS } from '$lib/components/hub/dashboard/dashboardViews';
import { DASHBOARD_LOADERS } from '$lib/components/hub/dashboard/dashboardLoaders.server';

/**
 * Dispatch to the view's server loader. 404s on unknown slugs or slugs not
 * present in the resolved dashboard_rail menu. The component registry is static
 * because Svelte components cannot be resolved from JSON; the menu controls
 * visibility and order.
 *
 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const parentData = await event.parent();
	const menuItems = parentData.dashboardMenu?.items;
	const slug = event.params.view;

	const meta = resolveView(slug, menuItems);
	// Only [view] component slugs are valid for this dynamic route; direct route
	// folders have their own +page.svelte and are not handled here.
	if (!meta || !meta.backendReady || !DASHBOARD_COMPONENT_SLUGS.has(slug)) {
		throw error(404, { code: 'not_found', message: 'Unknown dashboard view' });
	}

	const loader = DASHBOARD_LOADERS[slug];
	// Pass URL search params to the loader so views like content can filter
	// by status/type server-side (Published/Drafts/Scheduled tabs).
	const params: Record<string, string | undefined> = {};
	for (const [k, v] of event.url.searchParams) params[k] = v;
	const viewData = loader ? await loader(apiContext(event), params) : null;
	return { view: slug, viewData };
};
