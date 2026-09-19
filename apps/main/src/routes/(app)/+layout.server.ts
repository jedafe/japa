import type { LayoutServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet } from '$lib/server/api';
import { universityUrl } from '$lib/appOrigins';
import { isExceptionRoute } from '$lib/components/create/gate';

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

	// Sidebar widgets — path-dependent, always fetched per route. Both
	// positions resolve in parallel: the left rail renders when the side nav
	// is collapsed (Section 59, ADR-066).
	// Locale-prefixed URLs (/fr/studio) share the unprefixed route's sidebar
	// config — strip the active locale prefix before resolving (§18, ADR-013).
	const localePrefix =
		event.locals.locale && event.url.pathname.startsWith(`/${event.locals.locale}/`)
			? event.locals.locale.length + 1
			: 0;
	const sidebarPath = encodeURIComponent(
		localePrefix ? event.url.pathname.slice(localePrefix) : event.url.pathname
	);
	// ADR-058: the shell hides the right sidebar on every exception route
	// and zeroes left widgets on /studio, /create, /settings, /dashboard —
	// skip the resolve calls the layout would throw away anyway.
	const hideRight = isExceptionRoute(event.route.id);
	const rid = event.route.id ?? '';
	const hideLeft =
		rid.startsWith('/(app)/studio') ||
		rid === '/(app)/create' ||
		rid.startsWith('/(app)/settings') ||
		rid.startsWith('/(app)/dashboard');
	const [sidebarData, leftSidebarData] = await Promise.all([
		hideRight
			? null
			: apiGet<{ sidebar?: { id?: string } | null; widgets?: unknown[] }>(
					ctx,
					`/sidebars/resolve?app=main&position=right&path=${sidebarPath}`
				).catch((err) => {
					console.error('sidebar: right sidebar resolution failed', { path: event.url.pathname, err });
					return null;
				}),
		hideLeft
			? null
			: apiGet<{ sidebar?: { id?: string } | null; widgets?: unknown[] }>(
					ctx,
					`/sidebars/resolve?app=main&position=left&path=${sidebarPath}`
				).catch((err) => {
					console.error('sidebar: left sidebar resolution failed', { path: event.url.pathname, err });
					return null;
				})
	]);

	return {
		rightSidebarWidgets: sidebarData?.widgets ?? [],
		rightSidebarEnabled: sidebarData?.sidebar != null,
		leftSidebarWidgets: leftSidebarData?.widgets ?? []
	};
};
