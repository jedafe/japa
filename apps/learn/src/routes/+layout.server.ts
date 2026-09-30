import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { CurrencyInfo } from '$lib/utils/currency';
import type { Menu } from '$lib/types';

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

/**
 * Shared shell data for every Learn route: the resolved session (from hooks),
 * the learn navigation menu (ADR-054), and the platform logo. Each remote call
 * degrades gracefully — a failing widget must never blank the whole app.
 *
 * SSR/CSR boundary (ADR-060): only PUBLIC data is fetched here. Enrollment,
 * progress, quiz scores, and certificates are CSR-only (never in this load).
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

	// Feature flags (public endpoint) — fetched SSR so the layout can gate
	// client-only features on feature.dm without firing doomed requests when
	// the flag is off. Ref: sections/42-feature-flags.md
	let features: Record<string, boolean> = {};
	try {
		const res = await apiGet<Record<string, boolean>>(ctx, '/feature-flags');
		features = res ?? {};
	} catch {
		features = {};
	}

	// Navigation menu — ADR-054 cross-app menu system. The Learn nav slug is
	// `learn_sidebar` (seeded by migration 000213 — Explore · My Learning ·
	// Certificates · Bookmarks · Instructor Dashboard). Degrades to null if
	// unseeded; the LeftNav renders nothing in that case.
	let menu: Menu | null = null;
	try {
		menu = await apiGet<Menu>(ctx, '/menus/learn_sidebar');
	} catch {
		menu = null;
	}

	// Footer menu — ADR-054 cross-app menu system. The Learn footer slug is
	// `learn_footer`. Degrades to null if unseeded; GuestFooter renders nothing.
	let footerMenu: Menu | null = null;
	try {
		footerMenu = await apiGet<Menu>(ctx, '/menus/learn_footer');
	} catch {
		footerMenu = null;
	}

	// Mobile bottom tab bar menu — ADR-054. The Learn bottom bar slug is
	// `learn_bottom_bar` (seeded by migration 000213).
	let bottomMenu: Menu | null = null;
	try {
		bottomMenu = await apiGet<Menu>(ctx, '/menus/learn_bottom_bar');
	} catch {
		bottomMenu = null;
	}

	// Active currency table (§58, ADR-065) — public reference endpoint shared
	// by every app so prices render the platform's managed names/symbols.
	// Degrades to [] — helpers fall back to Intl/browser symbols.
	const currencies = await loadCurrenciesCached(ctx);

	// Authed-only: unread notification count for the LeftNav badge. Seeded
	// server-side so the badge paints during SSR; the realtime store keeps it
	// live via WebSocket notification events.
	let unreadNotifications = 0;
	if (user) {
		try {
			const res = await apiGet<{ count: number }>(ctx, '/me/notifications/unread-count');
			unreadNotifications = res.count;
		} catch {
			unreadNotifications = 0;
		}
	}

	// Section 09 Product Tour (learn surface): dismissal state for the
	// contextual walkthrough. Authed only — guests never see the tour.
	// Fail-closed on fetch error: an unreadable status means dismissal
	// can't persist either, so auto-running the tour would replay on
	// every page load. Explicit ?tour=replay still bypasses this.
	let tourDismissed = false;
	let tourSteps: string[] | null = null;
	if (user) {
		try {
			const status = await apiGet<OnboardingStatusLite>(ctx, '/onboarding/status');
			tourDismissed = status?.tour_dismissals?.['learn'] === true;
			tourSteps = status?.tour_steps?.['learn'] ?? null;
		} catch {
			tourDismissed = true;
		}
	}

	// Sitewide sidebar widgets (Sidebar Management System, Section 59, ADR-066).
	// Fetched SSR so the right sidebar paints during initial render. The resolve
	// endpoint matches the current page path against sidebar page_pattern globs
	// and filters widgets by the user's auth state and group membership.
	let rightSidebarWidgets: unknown[] = [];
	let leftSidebarWidgets: unknown[] = [];
	try {
		const sidebarPath = event.url.pathname;
		const [rightRes, leftRes] = await Promise.all([
			apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=learn&position=right&path=${encodeURIComponent(sidebarPath)}`),
			apiGet<{ widgets?: unknown[] }>(ctx, `/sidebars/resolve?app=learn&position=left&path=${encodeURIComponent(sidebarPath)}`)
		]);
		rightSidebarWidgets = rightRes.widgets ?? [];
		leftSidebarWidgets = leftRes.widgets ?? [];
	} catch {
		// sidebar endpoint not available — keep defaults
	}

	return {
		user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		features,
		menu,
		bottomMenu,
		footerMenu,
		unreadNotifications,
		currencies,
		rightSidebarWidgets,
		leftSidebarWidgets,
		tourDismissed,
		tourSteps,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		// Platform logo from appearance.logo_url / appearance.logo_url_dark (control panel).
		logoUrl: (siteSettings['appearance.logo_url'] as string | undefined) ?? '',
		logoDarkUrl: (siteSettings['appearance.logo_url_dark'] as string | undefined) ?? ''
	};
};
