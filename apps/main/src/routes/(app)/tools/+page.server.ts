import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '#lib/server/api.js';

/**
 * Tools hub landing page at /tools — the data-driven card grid for every
 * identity and functional tool (§67, §69–74, §§90–97, §§99–101).
 *
 * The tool list and per-tool enabled/disabled filtering live in
 * `x` and read `data.siteSettings` (provided by the root
 * layout) — disabled tools are hidden from the hub. Each tool's own page
 * additionally surfaces a disabled state when its backend handler returns 404.
 *
 * Ref: §78 (Tools Web Surface), conventions/system-settings-catalog.md.
 */
export const load: PageServerLoad = async (event) => {
	// Staff gate for the inline guide editor (same as /sources).
	const ctx = apiContext(event);
	const isSuperAdmin = (event.locals.user?.groups ?? []).some(
		(g) => g === 'Super Admin' || g === 'super_admin'
	);
	let canManage = isSuperAdmin;
	if (event.locals.user && !canManage) {
		try {
			const perms = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			canManage = (perms?.permissions ?? []).some((p) => p.startsWith('admin.settings'));
		} catch {
			canManage = false;
		}
	}
	return { canManage };
};
