import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { normalizeConfig, type CardStyleConfig } from '$lib/feed/cardStyle';
import type { Menu, OnboardingStatus, SpaceSuggestion, TrendingTag, UserSpaceRef } from '$lib/types';
import type { CurrencyInfo } from '$lib/utils/currency';

/**
 * Module-level caches for global, non-user-specific data that rarely changes.
 *
 * The root +layout.server.ts is intentionally independent of the current URL
 * and route. As long as it does not access event.url / event.params /
 * event.route, SvelteKit will not re-run this load on SPA navigation, so the
 * expensive shell data (settings, menus, spaces, notifications, flags) is
 * fetched once on the initial request and reused for every client-side route
 * change. This is what makes menu-nav clicks feel like fast SPA transitions
 * instead of full page reloads.
 *
 * Path-dependent data (right-sidebar widgets) lives in (app)/+layout.server.ts
 * so it can change per route without forcing the whole shell to reload.
 *
 * The caches are positive-only (misses aren't cached so the next request
 * retries immediately) and short-TTL so admin changes propagate within
 * a minute.
 */
const SETTINGS_TTL_MS = 60_000;
interface Cached<T> {
	value: T;
	expiresAt: number;
}
let settingsCache: Cached<Record<string, unknown>> | null = null;
let featureFlagsFallback: Record<string, boolean> | null = null;
// Active currency table (Section 58) — name/symbol lookup for money displays
// across every page. Same 60s TTL as settings: control-panel currency edits
// propagate within a minute.
let currenciesCache: Cached<CurrencyInfo[]> | null = null;

/**
 * Shared shell data for every route: the resolved session (from hooks), the
 * navigation menu, right-sidebar suggestions, and the user's joined spaces
 * (for the LeftNav Space switcher). Each remote call degrades gracefully —
 * a failing widget must never blank the whole app.
 *
 * All independent backend calls are fetched in parallel (Promise.all) rather
 * than serially.
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);
	const user = event.locals.user;
	const now = Date.now();

	// Register manual dependencies for shell data that can be invalidated
	// from the client (e.g. after joining/leaving a space). These do not make
	// the root load re-run on navigation, only on explicit invalidate(url).
	if (user) {
		event.depends('/api/v1/me/spaces');
	}

	// ── Global, cacheable data (served from module-level cache when fresh)
	// Site settings change rarely, so a 60s TTL avoids re-fetching them on
	// every navigation. Feature flags are fetched fresh on initial load so
	// admin toggles are reflected immediately; the backend Valkey cache makes
	// the call cheap.
	const settingsFromCache =
		settingsCache && settingsCache.expiresAt > now ? settingsCache.value : null;

	// ── Parallel fetch group: all independent calls fire at once.
	// Each resolves to a discriminated union so we can check success/failure
	// without try/catch around Promise.all (which would reject on any failure).
	const currenciesFromCache =
		currenciesCache && currenciesCache.expiresAt > now ? currenciesCache.value : null;

	const [
		settingsResult,
		currenciesResult,
		menuResult,
		footerMenuResult,
		mobileMenuResult,
		trendingTagsResult,
		suggestedSpacesResult,
		userSpacesResult,
		unreadResult,
		featuresResult,
		cardStyleResult,
		tourResult
	] = await Promise.all([
		// Site settings — from cache if fresh, otherwise fetch.
		settingsFromCache !== null
			? Promise.resolve({ ok: true as const, value: settingsFromCache, cached: true })
			: apiGet<Record<string, unknown>>(ctx, '/site/settings/public')
					.then((v) => ({ ok: true as const, value: v ?? ({} as Record<string, unknown>), cached: false }))
					.catch(() => ({ ok: false as const, value: {} as Record<string, unknown>, cached: false })),
		// Active currencies — public reference, cached like settings.
		currenciesFromCache !== null
			? Promise.resolve({ ok: true as const, value: currenciesFromCache, cached: true })
			: apiGet<CurrencyInfo[]>(ctx, '/currencies')
					.then((v) => ({ ok: true as const, value: v ?? [], cached: false }))
					.catch(() => ({ ok: false as const, value: [] as CurrencyInfo[], cached: false })),
		// Main sidebar menu — user-specific (RBAC filtered).
		apiGet<Menu>(ctx, '/menus/main_sidebar')
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({ ok: false as const, value: null as Menu | null })),
		// Footer menu — user-specific (RBAC filtered).
		apiGet<Menu>(ctx, '/menus/main_footer')
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({ ok: false as const, value: null as Menu | null })),
		// Mobile bottom-bar menu — admin-managed (bottom_bar placement);
		// MobileBottomTabBar falls back to the built-in tab set when absent.
		apiGet<Menu>(ctx, '/menus/mobile_bottom_bar')
			.then((v) => ({ ok: true as const, value: v }))
			.catch(() => ({ ok: false as const, value: null as Menu | null })),
		// Trending tags for the right sidebar — public /trending/tags endpoint
		// (Section 53) so the widget renders for guests too. Previously fetched
		// /suggestions/tags which is auth-only and serves a different purpose.
		apiList<TrendingTag>(ctx, '/trending/tags?limit=8')
			.then((v) => ({ ok: true as const, value: v.items }))
			.catch(() => ({ ok: false as const, value: [] as TrendingTag[] })),
		// Suggested spaces for the right sidebar — authed only. The endpoint
		// requires a session, so guests skip the request entirely instead of
		// paying a guaranteed 401 on every pageview.
		user
			? apiList<SpaceSuggestion>(ctx, '/suggestions/spaces?limit=5')
					.then((v) => ({ ok: true as const, value: v.items }))
					.catch(() => ({ ok: false as const, value: [] as SpaceSuggestion[] }))
			: Promise.resolve({ ok: true as const, value: [] as SpaceSuggestion[] }),
		// User's joined spaces — authed only. Request a small first page; the
		// left nav switcher only renders a handful and the full list is huge
		// for super admins.
		user
			? apiList<UserSpaceRef>(ctx, '/me/spaces?page_size=50&member_only=1')
					.then((v) => ({ ok: true as const, value: v.items }))
					.catch(() => ({ ok: false as const, value: [] as UserSpaceRef[] }))
			: Promise.resolve({ ok: true as const, value: [] as UserSpaceRef[] }),
		// Unread notification count — authed only.
		user
			? apiGet<{ count: number }>(ctx, '/me/notifications/unread-count')
					.then((v) => ({ ok: true as const, value: v.count }))
					.catch(() => ({ ok: false as const, value: 0 }))
			: Promise.resolve({ ok: true as const, value: 0 }),
		// Feature flags — fetched fresh on initial load so admin toggles
		// propagate. The backend caches flags in Valkey.
		apiGet<Record<string, boolean>>(ctx, '/feature-flags')
			.then((v) => ({ ok: true as const, value: v ?? {} }))
			.catch(() => ({ ok: false as const, value: {} as Record<string, boolean> })),
		// Feed card-style config — public; used by feed/list surfaces across
		// the (app) group.
		apiGet<unknown>(ctx, '/feed/card-style')
			.then((v) => ({ ok: true as const, value: normalizeConfig(v) }))
			.catch(() => ({ ok: false as const, value: { default: 'daily', byType: {} } as CardStyleConfig })),
		// Onboarding tour state — authed only. `tour_dismissed` guards the
		// ?tour=1 auto-trigger; `tour_steps.main` carries the admin-managed
		// step list (onboarding.tour_steps setting) for ProductTour (§09).
		user
			? apiGet<OnboardingStatus>(ctx, '/onboarding/status')
					.then((v) => ({
						ok: true as const,
						value: {
							dismissed: !!v?.tour_dismissed,
							steps: v?.tour_steps?.['main'] ?? null,
							completed: !!v?.completed,
							currentStep: v?.current_step ?? 1
						}
					}))
					.catch(() => ({
						ok: false as const,
						value: {
							dismissed: false,
							steps: null as string[] | null,
							completed: true,
							currentStep: 1
						}
					}))
			: Promise.resolve({
					ok: true as const,
					value: {
						dismissed: false,
						steps: null as string[] | null,
						completed: true,
						currentStep: 1
					}
				})
	]);

	// ── Apply results and populate caches for fresh (non-cached) fetches.
	let siteSettings = settingsResult.value;
	if (settingsResult.ok && !settingsResult.cached) {
		settingsCache = { value: siteSettings, expiresAt: now + SETTINGS_TTL_MS };
	} else if (!settingsResult.ok && settingsCache) {
		// Serve stale on fetch failure (transient backend blip).
		siteSettings = settingsCache.value;
	}

	let currencies = currenciesResult.value;
	if (currenciesResult.ok && !currenciesResult.cached) {
		currenciesCache = { value: currencies, expiresAt: now + SETTINGS_TTL_MS };
	} else if (!currenciesResult.ok && currenciesCache) {
		currencies = currenciesCache.value;
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
	const mobileMenu = mobileMenuResult.value;
	const trendingTags = trendingTagsResult.value;
	const suggestedSpaces = suggestedSpacesResult.value;
	const userSpaces = userSpacesResult.value;
	const unreadNotifications = unreadResult.value;

	const cardStyleConfig = cardStyleResult.ok
		? (cardStyleResult.value as CardStyleConfig)
		: { default: 'daily', byType: {} };
	const tourDismissed = tourResult.value.dismissed;
	const tourSteps = tourResult.value.steps;
	const onboardingCompleted = tourResult.value.completed;
	const onboardingStep = tourResult.value.currentStep;

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
		currencies,
		menu,
		footerMenu,
		mobileMenu,
		trendingTags,
		suggestedSpaces,
		userSpaces,
		navCollapsed: user?.ui?.nav_collapsed ?? false,
		unreadNotifications,
		features,
		cardStyleConfig,
		tourDismissed,
		tourSteps,
		onboardingCompleted,
		onboardingStep
	};
};
