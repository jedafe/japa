import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

/**
 * Shared shell data for every Tools route: the resolved session (from hooks),
 * the tool surface (hub vs single-tool), and the platform logo / site settings.
 * Each remote call degrades gracefully — a failing endpoint must never blank
 * the whole app.
 *
 * Ref: §78 (Tools Web Surface), ADR-086 (multi-surface architecture).
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

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		toolSurface: event.locals.toolSurface,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		// Platform logo from appearance.logo_url (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? ''
	};
};
