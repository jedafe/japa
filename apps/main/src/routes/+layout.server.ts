import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Menu, SpaceSuggestion, TagSuggestion, UserSpaceRef } from '$lib/types';

/**
 * Shared shell data for every route: the resolved session (from hooks), the
 * navigation menu, right-sidebar suggestions, and the user's joined spaces
 * (for the LeftNav Space switcher). Each remote call degrades gracefully —
 * a failing widget must never blank the whole app.
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

	let menu: Menu | null = null;
	try {
		// The skill calls this "primary_nav"; the seeded tenant menu slug is main_sidebar.
		menu = await apiGet<Menu>(ctx, '/menus/main_sidebar');
	} catch {
		menu = null;
	}

	let footerMenu: Menu | null = null;
	try {
		footerMenu = await apiGet<Menu>(ctx, '/menus/main_footer');
	} catch {
		footerMenu = null;
	}

	let trendingTags: TagSuggestion[] = [];
	try {
		trendingTags = (await apiList<TagSuggestion>(ctx, '/suggestions/tags?limit=8')).items;
	} catch {
		trendingTags = [];
	}

	let suggestedSpaces: SpaceSuggestion[] = [];
	try {
		suggestedSpaces = (await apiList<SpaceSuggestion>(ctx, '/suggestions/spaces?limit=5')).items;
	} catch {
		suggestedSpaces = [];
	}

	// Sitewide sidebar widgets (Sidebar Management System, Section 59, ADR-066).
	// Fetched SSR so the right sidebar paints during initial render. The resolve
	// endpoint matches the current page path against sidebar page_pattern globs
	// and filters widgets by the user's auth state, group membership, and
	// permission_node. When no sidebar matches, `sidebar` is null and the right
	// column is hidden (Section 59 §4).
	//
	// Note: the Main app's left column is the navigation menu (ADR-058), not
	// admin-managed widgets, so we do not resolve a left sidebar here.
	let rightSidebarWidgets: unknown[] = [];
	let rightSidebarEnabled = true;
	const sidebarPath = event.url.pathname;
	try {
		const rightRes = await apiGet<{ sidebar?: { id?: string } | null; widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=main&position=right&path=${encodeURIComponent(sidebarPath)}`);
		rightSidebarWidgets = rightRes.widgets ?? [];
		rightSidebarEnabled = rightRes.sidebar != null;
	} catch {
		// sidebar endpoint not available — keep defaults (enabled, empty)
	}

	// Authed-only: the user's joined spaces for the LeftNav Space switcher.
	let userSpaces: UserSpaceRef[] = [];
	if (user) {
		try {
			userSpaces = (await apiList<UserSpaceRef>(ctx, '/me/spaces')).items;
		} catch {
			userSpaces = [];
		}
	}

	// Authed-only: unread notification count for the LeftNav badge (Phase 23).
	// Seeded server-side so the badge paints during SSR; the realtime store
	// keeps it live via WebSocket notification events.
	let unreadNotifications = 0;
	if (user) {
		try {
			const res = await apiGet<{ count: number }>(ctx, '/me/notifications/unread-count');
			unreadNotifications = res.count;
		} catch {
			unreadNotifications = 0;
		}
	}

	// Feature flags (public endpoint) — fetched SSR so the layout can gate
	// client-only features like the WebSocket realtime connection on
	// feature.dm without firing doomed requests when the flag is off.
	// Ref: sections/42-feature-flags.md, sections/10-realtime-websockets.md
	let features: Record<string, boolean> = {};
	try {
		const res = await apiGet<Record<string, boolean>>(ctx, '/feature-flags');
		features = res ?? {};
	} catch {
		features = {};
	}

	return {
		user,
		// Dedicated auth flag — page-level load functions may return their own
		// `user` key (e.g. the profile page returns the viewed profile's user),
		// which overrides `user` in page.data. This boolean is only set by the
		// root layout and never overridden, so client-side auth guards can rely
		// on it regardless of which page is loaded.
		isAuthed: !!user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		isUniversityHost: event.locals.isUniversityHost,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		menu,
		footerMenu,
		trendingTags,
		suggestedSpaces,
		userSpaces,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		unreadNotifications,
		features,
		rightSidebarWidgets,
		rightSidebarEnabled
	};
};
