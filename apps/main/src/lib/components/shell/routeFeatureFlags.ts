/**
 * Maps frontend route paths to their feature flag key. When a feature flag is
 * disabled, any nav menu item whose URL matches a route in this map is hidden
 * from the LeftNav and MobileBottomTabBar so no trace of the toggled-off
 * feature leaks into the frontend.
 *
 * The map uses prefix matching: a nav item URL of `/stories/some-slug` matches
 * the `/stories` entry. The check is performed in LeftNav via
 * `routeFeatureFlags.get(item.url)` (exact match) — for prefix matching, use
 * `routeFeatureFlagForPath(path)`.
 *
 * A route may map to an array of flags — OR semantics: the nav item stays
 * visible when ANY listed flag is enabled. Used for composite surfaces like
 * /messages/bookings, which serves directory and mentorship deployments
 * alike (mirrors bookingsInboxEnabled in $lib/features).
 *
 * Only routes that correspond to a feature_flags key need to be listed here.
 * Core routes (home, search, profile, settings, spaces, tags, users) are never
 * feature-flagged and are intentionally absent.
 *
 * Ref: sections/42-feature-flags.md, internal/features/registry.go
 */

export const routeFeatureFlags: Map<string, string | string[]> = new Map<
	string,
	string | string[]
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
	// The bookings/mentorship inboxes live under /messages but are NOT
	// feature.dm surfaces — list them first so longest-prefix matching maps
	// them to their own flags (§111/000824). The bookings inbox is a
	// composite surface — OR-gated like bookingsInboxEnabled ($lib/features).
	['/messages/bookings', ['service_directory.enabled', 'mentorship.enabled']],
	['/messages/mentorship', 'mentorship.enabled'],
	['/messages', 'feature.dm'],
	// Commerce features
	['/dashboard/wallet', 'feature.wallet'],
	['/dashboard/affiliate', 'feature.affiliate_program'],
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
	// V2 Community features
	['/stories', 'story.enabled'],
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
	['/services', 'service_directory.enabled'],
	// Mentorship — /mentors 308-redirects into /services?kind=mentorship;
	// keep the nav item hidden unless mentorship itself is enabled.
	['/mentors', 'mentorship.enabled'],
	// Academy
	['/academy', 'feature.academy'],
	// Newsletter
	['/me/notifications', 'feature.notifications']
]);

/**
 * Resolve the feature flag(s) for a given path using prefix matching.
 * Returns a single flag key or an array (OR semantics — see the map
 * docstring); undefined when the path is not feature-flagged.
 */
export function routeFeatureFlagForPath(path: string): string | string[] | undefined {
	// Try exact match first.
	const exact = routeFeatureFlags.get(path);
	if (exact) return exact;

	// Try prefix match — longest prefix wins.
	let bestMatch: string | string[] | undefined;
	let bestLen = 0;
	for (const [route, flag] of routeFeatureFlags) {
		if (path === route || path.startsWith(route + '/')) {
			if (route.length > bestLen) {
				bestMatch = flag;
				bestLen = route.length;
			}
		}
	}
	return bestMatch;
}
