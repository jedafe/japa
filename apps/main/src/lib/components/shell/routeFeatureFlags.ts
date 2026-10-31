/**
 * Maps frontend route paths to their feature flag key. When a feature flag is
 * disabled, any nav menu item whose URL matches a route in this map is hidden
 * from the LeftNav and MobileBottomTabBar so no trace of the toggled-off
 * feature leaks into the frontend.
 *
 * The map uses prefix matching: a nav item URL of `/stories/some-slug` matches
 * the `/stories` entry. Resolution goes through `routeFeatureFlagForPath` /
 * `routeFeatureGateForPath` (longest prefix wins) — never `.get()` directly.
 *
 * Value forms:
 *  - `string`        — single flag; the item shows only when it is enabled.
 *  - `string[]`      — OR semantics: visible when ANY listed flag is on. Used
 *    for composite surfaces like /messages/bookings, which serves directory
 *    and mentorship deployments alike (mirrors bookingsInboxEnabled).
 *  - `{ all: [...] }` — AND semantics: visible only when EVERY flag is on.
 *    Used where the backend stacks multiple Gates (e.g. /compare is gated by
 *    feature.comparison AND feature.jce).
 *
 * Only routes that correspond to a feature_flags key need to be listed here.
 * Core routes (home, search, profile, settings, spaces, tags, users) are never
 * feature-flagged and are intentionally absent.
 *
 * Ref: sections/42-feature-flags.md, internal/features/registry.go
 */

export type RouteFeatureFlag = string | string[] | { all: string[] };

export const routeFeatureFlags: Map<string, RouteFeatureFlag> = new Map<
	string,
	RouteFeatureFlag
>([
	// Content features
	['/blog', 'feature.blog'],
	['/forum', 'feature.forum'],
	// §14 list amendment — hides any seeded /lists nav item when the type is
	// off; /list covers the singular family for completeness.
	['/lists', 'list.enabled'],
	['/list', 'list.enabled'],
	// Social features
	['/notifications', 'feature.notifications'],
	// /radar (the follows/intel radar feed) is gated by feature.follow —
	// see the /radar routes in internal/server/server.go.
	['/radar', 'feature.follow'],
	// The bookings/mentorship inboxes live under /messages but are NOT
	// feature.dm surfaces — list them first so longest-prefix matching maps
	// them to their own flags (§111/000824). The bookings inbox is a
	// composite surface — OR-gated like bookingsInboxEnabled ($lib/features).
	['/messages/bookings', ['service_directory.enabled', 'mentorship.enabled']],
	['/messages/mentorship', 'mentorship.enabled'],
	['/messages', 'feature.dm'],
	['/bookmarks', 'feature.bookmarks'],
	['/dashboard/bookmarks', 'feature.bookmarks'],
	// Community Story Wall detail pages live under /story/{slug}; the index
	// is /stories — both are gated by story.enabled.
	['/stories', 'story.enabled'],
	['/story', 'story.enabled'],
	// Commerce features
	['/dashboard/wallet', 'feature.points'],
	['/settings/wallet', 'feature.points'],
	['/dashboard/affiliate', 'feature.affiliate_program'],
	['/dashboard/tickets', 'feature.tickets'],
	['/jobs', 'jobs.enabled'],
	['/me/applications', 'jobs.enabled'],
	['/donate', 'tips.enabled'],
	['/me/tips', 'tips.enabled'],
	// Coupons & deals surfaces — all backed by the service_coupons feature.
	['/deals', 'service_coupons.enabled'],
	['/deal', 'service_coupons.enabled'],
	['/coupons', 'service_coupons.enabled'],
	// /me surfaces: service bookings are the buyer-side bookings inbox —
	// OR-gated like /messages/bookings; saved items are calendar entries.
	['/me/service-bookings', ['service_directory.enabled', 'mentorship.enabled']],
	['/me/saved', 'calendar.enabled'],
	// JCE comparisons are gated by feature.comparison AND feature.jce on the
	// backend (stacked Gates) — mirror that with `all` so the nav item hides
	// when either flag is off.
	['/compare', { all: ['feature.comparison', 'feature.jce'] }],
	['/comparisons', { all: ['feature.comparison', 'feature.jce'] }],
	// Gamification (§64) — leaderboard + challenges share gamification.enabled.
	['/leaderboard', 'gamification.enabled'],
	['/challenges', 'gamification.enabled'],
	// Public roadmap board + changelog (§113 / ADR-127).
	['/roadmap', 'roadmap.enabled'],
	// AI Content Studio surface (§104-era studio routes).
	['/studio', 'studio.enabled'],
	['/studios', 'studio.enabled'],
	// Video content type surface — /video/{slug} detail pages.
	['/video', 'video.enabled'],
	// Tools — V1
	['/tools/readiness', 'assessment.enabled'],
	['/tools/cost-of-living', 'col_calculator.enabled'],
	['/scholarships', 'scholarship_finder.enabled'],
	['/tools/remittance', 'remittance.enabled'],
	['/tools/travel', 'travel.enabled'],
	// /ai is the unified AI surface (ADR-130) — gated by feature.japabot.
	// feature.ai_agent only controls the tool executor inside the surface.
	['/ai/chat', 'feature.japabot'],
	['/ai', 'feature.japabot'],
	['/tools/visa-checklist', 'checklist.enabled'],
	// Tools — V2 functional
	['/tools/visa-routes', 'visa_routes.enabled'],
	['/tools/embassy-alerter', 'appointment_alert.enabled'],
	['/tools/processing-tracker', 'visa_timeline.enabled'],
	['/tools/attestation-guide', 'attestation_guide.enabled'],
	['/tools/language-test', 'language_test_hub.enabled'],
	['/tools/credential-checker', 'credential_checker.enabled'],
	['/tools/cv-formatter', 'cv_formatter.enabled'],
	['/tools/tax-calculator', 'tax_calculator.enabled'],
	['/tools/arrival-checklist', 'arrival_checklist.enabled'],
	['/tools/housing-guide', 'housing_guide.enabled'],
	['/tools/community-locator', 'community_locator.enabled'],
	// Tools — §§107–110 SEO growth tools
	['/tools/crs-calculator', 'crs_calculator.enabled'],
	['/tools/pof-calculator', 'pof_calculator.enabled'],
	['/tools/eca-guide', 'eca_guide.enabled'],
	['/tools/sop-generator', 'sop_generator.enabled'],
	['/tools/currency-converter', 'fx.enabled'],
	// V2 Community features
	['/wiki', 'wiki.enabled'],
	['/scam-alerts', 'scam_alert.enabled'],
	['/planning-groups', 'planning_group.enabled'],
	['/visa-timelines', 'visa_timeline.enabled'],
	['/digest', 'digest.enabled'],
	['/mutual-aid', 'mutual_aid.enabled'],
	['/calendar', 'calendar.enabled'],
	['/japa-stage', 'japa_stage.enabled'],
	['/ama', 'forum.ama.enabled'],
	['/achievements', 'achievement.enabled'],
	// /services?kind=mentorship is the mentorship surface (what /mentors
	// redirects to) — OR-gated like the bookings inbox: visible when either
	// the directory or mentorship is enabled.
	['/services?kind=mentorship', ['service_directory.enabled', 'mentorship.enabled']],
	['/services', 'service_directory.enabled'],
	// Mentorship — /mentors 308-redirects into /services?kind=mentorship;
	// keep the nav item hidden unless mentorship itself is enabled.
	['/mentors', 'mentorship.enabled'],
	// Blog series detail pages share the blog gate.
	['/series', 'feature.blog'],
	// Academy — '/learn' is the legacy alias still present in older seeded
	// menus; '/certificates' is the academy certificate viewer.
	['/academy', 'feature.academy'],
	['/learn', 'feature.academy'],
	['/certificates', 'feature.academy'],
	// Top-level visa-checklist alias (canonical tool lives at
	// /tools/visa-checklist).
	['/visa-checklist', 'checklist.enabled'],
	// Newsletter
	['/me/notifications', 'feature.notifications']
]);

/**
 * Resolve the feature flag gate for a given path using prefix matching.
 * Returns the raw map value (string | string[] | {all:[...]}); undefined when
 * the path is not feature-flagged.
 */
export function routeFeatureFlagForPath(path: string): RouteFeatureFlag | undefined {
	// Try exact match first — this covers query-carrying routes that have
	// their own entry (e.g. '/services?kind=mentorship').
	const exact = routeFeatureFlags.get(path);
	if (exact) return exact;

	// Normalize: menu items may carry query strings ('/services?kind=…') or
	// absolute URLs (external hosts) — reduce to the path before matching.
	let normalized = path;
	const q = normalized.indexOf('?');
	if (q !== -1) normalized = normalized.slice(0, q);
	const h = normalized.indexOf('#');
	if (h !== -1) normalized = normalized.slice(0, h);
	if (normalized.startsWith('http://') || normalized.startsWith('https://')) {
		try {
			normalized = new URL(normalized).pathname;
		} catch {
			return undefined; // unparseable → treat as ungated
		}
	}
	// Re-check exact after normalization.
	const normExact = routeFeatureFlags.get(normalized);
	if (normExact) return normExact;

	// Try prefix match — longest prefix wins.
	let bestMatch: RouteFeatureFlag | undefined;
	let bestLen = 0;
	for (const [route, flag] of routeFeatureFlags) {
		if (normalized === route || normalized.startsWith(route + '/')) {
			if (route.length > bestLen) {
				bestMatch = flag;
				bestLen = route.length;
			}
		}
	}
	return bestMatch;
}

/**
 * Whether a nav/menu item pointing at `path` should render given the
 * /feature-flags map. Ungated paths always return true. Absent flag keys read
 * as disabled — matching the backend DefaultFor("unknown") = false contract
 * and the internal-flag projection (keys omitted from the map are falsy).
 */
export function routeFeatureGateVisible(
	path: string,
	features: Record<string, boolean>
): boolean {
	const gate = routeFeatureFlagForPath(path);
	if (!gate) return true;

	if (typeof gate === 'string') return features[gate] === true;
	if (Array.isArray(gate)) return gate.some((f) => features[f] === true);
	return gate.all.every((f) => features[f] === true);
}
