import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { fetchDocNav } from '$lib/server/nav';
import type { DocNavTree, Menu } from '$lib/types';

/**
 * Shared shell data for every docs route: the resolved session (from hooks),
 * the audience-filtered nav tree, and the accessible audiences.
 *
 * The nav tree is fetched once per request and filtered to accessibleAudiences
 * (server-side gating — ADR-060). Each remote call degrades gracefully.
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const allNav = await fetchDocNav(ctx);
	// Filter to audiences the session can access (server-side gating).
	const nav: DocNavTree[] = allNav.filter((tree: DocNavTree) =>
		event.locals.accessibleAudiences.includes(tree.audience)
	);

	// Public site settings (appearance/branding) — fetched SSR so the primary
	// color and logo are available before hydration.
	let siteSettings: Record<string, unknown> = {};
	try {
		siteSettings = (await apiGet<Record<string, unknown>>(ctx, '/site/settings/public')) ?? {};
	} catch {
		siteSettings = {};
	}

	// Footer menu — ADR-054 cross-app menu system. The docs footer slug is
	// `docs_footer`. Degrades to null if unseeded; GuestFooter renders nothing.
	let footerMenu: Menu | null = null;
	try {
		footerMenu = await apiGet<Menu>(ctx, '/menus/docs_footer');
	} catch {
		footerMenu = null;
	}

	return {
		user: event.locals.user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		accessibleAudiences: event.locals.accessibleAudiences,
		nav,
		siteSettings,
		footerMenu,
		navCollapsed: event.locals.user?.ui?.nav_collapsed ?? false
	};
};
