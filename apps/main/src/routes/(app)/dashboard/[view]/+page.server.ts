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
	let slug = event.params.view;

	// Legacy slug: /dashboard/learning was renamed to /dashboard/academy.
	if (slug === 'learning') {
		throw redirect(308, `/dashboard/academy${event.url.search}`);
	}

	const meta = resolveView(slug, menuItems);
	// Only [view] component slugs are valid for this dynamic route; direct route
	// folders have their own +page.svelte and are not handled here.
	if (!meta || !meta.backendReady || !DASHBOARD_COMPONENT_SLUGS.has(slug)) {
		throw error(404, { code: 'not_found', message: 'Unknown dashboard view' });
	}

	// apiGet/apiList go through ctx.fetch with an absolute backend URL, so a
	// client-side invalidate('/api/v1/me/...') can never match a registered
	// fetch dependency. Declaring the keys here lets the relationships view's
	// unfollow/leave/unblock/unmute actions actually re-run this load.
	// Ref: routes/(app)/u/[username]/+page.server.ts (same pattern).
	for (const dep of [
		'/api/v1/me/following',
		'/api/v1/me/followed-tags',
		'/api/v1/me/spaces',
		'/api/v1/me/blocked-users',
		'/api/v1/me/muted-users',
		'/api/v1/me/muted-spaces'
	]) {
		event.depends(dep);
	}

	const loader = DASHBOARD_LOADERS[slug];
	// Pass URL search params to the loader so views can read filters
	// server-side (e.g. analytics' ?period=/start/end).
	const params: Record<string, string | undefined> = {};
	for (const [k, v] of event.url.searchParams) params[k] = v;
	const viewData = loader ? await loader(apiContext(event), params) : null;
	return { view: slug, viewData };
};
