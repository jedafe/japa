import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Menu, SpaceSuggestion, TagSuggestion } from '$lib/types';

/**
 * Shared shell data for every route: the resolved session (from hooks), the
 * navigation menu, and right-sidebar suggestions. Each remote call degrades
 * gracefully — a failing widget must never blank the whole app.
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const user = event.locals.user;

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

	return {
		user,
		theme: event.locals.theme,
		menu,
		trendingTags,
		suggestedSpaces,
		navCollapsed: user?.ui?.nav_collapsed ?? false
	};
};
