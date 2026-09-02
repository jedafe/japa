import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { Menu } from '$lib/types';

/**
 * Dashboard hub: loads the admin-configurable `dashboard_rail` menu so the
 * secondary left nav is API-driven and RBAC-gated (ADR-054, ADR-058).
 *
 * Ref: japaconnect-engineering/adr/ADR-054-navigation-menus.md
 * Ref: japaconnect-engineering/adr/ADR-058-main-app-shell.md
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const dashboardMenu = await apiGet<Menu>(ctx, '/menus/dashboard_rail').catch(() => null);
	return { dashboardMenu };
};
