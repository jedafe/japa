import { error } from '@sveltejs/kit';
import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '#lib/server/api.js';
import type { CurrencyInfo } from '#lib/utils/currency.js';
import type { Menu } from '#lib/types/index.js';

/** Onboarding status (Section 09) — only the tour dismissals are read. */
interface OnboardingStatusLite {
	tour_dismissed?: boolean;
	tour_dismissals?: Record<string, boolean>;
	tour_steps?: Record<string, string[]>;
}

// §58 — the active currency table changes rarely (Control → Currencies), so
// cache it briefly rather than re-fetching on every page load; helpers
// degrade to Intl symbols when it's unavailable.
const CURRENCIES_TTL_MS = 60_000;
let currenciesCache: { at: number; items: CurrencyInfo[] } | null = null;

async function loadCurrenciesCached(ctx: ReturnType<typeof apiContext>): Promise<CurrencyInfo[]> {
	if (currenciesCache && Date.now() - currenciesCache.at < CURRENCIES_TTL_MS) {
		return currenciesCache.items;
	}
	try {
		const items = (await apiGet<CurrencyInfo[]>(ctx, '/currencies')) ?? [];
		currenciesCache = { at: Date.now(), items };
		return items;
	} catch {
		return currenciesCache?.items ?? [];
	}
}

// Public shell catalogs (settings, feature flags, menus) are identical for
// every viewer on a host and change rarely — memoize briefly so repeat page
// loads skip the backend round-trips entirely.
const PUBLIC_TTL_MS = 60_000;
const publicCache = new Map<string, { at: number; value: unknown }>();

async function publicGetCached<T>(
	ctx: ReturnType<typeof apiContext>,
	path: string,
	fallback: T
): Promise<T> {
	const key = `${ctx.host}|${path}`;
	const hit = publicCache.get(key);
	if (hit && Date.now() - hit.at < PUBLIC_TTL_MS) return hit.value as T;
	try {
		const value = (await apiGet<T>(ctx, path)) ?? fallback;
		publicCache.set(key, { at: Date.now(), value });
		return value;
	} catch {
		return (hit?.value as T | undefined) ?? fallback;
	}
}

/**
 * Shared shell data for every Academy route: the resolved session (from hooks),
 * the academy navigation menu (ADR-054), and the platform logo. Each remote call
 * degrades gracefully — a failing widget must never blank the whole app.
 *
 * SSR/CSR boundary (ADR-060): only PUBLIC data is fetched here. Enrollment,
 * progress, quiz scores, and certificates are CSR-only (never in this load).
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const user = event.locals.user;

	// Every public catalog call is independent — fire them all in one wave
	// instead of paying a serial round-trip each (this alone was ~1.5s of
	// TTFB). Each degrades to its own fallback via publicGetCached.
	const sidebarPath = event.url.pathname;
	const siteSettingsP = publicGetCached<Record<string, unknown>>(ctx, '/site/settings/public', {});
	const featuresP = publicGetCached<Record<string, boolean>>(ctx, '/feature-flags', {});
	const menuP = publicGetCached<Menu | null>(ctx, '/menus/academy_sidebar', null);
	const footerMenuP = publicGetCached<Menu | null>(ctx, '/menus/academy_footer', null);
	const bottomMenuP = publicGetCached<Menu | null>(ctx, '/menus/academy_bottom_bar', null);
	const currenciesP = loadCurrenciesCached(ctx);
	const sidebarsP = Promise.all([
		apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=academy&position=right&path=${encodeURIComponent(sidebarPath)}`),
		apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=academy&position=left&path=${encodeURIComponent(sidebarPath)}`)
	]).catch(() => null);
	const unreadP = user
		? apiGet<{ count: number }>(ctx, '/me/notifications/unread-count').catch(() => null)
		: Promise.resolve(null);
	const tourP = user
		? apiGet<OnboardingStatusLite>(ctx, '/onboarding/status').catch(() => null)
		: Promise.resolve(null);

	// Feature flags (public endpoint) — fetched SSR so the layout can gate
	// client-only features on feature.dm without firing doomed requests when
	// the flag is off. Ref: sections/42-feature-flags.md
	// §42: the whole Academy surface is gated by feature.academy. When the
	// flag is off (or hidden on the academy surface) the app 404s rather
	// than rendering a shell whose API calls all fail. `=== false` (not
	// `!== true`) so a transient flags-fetch failure can't take the site down.
	const features = await featuresP;
	if (features['feature.academy'] === false) {
		throw error(404, { code: 'feature_disabled', message: 'This feature is not available' });
	}

	const siteSettings = await siteSettingsP;
	const menu = await menuP;
	const footerMenu = await footerMenuP;
	const bottomMenu = await bottomMenuP;
	const currencies = await currenciesP;

	const unreadRes = await unreadP;
	const unreadNotifications = unreadRes?.count ?? 0;

	// Section 09 Product Tour (academy surface): dismissal state for the
	// contextual walkthrough. Authed only — guests never see the tour.
	// Fail-closed on fetch error: an unreadable status means dismissal
	// can't persist either, so auto-running the tour would replay on
	// every page load. Explicit ?tour=replay still bypasses this.
	const tourStatus = await tourP;
	const tourDismissed = user ? (tourStatus?.tour_dismissals?.['academy'] ?? true) : false;
	const tourSteps = tourStatus?.tour_steps?.['academy'] ?? null;

	// Sitewide sidebar widgets (Sidebar Management System, Section 59, ADR-066).
	// Fetched SSR so the right sidebar paints during initial render. The resolve
	// endpoint matches the current page path against sidebar page_pattern globs
	// and filters widgets by the user's auth state and group membership.
	const sidebarRes = await sidebarsP;
	const rightSidebarWidgets = sidebarRes?.[0]?.widgets ?? [];
	const leftSidebarWidgets = sidebarRes?.[1]?.widgets ?? [];

	// §18 / ADR-013 — locales without a bundled dictionary are served from the
	// DB pack; the i18n store layers it over English so a missing key still
	// resolves to English text.
	const locale = event.locals.locale ?? 'en';
	const i18nPack = ['en', 'fr'].includes(locale)
		? null
		: await apiGet<Record<string, string>>(ctx, `/i18n/packs/${encodeURIComponent(locale)}`).catch(
				() => null
			);

	return {
		user,
		// Auth signal for client-side gates (e.g. the §124 SW auth report) —
		// never overridden by page-level `user` data the way a profile
		// record can shadow it.
		isAuthed: !!user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		i18nPack,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		features,
		menu,
		footerMenu,
		bottomMenu,
		unreadNotifications,
		currencies,
		rightSidebarWidgets,
		leftSidebarWidgets,
		tourDismissed,
		tourSteps,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		// Platform logo from appearance.logo_url / appearance.logo_url_dark (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? '',
		logoDarkUrl: (siteSettings['appearance.logo_url_dark'] as string | undefined) ?? '',
		// Server-render timestamp for hydration-stable time-derived output.
		serverNow: Date.now()
	};
};
