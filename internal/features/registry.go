// Package features implements the feature flag system for JapaConnect.
//
// Every non-core platform feature is toggleable by admin via the feature_flags
// table. When a feature is disabled, its API routes return 404 (not 403, to
// avoid revealing the feature exists), and frontend nav items are hidden.
//
// Core features (auth, RBAC, multi-tenancy, basic content creation, media
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
	{Key: "feature.learning", Label: "Learning Platform", Default: false},
	// Social features
	{Key: "feature.follow", Label: "Follow System", Default: true},
	{Key: "feature.dm", Label: "Direct Messages", Default: true},
	{Key: "feature.reactions", Label: "Reactions", Default: true},
	{Key: "feature.bookmarks", Label: "Bookmarks", Default: true},
	{Key: "feature.ratings_reviews", Label: "Ratings & Reviews", Default: true},
	{Key: "feature.mentorship", Label: "Mentorship Matching", Default: false},
	{Key: "feature.mentorship_comparison", Label: "Mentorship Comparison", Default: false},
	// Professional features
	{Key: "feature.professional", Label: "Professional Profiles", Default: true},
	// Commerce features
	{Key: "feature.wallet", Label: "Wallet & Credits", Default: false},
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
	// V2 Community Features (§79–§89, §98)
	{Key: "feature.story", Label: "Community Story Wall", Default: false},
	{Key: "feature.wiki", Label: "Destination Wiki", Default: false},
	{Key: "feature.scam_alert", Label: "Scam Alert Board", Default: false},
	{Key: "feature.planning_group", Label: "Japa Planning Groups", Default: false},
	{Key: "feature.visa_timeline", Label: "Visa Timeline Tracker", Default: false},
	{Key: "feature.digest", Label: "Community Digest Widget", Default: false},
	{Key: "mutual_aid.enabled", Label: "Mutual Aid Board", Default: false},
	{Key: "feature.calendar", Label: "Community Calendar", Default: false},
	{Key: "feature.japa_stage", Label: "Japa Stage Progress Profiles", Default: false},
	{Key: "feature.ama", Label: "Ask an Expat AMA", Default: false},
	{Key: "feature.achievement", Label: "Community Achievement Feed", Default: false},
	{Key: "feature.service_directory", Label: "Service Directory", Default: false},
	// V2 Functional Tools (§90–§97, §99–§101) — Phase 49
	{Key: "feature.visa_routes", Label: "Visa Route Comparison", Default: false},
	{Key: "feature.appointment_alert", Label: "Embassy Appointment Alerter", Default: false},
	{Key: "feature.arrival_checklist", Label: "Arrival Checklist", Default: false},
	{Key: "feature.credential_checker", Label: "Professional Credential Checker", Default: false},
	{Key: "feature.cv_formatter", Label: "CV Formatter", Default: false},
	{Key: "feature.housing_guide", Label: "Housing Guide", Default: false},
	{Key: "feature.attestation_guide", Label: "Attestation Guide", Default: false},
	{Key: "feature.language_test_hub", Label: "Language Test Hub", Default: false},
	{Key: "feature.tax_calculator", Label: "Tax Calculator", Default: false},
	{Key: "feature.community_locator", Label: "Diaspora Community Locator", Default: false},
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
