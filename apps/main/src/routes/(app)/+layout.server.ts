import type { LayoutServerLoad } from './$types';
import { redirect } from '@sveltejs/kit';
import { apiContext, apiGet } from '$lib/server/api';
import { academyUrl } from '$lib/appOrigins';

/**
 * App-shell scoped data for every `(app)` route. This layout is re-run on
 * every SPA navigation because the right-sidebar widgets are path-dependent
 * (`/sidebars/resolve` needs the current pathname). Keeping the path-dependent
 * fetch here — instead of in the root `+layout.server.ts` — means the root
 * shell (user, menus, spaces, notifications, settings, flags) is fetched once
 * and reused across navigations, making route switches feel instant.
 */
export const load: LayoutServerLoad = async (event) => {
	if (event.locals.isAcademyHost) {
		throw redirect(302, academyUrl(`${event.url.pathname}${event.url.search}`));
	}

	// ADR-127: the roadmap host serves only the public roadmap surface.
	// Allowlist /roadmap/* plus /login (vote/submit auth flow) and /api/* —
	// every other path redirects to the board. Locale-prefixed paths
	// (/fr/roadmap) are checked against their unprefixed form; the locale
	// cookie keeps the session language across the redirect.
	if (event.locals.isRoadmapHost) {
		const pathname = event.url.pathname;
		const unprefixed =
			event.locals.locale && pathname.startsWith(`/${event.locals.locale}/`)
				? pathname.slice(event.locals.locale.length + 1)
				: pathname;
		if (
			!unprefixed.startsWith('/roadmap') &&
			!unprefixed.startsWith('/login') &&
			!unprefixed.startsWith('/api')
		) {
			throw redirect(302, '/roadmap');
		}
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
	// ADR-058: /studio and /create render no right sidebar at all, so their
	// resolve call is skipped. The Exception #1 singles (blog, content,
	// guide, story, comparisons, video) hide the shell's sidebar but render
	// their own in-page <aside> fed by rightSidebarWidgets — skipping the
	// resolve left those permanently empty. (/video/[slug]'s lean
	// sourceOnly player page has no aside and simply ignores the data.)
	// /roadmap is a wide board surface: the §113 kanban columns cannot fit
	// the ~600px center column, so the board index reclaims the sidebar
	// column (same hideRightSidebar path as /studio) instead of fetching
	// widgets that would never render. Roadmap subpages (item detail,
	// changelog, submit) keep the normal shell.
	const rid = event.route.id ?? '';
	const hideRight =
		rid.startsWith('/(app)/studio') ||
		rid === '/(app)/create' ||
		rid === '/(app)/roadmap';
	const hideLeft =
		rid.startsWith('/(app)/studio') ||
		rid === '/(app)/create' ||
		rid.startsWith('/(app)/settings') ||
		rid.startsWith('/(app)/dashboard') ||
		rid === '/(app)/roadmap';
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
		leftSidebarWidgets: leftSidebarData?.widgets ?? [],
		// User's explicit pin on forced-collapse (exception) routes — stored in
		// a cookie (not localStorage) so SSR renders the same nav width as the
		// client; localStorage reads post-mount caused a post-hydration reflow.
		navPinOverride: event.cookies.get('nav_pin_override') === '1',
		// Server-render timestamp — client components render time-dependent
		// values (footers, relative times) from this so SSR and hydration
		// produce identical output instead of tick-over mismatches.
		serverNow: Date.now()
	};
};
