import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { Menu, SpaceSuggestion, TrendingTag, UserSpaceRef } from '$lib/types';

/**
 * Module-level caches for global, non-user-specific data that rarely changes.
 *
 * The root +layout.server.ts re-runs on every SPA navigation (SvelteKit
 * fetches fresh data for the new route from the server). Without caching,
 * each navigation paid 9 serial backend round-trips just for shell data
 * that is identical between routes — making menu-nav clicks feel like full
 * page reloads while inline tab switches (which don't trigger a route
 * change) were instant.
 *
 * The caches are positive-only (misses aren't cached so the next request
 * retries immediately) and short-TTL so admin changes propagate within
 * a minute. Feature flags are an exception: they are fetched fresh on
 * every navigation so toggles take effect immediately. The backend caches
 * the flag lookup in Valkey, so the extra HTTP call is cheap.
 * User-specific data (menu, spaces, notifications) is always fetched
 * fresh but in parallel with the rest.
 */
const SETTINGS_TTL_MS = 60_000;
interface Cached<T> {
	value: T;
	expiresAt: number;
}
let settingsCache: Cached<Record<string, unknown>> | null = null;
let featureFlagsFallback: Record<string, boolean> | null = null;

/**
 * Shared shell data for every route: the resolved session (from hooks), the
 * navigation menu, right-sidebar suggestions, and the user's joined spaces
 * (for the LeftNav Space switcher). Each remote call degrades gracefully —
 * a failing widget must never blank the whole app.
 *
 * All independent backend calls are fetched in parallel (Promise.all) rather
 * than serially. Previously these were 9 sequential awaits, each blocking
 * the next — making every SPA navigation wait for 9 serial round-trips
 * before the page could render. Now they all fire at once and the layout
 * load completes in roughly one round-trip instead of nine.
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const user = event.locals.user;
	const now = Date.now();

	// ── Global, cacheable data (served from module-level cache when fresh)
	// Site settings change rarely, so a 60s TTL avoids re-fetching them on
	// every navigation. Feature flags are fetched fresh so admin toggles are
	// reflected immediately; the backend Valkey cache makes the call cheap.
	const settingsFromCache =
		settingsCache && settingsCache.expiresAt > now ? settingsCache.value : null;

	// ── Parallel fetch group: all independent calls fire at once.
	// Each resolves to a discriminated union so we can check success/failure
	// without try/catch around Promise.all (which would reject on any failure).
	const [
		settingsResult,
		menuResult,
		footerMenuResult,
		trendingTagsResult,
		suggestedSpacesResult,
		sidebarResult,
		userSpacesResult,
		unreadResult,
		featuresResult
	] = await Promise.all([
		// Site settings — from cache if fresh, otherwise fetch.
		settingsFromCache !== null
			? Promise.resolve({ ok: true as const, value: settingsFromCache, cached: true })
			: apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
					.then((v) => ({ ok: true as const, value: v ?? ({} as Record<string, unknown>), cached: false }))
					.catch(() => ({ ok: false as const, value: {} as Record<string, unknown>, cached: false })),
		// Main sidebar menu — user-specific (RBAC filtered), always fetched.
		apiGet<Menu>(ctx, '/menus/main_sidebar')
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({ ok: false as const, value: null as Menu | null })),
		// Footer menu — user-specific (RBAC filtered), always fetched.
		apiGet<Menu>(ctx, '/menus/main_footer')
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({ ok: false as const, value: null as Menu | null })),
		// Trending tags for the right sidebar — public /trending/tags endpoint
		// (Section 53) so the widget renders for guests too. Previously fetched
		// /suggestions/tags which is auth-only and serves a different purpose.
		apiList<TrendingTag>(ctx, '/trending/tags?limit=8')
			.then((v) => ({ ok: true as const, value: v.items }))
			.catch(() => ({ ok: false as const, value: [] as TrendingTag[] })),
		// Suggested spaces for the right sidebar.
		apiList<SpaceSuggestion>(ctx, '/suggestions/spaces?limit=5')
			.then((v) => ({ ok: true as const, value: v.items }))
			.catch(() => ({ ok: false as const, value: [] as SpaceSuggestion[] })),
		// Sidebar widgets — path-dependent, always fetched.
		apiGet<{ sidebar?: { id?: string } | null; widgets?: unknown[] }>(
			ctx,
			`/sidebars/resolve?app=main&position=right&path=${encodeURIComponent(event.url.pathname)}`
		)
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({
				ok: false as const,
				value: null as { sidebar?: { id?: string } | null; widgets?: unknown[] } | null
			})),
		// User's joined spaces — authed only.
		user
			? apiList<UserSpaceRef>(ctx, '/me/spaces')
					.then((v) => ({ ok: true as const, value: v.items }))
					.catch(() => ({ ok: false as const, value: [] as UserSpaceRef[] }))
			: Promise.resolve({ ok: true as const, value: [] as UserSpaceRef[] }),
		// Unread notification count — authed only.
		user
			? apiGet<{ count: number }>(ctx, '/me/notifications/unread-count')
					.then((v) => ({ ok: true as const, value: v.count }))
					.catch(() => ({ ok: false as const, value: 0 }))
			: Promise.resolve({ ok: true as const, value: 0 }),
		// Feature flags — fetched fresh every navigation so admin toggles
		// propagate immediately. The backend caches flags in Valkey.
		apiGet<Record<string, boolean>>(ctx, '/feature-flags')
			.then((v) => ({ ok: true as const, value: v ?? {} }))
			.catch(() => ({ ok: false as const, value: {} as Record<string, boolean> }))
	]);

	// ── Apply results and populate caches for fresh (non-cached) fetches.
	let siteSettings = settingsResult.value;
	if (settingsResult.ok && !settingsResult.cached) {
		settingsCache = { value: siteSettings, expiresAt: now + SETTINGS_TTL_MS };
	} else if (!settingsResult.ok && settingsCache) {
		// Serve stale on fetch failure (transient backend blip).
		siteSettings = settingsCache.value;
	}

	let features = featuresResult.value;
	if (featuresResult.ok) {
		featureFlagsFallback = features;
	} else if (featureFlagsFallback) {
		// Serve last-known flags on transient backend blip.
		features = featureFlagsFallback;
	}

	const menu = menuResult.value;
	const footerMenu = footerMenuResult.value;
	const trendingTags = trendingTagsResult.value;
	const suggestedSpaces = suggestedSpacesResult.value;
	const sidebarData = sidebarResult.value;
	const userSpaces = userSpacesResult.value;
	const unreadNotifications = unreadResult.value;

	const rightSidebarWidgets = sidebarData?.widgets ?? [];
	const rightSidebarEnabled = sidebarData?.sidebar != null;

	return {
		user,
		// Dedicated auth flag — page-level load functions may return their own
		// `user` key (e.g. the profile page returns the viewed profile's user),
		// which overrides `user` in page.data. This boolean is only set by the
		// root layout and never overridden, so client-side auth guards can rely
		// on it regardless of which page is loaded.
		isAuthed: !!user,
		theme: event.locals.theme,
		locale: event.locals.locale,
		isUniversityHost: event.locals.isUniversityHost,
		csrfToken: event.locals.csrfToken ?? null,
		siteSettings,
		menu,
		footerMenu,
		trendingTags,
		suggestedSpaces,
		userSpaces,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		unreadNotifications,
		features,
		rightSidebarWidgets,
		rightSidebarEnabled
	};
};
