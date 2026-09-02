import type { HubGroup, Menu, MenuItem } from '$lib/types';

/**
 * Client-side view metadata for a dashboard rail item. The authoritative list
 * comes from `GET /api/v1/menus/dashboard_rail`; this file only provides the
 * shape and helpers to map that API payload to the hub rail.
 *
 * `isRoute: true` marks entries that are separate route folders (e.g.
 * /dashboard/tickets) rather than `[view]` slugs — they have no component but
 * are linked directly from the rail.
 *
 * Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md
 */
export interface ViewMeta {
	slug: string;
	label: string;
	icon: string;
	backendReady: boolean;
	isRoute: boolean;
}

export const DEFAULT_VIEW = 'overview';

// Slugs that have a `[view]` component. Must match keys in dashboardComponents.ts.
export const DASHBOARD_COMPONENT_SLUGS: Set<string> = new Set([
	'overview',
	'analytics',
	'content',
	'relationships',
	'learning',
	'certificates',
	'knowledge-base'
]);

/**
 * Resolve the last path segment of a dashboard URL into a slug.
 * '/dashboard/overview' -> 'overview'; '/dashboard/overview?tab=...' -> 'overview'.
 */
export function dashboardSlug(url: string): string {
	try {
		const path = new URL(url, 'http://localhost').pathname;
		return path.split('/').pop() ?? '';
	} catch {
		return '';
	}
}

export function viewMetaFromMenuItem(item: MenuItem): ViewMeta | null {
	const slug = dashboardSlug(item.url);
	if (!slug) return null;
	const isRoute = !DASHBOARD_COMPONENT_SLUGS.has(slug);
	return {
		slug,
		label: item.label,
		icon: item.icon ?? 'circle',
		backendReady: item.status === 'active',
		isRoute
	};
}

export function resolveView(slug: string, menuItems?: MenuItem[]): ViewMeta | null {
	if (menuItems) {
		const item = menuItems.find((i) => dashboardSlug(i.url) === slug);
		if (!item) return null;
		return viewMetaFromMenuItem(item);
	}
	// Without a menu, fall back to the hardcoded component slugs. This path is
	// used by the [view] server loader when the menu payload is unavailable.
	if (!DASHBOARD_COMPONENT_SLUGS.has(slug)) return null;
	return {
		slug,
		label: `dashboard.nav.${slug}`,
		icon: 'circle',
		backendReady: true,
		isRoute: false
	};
}

export function viewsByGroup(menu: Menu | null): [HubGroup, ViewMeta[]][] {
	if (!menu?.items?.length) return [];
	const group = (menu.name || 'Activity') as HubGroup;
	const views = menu.items
		.filter((item) => !item.is_ad_slot && item.status === 'active' && item.url)
		.map(viewMetaFromMenuItem)
		.filter((v): v is ViewMeta => v != null);
	return views.length ? [[group, views]] : [];
}
