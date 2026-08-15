import type { LayoutServerLoad } from './$types';

/**
 * Shared shell data for every Tools route: the resolved session (from hooks),
 * the tool surface (hub vs single-tool), and the platform logo / site settings.
 * Each remote call degrades gracefully — a failing endpoint must never blank
 * the whole app.
 *
 * Ref: §78 (Tools Web Surface), ADR-086 (multi-surface architecture).
 */
export const load: LayoutServerLoad = async (event) => {
	const user = event.locals.user;

	// Public site settings (appearance/branding) — already fetched once in
	// hooks.server.ts and stored in event.locals.siteSettings. Reusing it here
	// avoids a redundant /site/settings/public fetch on every request.
	// Ref: §4.1 (performance), conventions/system-settings-catalog.md.
	const siteSettings = event.locals.siteSettings;

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		toolSurface: event.locals.toolSurface,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		features: event.locals.features,
		// Platform logo from appearance.logo_url (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? ''
	};
};
