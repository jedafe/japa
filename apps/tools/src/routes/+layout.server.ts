import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { CurrencyInfo } from '$lib/utils/currency';

// §58 — active currency list for symbol/name display on money-bearing
// tools (POF, ECA fees, remittance). Served by the public
// GET /api/v1/currencies reference endpoint; cached briefly so every page
// load doesn't re-hit the backend.
const CURRENCIES_TTL_MS = 60_000;
let currenciesCache: { at: number; items: CurrencyInfo[] } | null = null;

async function loadCurrenciesCached(event: Parameters<LayoutServerLoad>[0]): Promise<CurrencyInfo[]> {
	if (currenciesCache && Date.now() - currenciesCache.at < CURRENCIES_TTL_MS) {
		return currenciesCache.items;
	}
	try {
		const items = await apiGet<CurrencyInfo[]>(apiContext(event), '/currencies');
		currenciesCache = { at: Date.now(), items };
		return items;
	} catch {
		return currenciesCache?.items ?? [];
	}
}

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
	const currencies = await loadCurrenciesCached(event);

	// Public site settings (appearance/branding) — already fetched once in
	// hooks.server.ts and stored in event.locals.siteSettings. Reusing it here
	// avoids a redundant /site/settings/public fetch on every request.
	// Ref: §4.1 (performance), conventions/system-settings-catalog.md.
	const siteSettings = event.locals.siteSettings;

	// §18 / ADR-013 — locales without a bundled dictionary are served from the
	// DB pack; the i18n store layers it over English so a missing key still
	// resolves to English text.
	const locale = event.locals.locale ?? 'en';
	const i18nPack = ['en', 'fr'].includes(locale)
		? null
		: await apiGet<Record<string, string>>(
				apiContext(event),
				`/i18n/packs/${encodeURIComponent(locale)}`
			).catch(() => null);

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		i18nPack,
		toolSurface: event.locals.toolSurface,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		currencies,
		features: event.locals.features,
		// Platform logo from appearance.logo_url / appearance.logo_url_dark (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? '',
		logoDarkUrl: (siteSettings['appearance.logo_url_dark'] as string | undefined) ?? ''
	};
};
