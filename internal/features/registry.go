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
	Key         string // e.g. "feature.blog" — the feature_flags table key
	Label       string // Human-readable name for admin UI
	Description string // Short description shown on the extensions catalog
	Default     bool   // Default state: true = enabled, false = disabled
	// Category groups entries in the control /extensions UI. One of:
	// Content | Social | Commerce | Tools | AI | Engagement | Integrations | Platform
	Category string
	// SettingsCategory is the control-panel /settings?category=… slug the
	// entry's Configure link points at. Empty string = no settings page.
	SettingsCategory string
	// DefaultSurfaces maps a surface name ("main", "tools", "academy", "docs",
	// "mobile", "admin", "extension", "desktop") to its default visibility
	// state: "visible" | "hidden" | "disabled". Absent entries default to
	// visible everywhere; a nil map means all surfaces visible.
	DefaultSurfaces map[string]string
}

// Registry is the complete catalog of feature flags known to the platform.
// Every new feature added to JapaConnect must be registered here, and every
// key passed to features.Gate must have an entry (enforced by
// TestRegistryCoversAllGatedKeys). This is the single source of truth for the
// control panel extensions catalog — it absorbed the hardcoded
// knownExtensions list previously kept in internal/control/handlers.go.
//
// Default is the fresh-install default: it applies only when no
// feature_flags row exists. A stored row always wins, so operator installs
// keep whatever the baseline seed (000735 — a live-DB snapshot) or a later
// migration wrote. Several features are therefore seeded enabled while
// Default is false — deliberate divergence, pinned and documented by
// seedDefaultOverrides and TestRegistryMatchesBaselineSeed
// (registry_seed_test.go). Do not "fix" the drift by flipping a Default;
// decide which side is correct for fresh installs and update the allowlist.
var Registry = []Feature{
	// Content features
	{Key: "feature.blog", Label: "Blog", Description: "Blog series, content reviews, and featured posts", Default: true, Category: "Content", SettingsCategory: "content"},
	{Key: "feature.forum", Label: "Forum / Q&A", Description: "Community forum and question-and-answer boards", Default: true, Category: "Social"},
	{Key: "feature.comparison", Label: "Comparison Pages", Description: "Side-by-side comparison pages powered by the Japa Comparison Engine", Default: true, Category: "Tools"},
	{Key: "feature.academy", Label: "Japa Academy", Description: "Japa Academy learning platform surface", Default: false, Category: "Platform"},
	{Key: "feature.newsletter", Label: "Newsletter & Digest", Description: "Email newsletter subscriber management with weekly digests", Default: false, Category: "Content", SettingsCategory: "newsletter"},
	// Social features
	{Key: "feature.follow", Label: "Follow System", Description: "Follow users, organizations, and tags for personalized feeds", Default: true, Category: "Social"},
	{Key: "feature.dm", Label: "Direct Messages", Description: "Private direct messaging between members", Default: true, Category: "Social"},
	{Key: "feature.reactions", Label: "Reactions", Description: "Emoji reactions on content and comments", Default: true, Category: "Engagement"},
	{Key: "feature.bookmarks", Label: "Bookmarks", Description: "Save and organize content for later reading", Default: true, Category: "Engagement"},
	{Key: "feature.ratings_reviews", Label: "Ratings & Reviews", Description: "Star ratings and written reviews on rateable entities", Default: true, Category: "Engagement"},
	{Key: "mentorship.enabled", Label: "Mentorship Matching", Description: "Mentor/mentee matching between experienced members and newcomers", Default: false, Category: "Social"},
	{Key: "mentorship.comparison", Label: "Mentorship Comparison", Description: "Export selected mentors to a side-by-side JCE comparison", Default: false, Category: "Social"},
	// Commerce features
	{Key: "feature.wallet", Label: "Wallet & Credits", Description: "Member credit wallet, balances, and credit purchases", Default: false, Category: "Commerce"},
	{Key: "feature.withdraw", Label: "Credit Withdrawals", Description: "Enable user credit withdrawals and payouts", Default: true, Category: "Commerce"},
	{Key: "feature.ads", Label: "Self-Serve Advertising", Description: "Self-serve ad campaign creation and management platform", Default: true, Category: "Commerce", SettingsCategory: "ads"},
	{Key: "feature.referral", Label: "Referral Program", Description: "User referral program with credit rewards and attribution tracking", Default: false, Category: "Commerce", SettingsCategory: "referral"},
	{Key: "feature.affiliate_program", Label: "Affiliate Program", Description: "Affiliate tracking and commission management", Default: false, Category: "Commerce"},
	{Key: "jobs.enabled", Label: "Jobs Board", Description: "Job listings board with featured placement and credit-based promotion", Default: true, Category: "Commerce", SettingsCategory: "jobs"},
	// Tools
	{Key: "feature.tickets", Label: "Ticket System", Description: "Support ticket intake, triage, and resolution workflow", Default: true, Category: "Platform"},
	{Key: "feature.notifications", Label: "Notifications", Description: "In-app, email, and push notification delivery", Default: true, Category: "Platform"},
	// AI features
	{Key: "feature.japabot", Label: "JapaBot", Description: "RAG-grounded AI japa guide chatbot on the Tools surface", Default: false, Category: "AI", SettingsCategory: "japabot"},
	{Key: "feature.ai_content_gen", Label: "AI Assistant", Description: "AI-powered content assistance and moderation", Default: false, Category: "AI", SettingsCategory: "rag"},
	{Key: "feature.ai_agent", Label: "AI Agent", Description: "AI agent executor for multi-step tool-using workflows (§103)", Default: false, Category: "AI"},
	// Section 70 — Visa Document Checklist Generator (ADR-070).
	// Default true per ADR-070 §5 / §70 system_settings (checklist.enabled).
	// The feature_flags row is optional; when absent the registry default
	// applies, so the tool is reachable out-of-the-box and can be disabled
	// by setting the flag false in the control panel.
	{Key: "checklist.enabled", Label: "Visa Document Checklist", Description: "Interactive visa document checklist tool for immigrants", Default: true, Category: "Tools", SettingsCategory: "checklist"},
	// V2 Community Features (§79–§89, §98) — keys renamed by migration 000404
	// from feature.{name} to {domain}.enabled per conventions/feature-flag-naming.md.
	{Key: "story.enabled", Label: "Community Story Wall", Description: "Member-submitted immigration journey stories", Default: false, Category: "Social"},
	// Content-type killswitches — previously admin system_settings toggles;
	// unified under feature_flags so /extensions is the single control point.
	{Key: "video.enabled", Label: "Video Content", Description: "Video content type, channel import, and the /create composer", Default: true, Category: "Content"},
	{Key: "list.enabled", Label: "Listicles", Description: "List content type, /lists index, and the /create composer", Default: true, Category: "Content"},
	{Key: "wiki.enabled", Label: "Destination Wiki", Description: "Community-maintained destination knowledge wiki", Default: false, Category: "Content"},
	// §80 Community Field Ops — structured entity-field edits riding the
	// wiki revision queue. Requires wiki.enabled (routes nest inside its gate).
	{Key: "wiki.structured_edits", Label: "Wiki Structured Field Edits", Description: "Structured entity-field edits on wiki entries via the revision queue", Default: false, Category: "Content"},
	{Key: "scam_alert.enabled", Label: "Scam Alert Board", Description: "Community-reported scam alerts with verification workflow", Default: false, Category: "Social"},
	{Key: "planning_group.enabled", Label: "Japa Planning Groups", Description: "Cohort planning groups for members relocating together", Default: false, Category: "Social"},
	{Key: "visa_timeline.enabled", Label: "Visa Processing Time Tracker", Description: "Visa processing time statistics and trends — average, median, and percentile wait times", Default: false, Category: "Tools", SettingsCategory: "visa_timeline"},
	{Key: "digest.enabled", Label: "Community Digest Widget", Description: "Periodic digest of community activity surfaced on the feed", Default: false, Category: "Engagement"},
	{Key: "mutual_aid.enabled", Label: "Mutual Aid Board", Description: "Member-to-member offers and requests for help", Default: false, Category: "Social"},
	{Key: "calendar.enabled", Label: "Community Calendar", Description: "Shared community events and immigration deadline calendar", Default: false, Category: "Social"},
	{Key: "japa_stage.enabled", Label: "Japa Stage Progress Profiles", Description: "Stage-of-journey progress profiles on member accounts", Default: false, Category: "Social"},
	{Key: "forum.ama.enabled", Label: "Ask an Expat AMA", Description: "Scheduled ask-me-anything sessions hosted by verified expats", Default: false, Category: "Social"},
	{Key: "achievement.enabled", Label: "Community Achievement Feed", Description: "Milestone and achievement announcements in the community feed", Default: false, Category: "Engagement"},
	{Key: "service_directory.enabled", Label: "Service Directory", Description: "Verified immigrant-service listings with claim flow (§98)", Default: false, Category: "Commerce"},
	{Key: "service_packages.enabled", Label: "Service Packages & Bookings", Description: "Packaged service offerings with booking requests (§111)", Default: false, Category: "Commerce"},
	{Key: "service_coupons.enabled", Label: "Service Listing Coupons", Description: "Discount coupons attachable to service listings", Default: false, Category: "Commerce"},
	{Key: "service_packages.tiers_enabled", Label: "Service Package Tiers", Description: "Tiered pricing levels on service packages", Default: false, Category: "Commerce"},
	// §113 Public Product Roadmap (ADR-127) — community-votable board +
	// changelog. Default on; all routes 404 via features.Gate when off.
	{Key: "roadmap.enabled", Label: "Public Product Roadmap", Description: "Community-votable public roadmap board and changelog", Default: true, Category: "Platform"},
	// V2 Functional Tools (§90–§97, §99–§101) — Phase 49
	{Key: "visa_routes.enabled", Label: "Visa Route Comparison", Description: "Compare visa routes side by side — eligibility, fees, path to PR, and renewal terms", Default: false, Category: "Tools", SettingsCategory: "visa_routes"},
	{Key: "appointment_alert.enabled", Label: "Embassy Appointment Alerter", Description: "Subscribe to embassy appointment availability alerts and get notified when slots open", Default: false, Category: "Tools", SettingsCategory: "appointment_alert"},
	{Key: "arrival_checklist.enabled", Label: "Arrival Checklist", Description: "Phased post-arrival task checklist — registration, banking, and settling-in steps", Default: false, Category: "Tools", SettingsCategory: "arrival_checklist"},
	{Key: "credential_checker.enabled", Label: "Professional Credential Checker", Description: "Credential recognition pathways — regulatory body, timeline, cost, and steps", Default: false, Category: "Tools", SettingsCategory: "credential_checker"},
	{Key: "cv_formatter.enabled", Label: "CV Formatter", Description: "Destination-specific CV standards and AI-assisted formatting", Default: false, Category: "Tools", SettingsCategory: "cv_formatter"},
	{Key: "housing_guide.enabled", Label: "Housing Guide", Description: "Destination-specific housing guides — platforms, landlord requirements, rent ranges, and scam categories", Default: false, Category: "Tools", SettingsCategory: "housing_guide"},
	{Key: "attestation_guide.enabled", Label: "Attestation Guide", Description: "Step-by-step document attestation pathways — apostille requirements, steps, and fees", Default: false, Category: "Tools", SettingsCategory: "attestation_guide"},
	{Key: "language_test_hub.enabled", Label: "Language Test Hub", Description: "Required language tests for your visa — minimum scores, prep resources, and community score stories", Default: false, Category: "Tools", SettingsCategory: "language_test_hub"},
	{Key: "tax_calculator.enabled", Label: "Tax Calculator", Description: "Destination-specific tax guides — residency triggers, worldwide income rules, and double taxation treaties", Default: false, Category: "Tools", SettingsCategory: "tax_calculator"},
	{Key: "community_locator.enabled", Label: "Diaspora Community Locator", Description: "Find diaspora organisations near your destination — cultural groups, religious communities, and social clubs", Default: false, Category: "Tools", SettingsCategory: "community_locator"},
	// V2 SEO growth tools (§§107–110, ADR-118..121) — Tools surface
	{Key: "crs_calculator.enabled", Label: "CRS Calculator", Description: "Calculate your Comprehensive Ranking System score against the live IRCC ruleset, see the full factor breakdown, and track recent draw cut-offs", Default: true, Category: "Tools", SettingsCategory: "crs_calculator"},
	{Key: "pof_calculator.enabled", Label: "Proof of Funds Calculator", Description: "Work out the proof-of-funds requirement for your destination and family size — with live NGN and USD equivalents", Default: true, Category: "Tools", SettingsCategory: "pof_calculator"},
	{Key: "eca_guide.enabled", Label: "ECA Evaluation Guide", Description: "Compare credential evaluation providers, follow the step-by-step guide for your destination, and read real fee and timeline reports from members", Default: true, Category: "Tools", SettingsCategory: "eca_guide"},
	{Key: "sop_generator.enabled", Label: "AI SOP Generator", Description: "Generate a tailored statement of purpose draft from proven templates, browse real samples, and refine until it is ready", Default: true, Category: "Tools", SettingsCategory: "sop_generator"},
	// V1 Tools — originally toggled via system_settings, now unified under
	// feature_flags so the Control Panel Extensions page and the frontend
	// tool registries share a single source of truth.
	{Key: "assessment.enabled", Label: "Japa Readiness Assessment", Description: "AI-assisted readiness assessment with personalized roadmap", Default: true, Category: "Tools", SettingsCategory: "assessment"},
	{Key: "col_calculator.enabled", Label: "Cost of Living Calculator", Description: "Compare cost of living across destination cities", Default: true, Category: "Tools", SettingsCategory: "col_calculator"},
	{Key: "scholarship_finder.enabled", Label: "Scholarship Finder", Description: "Search and track scholarships with deadline reminders", Default: true, Category: "Tools", SettingsCategory: "scholarship_finder"},
	{Key: "remittance.enabled", Label: "Remittance Rate Tracker", Description: "Track and compare remittance exchange rates over time", Default: true, Category: "Tools", SettingsCategory: "remittance"},
	{Key: "fx.enabled", Label: "Currency Converter", Description: "Live currency conversion with NGN and destination-currency pairs", Default: true, Category: "Tools"},
	{Key: "travel.enabled", Label: "Travel Search", Description: "Travel search and booking integration for immigration journeys", Default: true, Category: "Tools", SettingsCategory: "travel"},
	{Key: "gamification.enabled", Label: "Gamification", Description: "Badges, challenges, and leaderboards for user engagement", Default: true, Category: "Engagement", SettingsCategory: "gamification"},
	// push.enabled default is false to match the legacy system_settings
	// semantics (off until FCM/APNs credentials are configured); migration
	// 000811 seeds an explicit row preserving the operator's setting.
	{Key: "push.enabled", Label: "Mobile Push Notifications", Description: "FCM and APNs push notification delivery for mobile apps", Default: false, Category: "Integrations", SettingsCategory: "push"},
	{Key: "geo_personalization.enabled", Label: "Geo Personalization", Description: "Geo-aware feed, search, and discovery ranking based on user location", Default: true, Category: "Integrations", SettingsCategory: "geo_personalization"},
	// V2 AI / Infrastructure extensions
	{Key: "rag.enabled", Label: "RAG / JapaBot Knowledge", Description: "Retrieval-augmented generation knowledge layer for JapaBot grounding", Default: false, Category: "AI", SettingsCategory: "rag"},
	{Key: "rag.spec_corpus", Label: "Engineering Spec Corpus", Description: "Indexes the .jeos spec tree into RAG as private spec_doc chunks for the staff-only spec.search agent tool (§121)", Default: false, Category: "AI", SettingsCategory: "rag"},
	{Key: "studio.enabled", Label: "AI Content Studio", Description: "AI-powered content generation studio with persona-driven jobs", Default: false, Category: "AI", SettingsCategory: "studio"},
	{Key: "participation.enabled", Label: "Participation Orchestration", Description: "Persona-driven participation orchestration engine", Default: false, Category: "AI", SettingsCategory: "participation"},
	{Key: "campaign.enabled", Label: "AI Content Campaigns", Description: "Admin-run AI content campaign planning and execution (§104)", Default: true, Category: "AI"},
	{Key: "intel.enabled", Label: "Intelligence System", Description: "Global toggle for the intelligence pipeline — radar feeds, source crawling, and promotion", Default: true, Category: "AI"},
	{Key: "webhooks.enabled", Label: "Webhooks", Description: "Outgoing webhook delivery system", Default: false, Category: "Integrations"},
	// Japa Comparison Engine (JCE) — Sub-spec 1
	{Key: "feature.jce", Label: "Japa Comparison Engine", Description: "Generic comparison engine powering side-by-side entity comparisons", Default: true, Category: "Tools", SettingsCategory: "jce"},
	{Key: "feature.jce.nested_items", Label: "JCE Nested Item References", Description: "Allow comparison items to reference nested sub-entities", Default: true, Category: "Tools", SettingsCategory: "jce"},
	{Key: "feature.jce.ai_quick", Label: "JCE AI Quick Comparisons", Description: "AI-generated quick comparison drafts on the compare surface", Default: false, Category: "AI", SettingsCategory: "jce"},
	// JCE Sub-spec 4 — data-source provenance, freshness, and refresh
	{Key: "feature.jce.data_sources", Label: "JCE Data Sources", Description: "Registered data sources with trust and freshness tracking for comparison values", Default: true, Category: "Tools", SettingsCategory: "jce"},
	{Key: "feature.jce.refresh", Label: "JCE Value Refresh", Description: "Manual and scheduled refresh of comparison values from registered data sources", Default: true, Category: "Tools", SettingsCategory: "jce"},
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

// RegistryLookup returns the Feature entry for a key and true if found,
// or a zero Feature and false if the key is not in the registry.
func RegistryLookup(key string) (Feature, bool) {
	for _, f := range Registry {
		if f.Key == key {
			return f, true
		}
	}

	return Feature{}, false
}
