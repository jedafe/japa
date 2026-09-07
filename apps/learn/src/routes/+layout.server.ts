import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Menu } from '$lib/types';

/**
 * Shared shell data for every Learn route: the resolved session (from hooks),
 * the learn navigation menu (ADR-054), and the platform logo. Each remote call
 * degrades gracefully — a failing widget must never blank the whole app.
 *
 * SSR/CSR boundary (ADR-060): only PUBLIC data is fetched here. Enrollment,
 * progress, quiz scores, and certificates are CSR-only (never in this load).
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const user = event.locals.user;

	// Public site settings (appearance/branding) — fetched SSR so the primary
	// color and logo are available before hydration, preventing a flash of the
	// default theme. Ref: conventions/system-settings-catalog.md (Appearance).
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

	// Navigation menu — ADR-054 cross-app menu system. The Learn nav slug is
	// `learn_sidebar` (seeded by migration 000213 — Explore · My Learning ·
	// Certificates · Bookmarks · Instructor Dashboard). Degrades to null if
	// unseeded; the LeftNav renders nothing in that case.
	let menu: Menu | null = null;
	try {
		menu = await apiGet<Menu>(ctx, '/menus/learn_sidebar');
	} catch {
		menu = null;
	}

	// Footer menu — ADR-054 cross-app menu system. The Learn footer slug is
	// `learn_footer`. Degrades to null if unseeded; GuestFooter renders nothing.
	let footerMenu: Menu | null = null;
	try {
		footerMenu = await apiGet<Menu>(ctx, '/menus/learn_footer');
	} catch {
		footerMenu = null;
	}

	// Mobile bottom tab bar menu — ADR-054. The Learn bottom bar slug is
	// `learn_bottom_bar` (seeded by migration 000213).
	let bottomMenu: Menu | null = null;
	try {
		bottomMenu = await apiGet<Menu>(ctx, '/menus/learn_bottom_bar');
	} catch {
		bottomMenu = null;
	}

	// Authed-only: unread notification count for the LeftNav badge. Seeded
	// server-side so the badge paints during SSR; the realtime store keeps it
	// live via WebSocket notification events.
	let unreadNotifications = 0;
	if (user) {
		try {
			const res = await apiGet<{ count: number }>(ctx, '/me/notifications/unread-count');
			unreadNotifications = res.count;
		} catch {
			unreadNotifications = 0;
		}
	}

	// Sitewide sidebar widgets (Sidebar Management System, Section 59, ADR-066).
	// Fetched SSR so the right sidebar paints during initial render. The resolve
	// endpoint matches the current page path against sidebar page_pattern globs
	// and filters widgets by the user's auth state and group membership.
	let rightSidebarWidgets: unknown[] = [];
	let leftSidebarWidgets: unknown[] = [];
	try {
		const sidebarPath = event.url.pathname;
		const [rightRes, leftRes] = await Promise.all([
			apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=learn&position=right&path=${encodeURIComponent(sidebarPath)}`),
			apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=learn&position=left&path=${encodeURIComponent(sidebarPath)}`)
		]);
		rightSidebarWidgets = rightRes.widgets ?? [];
		leftSidebarWidgets = leftRes.widgets ?? [];
	} catch {
		// sidebar endpoint not available — keep defaults
	}

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		features,
		menu,
		bottomMenu,
		footerMenu,
		unreadNotifications,
		rightSidebarWidgets,
		leftSidebarWidgets,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		// Platform logo from appearance.logo_url (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? ''
	};
};
