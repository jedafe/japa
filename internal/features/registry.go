// Package features implements the feature flag system for JapaConnect.
//
// Every non-core platform feature is toggleable by admin via the feature_flags
// table. When a feature is disabled, its API routes return 404 (not 403, to
// avoid revealing the feature exists), and frontend nav items are hidden.
//
// Core features (auth, RBAC, organization scoping, basic content creation, media
// upload) are NOT feature-flagged - they always exist.
package features

// Feature represents a single feature flag definition.
type Feature struct {
	Key     string // e.g. "feature.blog"
	Label   string // Human-readable name for admin UI
	Default bool   // Default state: true = enabled, false = disabled
}

// Registry is the complete list of feature flags known to the platform.
// Every new feature added to JapaConnect must be registered here.
// A missing entry in the DB for a registered feature means it uses
// the Default value defined here.
var Registry = []Feature{
	// Content features
	{Key: "feature.blog", Label: "Blog", Default: true},
	{Key: "feature.forum", Label: "Forum / Q&A", Default: true},
	{Key: "feature.comparison", Label: "Comparison Pages", Default: true},
	{Key: "feature.academy", Label: "Japa Academy", Default: false},
	// Social features
	{Key: "feature.follow", Label: "Follow System", Default: true},
	{Key: "feature.dm", Label: "Direct Messages", Default: true},
	{Key: "feature.reactions", Label: "Reactions", Default: true},
	{Key: "feature.bookmarks", Label: "Bookmarks", Default: true},
	{Key: "feature.ratings_reviews", Label: "Ratings & Reviews", Default: true},
	{Key: "mentorship.enabled", Label: "Mentorship Matching", Default: false},
	{Key: "mentorship.comparison", Label: "Mentorship Comparison", Default: false},
	// Commerce features
	{Key: "feature.wallet", Label: "Wallet & Credits", Default: false},
	{Key: "feature.withdraw", Label: "Credit Withdrawals", Default: true},
	{Key: "feature.ads", Label: "Advertising", Default: false},
	{Key: "feature.referral", Label: "Referral Program", Default: false},
	{Key: "feature.affiliate_program", Label: "Affiliate Program", Default: false},
	// Tools
	{Key: "feature.tickets", Label: "Ticket System", Default: true},
	{Key: "feature.newsletter", Label: "Newsletter", Default: false},
	{Key: "feature.notifications", Label: "Notifications", Default: true},
	// AI features
	{Key: "feature.japabot", Label: "JapaBot AI Chat", Default: false},
	{Key: "feature.ai_content_gen", Label: "AI Content Generation", Default: false},
	{Key: "feature.ai_agent", Label: "AI Agent", Default: false},
	// Section 70 — Visa Document Checklist Generator (ADR-070).
	// Default true per ADR-070 §5 / §70 system_settings (checklist.enabled).
	// The feature_flags row is optional; when absent the registry default
	// applies, so the tool is reachable out-of-the-box and can be disabled
	// by setting the flag false in the control panel.
	{Key: "checklist.enabled", Label: "Visa Document Checklist", Default: true},
	// V2 Community Features (§79–§89, §98) — keys renamed by migration 000404
	// from feature.{name} to {domain}.enabled per conventions/feature-flag-naming.md.
	{Key: "story.enabled", Label: "Community Story Wall", Default: false},
	{Key: "wiki.enabled", Label: "Destination Wiki", Default: false},
	// §80 Community Field Ops — structured entity-field edits riding the
	// wiki revision queue. Requires wiki.enabled (routes nest inside its gate).
	{Key: "wiki.structured_edits", Label: "Wiki Structured Field Edits", Default: false},
	{Key: "scam_alert.enabled", Label: "Scam Alert Board", Default: false},
	{Key: "planning_group.enabled", Label: "Japa Planning Groups", Default: false},
	{Key: "visa_timeline.enabled", Label: "Visa Timeline Tracker", Default: false},
	{Key: "digest.enabled", Label: "Community Digest Widget", Default: false},
	{Key: "mutual_aid.enabled", Label: "Mutual Aid Board", Default: false},
	{Key: "calendar.enabled", Label: "Community Calendar", Default: false},
	{Key: "japa_stage.enabled", Label: "Japa Stage Progress Profiles", Default: false},
	{Key: "forum.ama.enabled", Label: "Ask an Expat AMA", Default: false},
	{Key: "achievement.enabled", Label: "Community Achievement Feed", Default: false},
	{Key: "service_directory.enabled", Label: "Service Directory", Default: false},
	{Key: "service_packages.enabled", Label: "Service Packages & Bookings", Default: false},
	// §113 Public Product Roadmap (ADR-127) — community-votable board +
	// changelog. Default on; all routes 404 via features.Gate when off.
	{Key: "roadmap.enabled", Label: "Public Product Roadmap", Default: true},
	// V2 Functional Tools (§90–§97, §99–§101) — Phase 49
	{Key: "visa_routes.enabled", Label: "Visa Route Comparison", Default: false},
	{Key: "appointment_alert.enabled", Label: "Embassy Appointment Alerter", Default: false},
	{Key: "arrival_checklist.enabled", Label: "Arrival Checklist", Default: false},
	{Key: "credential_checker.enabled", Label: "Professional Credential Checker", Default: false},
	{Key: "cv_formatter.enabled", Label: "CV Formatter", Default: false},
	{Key: "housing_guide.enabled", Label: "Housing Guide", Default: false},
	{Key: "attestation_guide.enabled", Label: "Attestation Guide", Default: false},
	{Key: "language_test_hub.enabled", Label: "Language Test Hub", Default: false},
	{Key: "tax_calculator.enabled", Label: "Tax Calculator", Default: false},
	{Key: "community_locator.enabled", Label: "Diaspora Community Locator", Default: false},
	// V1 Tools — originally toggled via system_settings, now unified under
	// feature_flags so the Control Panel Extensions page and the frontend
	// tool registries share a single source of truth.
	{Key: "assessment.enabled", Label: "Japa Readiness Assessment", Default: true},
	{Key: "col_calculator.enabled", Label: "Cost of Living Calculator", Default: true},
	{Key: "scholarship_finder.enabled", Label: "Scholarship Finder", Default: true},
	{Key: "remittance.enabled", Label: "Remittance Rate Tracker", Default: true},
	{Key: "fx.enabled", Label: "Currency Converter", Default: true},
	{Key: "travel.enabled", Label: "Travel Search", Default: true},
	{Key: "jobs.enabled", Label: "Jobs Board", Default: true},
	{Key: "gamification.enabled", Label: "Gamification", Default: true},
	{Key: "push.enabled", Label: "Mobile Push Notifications", Default: true},
	{Key: "geo_personalization.enabled", Label: "Geo Personalization", Default: true},
	// V2 AI / Infrastructure extensions
	{Key: "rag.enabled", Label: "RAG / JapaBot Knowledge", Default: false},
	{Key: "studio.enabled", Label: "AI Content Studio", Default: false},
	{Key: "participation.enabled", Label: "Participation Orchestration", Default: false},
	{Key: "webhooks.enabled", Label: "Webhooks", Default: false},
	// Japa Comparison Engine (JCE) — Sub-spec 1
	{Key: "feature.jce", Label: "Japa Comparison Engine", Default: true},
	{Key: "feature.jce.nested_items", Label: "JCE Nested Item References", Default: true},
	{Key: "feature.jce.ai_quick", Label: "JCE AI Quick Comparisons", Default: false},
}

// DefaultFor returns the default enabled state for a feature key.
// Returns false if the key is not in the registry (unknown features
// default to disabled for safety).
func DefaultFor(key string) bool {
	for _, f := range Registry {
		if f.Key == key {
			return f.Default
		}
	}

	return false
}

// AllKeys returns all registered feature flag keys.
func AllKeys() []string {
	keys := make([]string, len(Registry))
	for i, f := range Registry {
		keys[i] = f.Key
	}

	return keys
}

// registryLookup returns the Feature entry for a key and true if found,
// or a zero Feature and false if the key is not in the registry.
func registryLookup(key string) (Feature, bool) {
	for _, f := range Registry {
		if f.Key == key {
			return f, true
		}
	}

	return Feature{}, false
}
