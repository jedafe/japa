import type { LayoutServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet } from '$lib/server/api';
import { universityUrl } from '$lib/appOrigins';

/**
 * App-shell scoped data for every `(app)` route. This layout is re-run on
 * every SPA navigation because the right-sidebar widgets are path-dependent
 * (`/sidebars/resolve` needs the current pathname). Keeping the path-dependent
 * fetch here — instead of in the root `+layout.server.ts` — means the root
 * shell (user, menus, spaces, notifications, settings, flags) is fetched once
 * and reused across navigations, making route switches feel instant.
 */
export const load: LayoutServerLoad = async (event) => {
	if (event.locals.isUniversityHost) {
		throw redirect(302, universityUrl(`${event.url.pathname}${event.url.search}`));
	}

	const ctx = apiContext(event);

	// Sidebar widgets — path-dependent, always fetched per route.
	const sidebarData = await apiGet<{ sidebar?: { id?: string } | null; widgets?: unknown[] }>(
		ctx,
		`/sidebars/resolve?app=main&position=right&path=${encodeURIComponent(event.url.pathname)}`
	).catch(() => null);

	return {
		rightSidebarWidgets: sidebarData?.widgets ?? [],
		rightSidebarEnabled: sidebarData?.sidebar != null
	};
};
