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

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		isUniversityHost: event.locals.isUniversityHost,
		siteSettings,
		menu,
		trendingTags,
		suggestedSpaces,
		userSpaces,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		unreadNotifications
	};
};
