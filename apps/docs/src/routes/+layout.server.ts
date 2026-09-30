import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { fetchDocNav } from '$lib/server/nav';
import type { DocNavTree, Menu } from '$lib/types';

/** Onboarding status (Section 09) — only the tour dismissals are read. */
interface OnboardingStatusLite {
	tour_dismissed?: boolean;
	tour_dismissals?: Record<string, boolean>;
	tour_steps?: Record<string, string[]>;
}

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

	// Feature flags (public endpoint) — fetched SSR so the layout can gate
	// client-only features on feature.dm without firing doomed requests when
	// the flag is off. Ref: sections/42-feature-flags.md
	let features: Record<string, boolean> = {};
	try {
		const res = await apiGet<Record<string, boolean>>(ctx, '/feature-flags');
		features = res ?? {};
	} catch {
		features = {};
	}

	// Footer menu — ADR-054 cross-app menu system. The docs footer slug is
	// `docs_footer`. Degrades to null if unseeded; GuestFooter renders nothing.
	let footerMenu: Menu | null = null;
	try {
		footerMenu = await apiGet<Menu>(ctx, '/menus/docs_footer');
	} catch {
		footerMenu = null;
	}

	// Sitewide sidebar widgets (Sidebar Management System, Section 59, ADR-066).
	// Fetched SSR so the right sidebar paints during initial render. The resolve
	// endpoint matches the current page path against sidebar page_pattern globs
	// and filters widgets by the user's auth state and group membership.
	let rightSidebarWidgets: unknown[] = [];
	let leftSidebarWidgets: unknown[] = [];
	// Settled independently — a failing left rail must not blank the right
	// sidebar (and vice versa).
	const sidebarPath = event.url.pathname;
	const [rightRes, leftRes] = await Promise.allSettled([
		apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=docs&position=right&path=${encodeURIComponent(sidebarPath)}`),
		apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=docs&position=left&path=${encodeURIComponent(sidebarPath)}`)
	]);
	if (rightRes.status === 'fulfilled') rightSidebarWidgets = rightRes.value.widgets ?? [];
	if (leftRes.status === 'fulfilled') leftSidebarWidgets = leftRes.value.widgets ?? [];

	// Section 09 Product Tour (docs surface): dismissal state for the
	// contextual walkthrough. Authed only — guests never see the tour.
	let tourDismissed = false;
	let tourSteps: string[] | null = null;
	if (event.locals.user) {
		try {
			const status = await apiGet<OnboardingStatusLite>(ctx, '/onboarding/status');
			tourDismissed = status?.tour_dismissals?.['docs'] === true;
			tourSteps = status?.tour_steps?.['docs'] ?? null;
		} catch {
			tourDismissed = false;
		}
	}

	return {
		user: event.locals.user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		accessibleAudiences: event.locals.accessibleAudiences,
		audienceGates: event.locals.audienceGates,
		nav,
		siteSettings,
		features,
		footerMenu,
		rightSidebarWidgets,
		leftSidebarWidgets,
		audienceOrder: event.locals.audienceOrder,
		navCollapsed: event.locals.navCollapsed,
		tourDismissed,
		tourSteps
	};
};
