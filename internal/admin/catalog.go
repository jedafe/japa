package admin

import (
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// CatalogOption is a single value/label pair returned by the admin catalog
// endpoint. The admin frontend uses these to populate select/filter dropdowns
// instead of hardcoded arrays (Rule 16).
type CatalogOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// adminCatalogs is the source of truth for domain-specific option lists used
// in the admin panel. Each catalog is keyed by a dot-separated name of the
// form `<entity>.<option>` (e.g. "spaces.status", "tickets.priority").
// Values that are static domain enums live here; values that are per-tenant
// are merged at request time.
var adminCatalogs = map[string][]CatalogOption{
	"spaces.status": {
		{Value: "active", Label: "Active"},
		{Value: "suspended", Label: "Suspended"},
		{Value: "archived", Label: "Archived"},
	},
	"spaces.level": {
		{Value: "1", Label: "1"},
		{Value: "2", Label: "2"},
		{Value: "3", Label: "3"},
	},
	"scholarships.verification_status": {
		{Value: "verified", Label: "Verified"},
		{Value: "needs_review", Label: "Needs Review"},
		{Value: "conflict", Label: "Conflict"},
		{Value: "stale", Label: "Stale"},
	},
	"tools.visa_routes.status": {
		{Value: "draft", Label: "Draft"},
		{Value: "ai_draft", Label: "AI Draft"},
		{Value: "pending", Label: "Pending"},
		{Value: "published", Label: "Published"},
		{Value: "rejected", Label: "Rejected"},
	},
	"tools.credential_checker.status": {
		{Value: "", Label: "All"},
		{Value: "ai_draft", Label: "AI Draft"},
		{Value: "pending", Label: "Pending"},
		{Value: "published", Label: "Published"},
		{Value: "rejected", Label: "Rejected"},
		{Value: "draft", Label: "Draft"},
	},
	"tools.arrival_checklist.status": {
		{Value: "", Label: "All"},
		{Value: "draft", Label: "Draft"},
		{Value: "ai_draft", Label: "AI Draft"},
		{Value: "pending", Label: "Pending"},
		{Value: "published", Label: "Published"},
		{Value: "rejected", Label: "Rejected"},
	},
	"university.tracks.difficulty": {
		{Value: "beginner", Label: "Beginner"},
		{Value: "intermediate", Label: "Intermediate"},
		{Value: "advanced", Label: "Advanced"},
	},
	"university.instructor.social_platforms": {
		{Value: "twitter", Label: "Twitter"},
		{Value: "linkedin", Label: "LinkedIn"},
		{Value: "website", Label: "Website"},
		{Value: "youtube", Label: "YouTube"},
	},
	"university.tracks.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "approved", Label: "Approved"},
		{Value: "rejected", Label: "Rejected"},
	},
	"university.payouts.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "processing", Label: "Processing"},
		{Value: "paid", Label: "Paid"},
		{Value: "failed", Label: "Failed"},
	},
	"university.instructors.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "approved", Label: "Approved"},
		{Value: "rejected", Label: "Rejected"},
		{Value: "suspended", Label: "Suspended"},
	},
	"university.revenue_splits.source": {
		{Value: "purchase", Label: "Purchase"},
		{Value: "pro_stream", Label: "Pro Stream"},
	},
	"community.mutual_aid.status": {
		{Value: "active", Label: "Active"},
		{Value: "fulfilled", Label: "Fulfilled"},
		{Value: "removed", Label: "Removed"},
		{Value: "expired", Label: "Expired"},
	},
	"community.story_wall.status": {
		{Value: "draft", Label: "Draft"},
		{Value: "pending", Label: "Pending"},
		{Value: "published", Label: "Published"},
	},
	"community.achievements.status": {
		{Value: "active", Label: "Active"},
		{Value: "inactive", Label: "Inactive"},
		{Value: "pending", Label: "Pending"},
	},
	"community.scam_alerts.status": {
		{Value: "confirmed", Label: "Confirmed"},
		{Value: "debunked", Label: "Debunked"},
		{Value: "duplicate", Label: "Duplicate"},
		{Value: "under_review", Label: "Under review"},
	},
	"community.digest.status": {
		{Value: "draft", Label: "Draft"},
		{Value: "review", Label: "Review"},
		{Value: "published", Label: "Published"},
		{Value: "cancelled", Label: "Cancelled"},
	},
	"community.services.status": {
		{Value: "draft", Label: "Draft"},
		{Value: "published", Label: "Published"},
		{Value: "pending", Label: "Pending"},
		{Value: "archived", Label: "Archived"},
	},
	"media.mime_type": {
		{Value: "image/", Label: "Images"},
		{Value: "video/", Label: "Videos"},
		{Value: "audio/", Label: "Audio"},
		{Value: "application/", Label: "Documents"},
	},
	"media.status": {
		{Value: "approved", Label: "Approved"},
		{Value: "pending", Label: "Pending"},
		{Value: "quarantined", Label: "Quarantined"},
		{Value: "rejected", Label: "Rejected"},
	},
	"tickets.priority": {
		{Value: "low", Label: "Low"},
		{Value: "normal", Label: "Normal"},
		{Value: "high", Label: "High"},
		{Value: "urgent", Label: "Urgent"},
	},
	"tickets.status": {
		{Value: "open", Label: "Open"},
		{Value: "in_progress", Label: "In Progress"},
		{Value: "waiting_on_user", Label: "Waiting on User"},
		{Value: "resolved", Label: "Resolved"},
		{Value: "closed", Label: "Closed"},
		{Value: "spam", Label: "Spam"},
	},
	"travel.provider_type": {
		{Value: "flight", Label: "Flights"},
		{Value: "hotel", Label: "Hotels"},
		{Value: "both", Label: "Flights & Hotels"},
	},
	"gamification.trigger_type": {
		{Value: "lifetime_points", Label: "Lifetime Points"},
		{Value: "activity_count", Label: "Activity Count"},
		{Value: "manual", Label: "Manual"},
	},
	"data_exports.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "processing", Label: "Processing"},
		{Value: "completed", Label: "Completed"},
		{Value: "failed", Label: "Failed"},
	},
	"data_exports.type": {
		{Value: "users", Label: "Users"},
		{Value: "posts", Label: "Posts"},
		{Value: "comments", Label: "Comments"},
		{Value: "orders", Label: "Orders"},
		{Value: "transactions", Label: "Transactions"},
		{Value: "analytics", Label: "Analytics"},
		{Value: "moderation", Label: "Moderation Logs"},
	},
	"data_exports.format": {
		{Value: "csv", Label: "CSV"},
		{Value: "json", Label: "JSON"},
	},
	"data_imports.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "processing", Label: "Processing"},
		{Value: "completed", Label: "Completed"},
		{Value: "partial", Label: "Partial"},
		{Value: "failed", Label: "Failed"},
	},
	"data_imports.type": {
		{Value: "users", Label: "Users"},
		{Value: "posts", Label: "Posts"},
		{Value: "comments", Label: "Comments"},
		{Value: "orders", Label: "Orders"},
		{Value: "transactions", Label: "Transactions"},
		{Value: "analytics", Label: "Analytics"},
		{Value: "moderation", Label: "Moderation Logs"},
	},
	"data_imports.format": {
		{Value: "csv", Label: "CSV"},
		{Value: "json", Label: "JSON"},
	},
	"data_imports.mode": {
		{Value: "create", Label: "Create (skip existing)"},
		{Value: "upsert", Label: "Upsert (update or insert)"},
	},
	"affiliate.status": {
		{Value: "active", Label: "Active"},
		{Value: "suspended", Label: "Suspended"},
		{Value: "banned", Label: "Banned"},
	},
	"affiliate.action_taken": {
		{Value: "flagged", Label: "Flagged"},
		{Value: "blocked", Label: "Blocked"},
		{Value: "approved", Label: "Approved"},
	},
	"affiliate.credited": {
		{Value: "true", Label: "Credited"},
		{Value: "false", Label: "Pending"},
	},
	"reports.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "reviewing", Label: "Reviewing"},
		{Value: "actioned", Label: "Actioned"},
		{Value: "dismissed", Label: "Dismissed"},
	},
	"reports.entity_type": {
		{Value: "content", Label: "Content"},
		{Value: "comment", Label: "Comment"},
		{Value: "user", Label: "User"},
		{Value: "space", Label: "Space"},
		{Value: "message", Label: "Message"},
	},
	"moderation.appeals.status": {
		{Value: "pending", Label: "Pending"},
		{Value: "upheld", Label: "Upheld"},
		{Value: "overturn", Label: "Overturn"},
		{Value: "dismissed", Label: "Dismissed"},
	},
	"moderation.alerts.severity": {
		{Value: "low", Label: "Low"},
		{Value: "medium", Label: "Medium"},
		{Value: "high", Label: "High"},
		{Value: "critical", Label: "Critical"},
	},
	"moderation.alerts.status": {
		{Value: "open", Label: "Open"},
		{Value: "acknowledged", Label: "Acknowledged"},
		{Value: "resolved", Label: "Resolved"},
	},
	"intelligence.review.status": {
		{Value: "review_required", Label: "Review Required"},
		{Value: "extracted", Label: "Extracted"},
		{Value: "approved", Label: "Approved"},
		{Value: "rejected", Label: "Rejected"},
	},
	"intelligence.review.entity_type": {
		{Value: "scholarship", Label: "Scholarship"},
		{Value: "job", Label: "Job"},
		{Value: "visa", Label: "Visa"},
		{Value: "housing", Label: "Housing"},
		{Value: "remittance", Label: "Remittance"},
		{Value: "cost_of_living", Label: "Cost of Living"},
		{Value: "travel_routes", Label: "Travel Routes"},
		{Value: "grants", Label: "Grants"},
		{Value: "admission", Label: "Admission"},
		{Value: "banking", Label: "Banking"},
		{Value: "work_career", Label: "Work & Career"},
		{Value: "health_insurance", Label: "Health Insurance"},
		{Value: "culture_lifestyle", Label: "Culture & Lifestyle"},
		{Value: "wiki_page", Label: "Wiki Page"},
		{Value: "language_test_requirement", Label: "Language Test Requirement"},
		{Value: "tax_obligation_guide", Label: "Tax Obligation Guide"},
		{Value: "community_organisation", Label: "Community Organisation"},
	},
}

// CatalogOptions returns a defensive copy of the options for a named catalog.
// It is exported so other packages (e.g. the university marketplace) can expose
// the same source of truth without duplicating option lists (Rule 16).
func CatalogOptions(name string) ([]CatalogOption, bool) {
	opts, ok := adminCatalogs[name]
	if !ok {
		return nil, false
	}

	out := make([]CatalogOption, len(opts))
	copy(out, opts)

	return out, true
}

// catalogKeys returns the list of registered catalog names, sorted.
func catalogKeys() []string {
	keys := make([]string, 0, len(adminCatalogs))
	for k := range adminCatalogs {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// WidgetConfigField describes a single config field for a widget type.
type WidgetConfigField struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Type    string          `json:"type"`
	Options []CatalogOption `json:"options,omitempty"`
	Default any             `json:"default,omitempty"`
	Help    string          `json:"help,omitempty"`
}

// CardTypeOption is a single card-style choice with the icon/desc metadata the
// admin widgets card-type picker needs.
type CardTypeOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Icon  string `json:"icon"`
	Desc  string `json:"desc"`
}

// WidgetConfigOptions is the Rule-16-compliant source of truth for the widget
// type list, per-type config schemas, and card type choices used in
// apps/admin/widgets.
type WidgetConfigOptions struct {
	WidgetTypes        []CatalogOption                `json:"widget_types"`
	ConfigSchemas      map[string][]WidgetConfigField `json:"config_schemas"`
	CardTypes          []CardTypeOption               `json:"card_types"`
	ContentWidgetTypes []string                       `json:"content_widget_types"`
}

// widgetConfigCatalog holds the canonical widget type list and per-type
// config field schemas. Content type fields use a placeholder "All types"
// option because the frontend fetches the canonical content types from
// GET /content-types and injects them at runtime.
var widgetConfigCatalog = WidgetConfigOptions{
	WidgetTypes: []CatalogOption{
		{Value: "latest_content", Label: "Latest Content"},
		{Value: "trending_tags", Label: "Trending Tags"},
		{Value: "space_members", Label: "Space Members"},
		{Value: "user_leaderboard", Label: "User Leaderboard"},
		{Value: "upcoming_events", Label: "Upcoming Events"},
		{Value: "featured_spaces", Label: "Featured Spaces"},
		{Value: "custom_html", Label: "Custom HTML"},
		{Value: "featured_content", Label: "Featured Content (admin-pinned)"},
		{Value: "blog_posts", Label: "Blog Posts"},
		{Value: "forum_threads", Label: "Forum Threads"},
		{Value: "job_listings", Label: "Job Listings"},
		{Value: "achievements", Label: "Achievements"},
		{Value: "learning_tracks", Label: "Learning Tracks"},
		{Value: "community_digest", Label: "Community Digest"},
		{Value: "reviews", Label: "Reviews"},
		{Value: "comparisons", Label: "Comparisons"},
		{Value: "community_organisations", Label: "Community Organisations"},
		{Value: "mentor_spotlight", Label: "Mentor Spotlight"},
		{Value: "newsletter_signup", Label: "Newsletter Signup"},
	},
	ConfigSchemas: map[string][]WidgetConfigField{
		"latest_content": {
			{Key: "content_type", Label: "Content Type", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All types"}}, Help: "Filter by content type. Leave as \"All types\" for mixed content."},
			{Key: "sort", Label: "Sort", Type: "select", Default: "recent", Options: []CatalogOption{{Value: "recent", Label: "Most Recent"}, {Value: "popular", Label: "Most Viewed"}, {Value: "discussed", Label: "Most Discussed"}}},
			{Key: "featured", Label: "Featured Only", Type: "checkbox", Default: false, Help: "Show only pinned/featured content."},
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5, Help: "Max items to return."},
		},
		"featured_content": {
			{Key: "content_type", Label: "Content Type", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All types"}}, Help: "Filter by content type."},
			{Key: "sort", Label: "Sort", Type: "select", Default: "featured", Options: []CatalogOption{{Value: "featured", Label: "Featured Order (admin-curated)"}, {Value: "recent", Label: "Most Recent"}, {Value: "popular", Label: "Most Viewed"}, {Value: "discussed", Label: "Most Discussed"}}},
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"blog_posts": {
			{Key: "sort", Label: "Sort", Type: "select", Default: "recent", Options: []CatalogOption{{Value: "recent", Label: "Most Recent"}, {Value: "popular", Label: "Most Viewed"}, {Value: "discussed", Label: "Most Discussed"}}},
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"forum_threads": {
			{Key: "sort", Label: "Sort", Type: "select", Default: "hot", Options: []CatalogOption{{Value: "hot", Label: "Hot (vote score)"}, {Value: "recent", Label: "Most Recent"}, {Value: "unanswered", Label: "Unanswered"}}},
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"job_listings": {
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"achievements": {
			{Key: "milestone_type", Label: "Milestone Type", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All milestones"}, {Value: "relocation", Label: "Relocation"}, {Value: "career", Label: "Career"}, {Value: "education", Label: "Education"}, {Value: "community", Label: "Community"}}},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"learning_tracks": {
			{Key: "difficulty", Label: "Difficulty", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All levels"}, {Value: "beginner", Label: "Beginner"}, {Value: "intermediate", Label: "Intermediate"}, {Value: "advanced", Label: "Advanced"}}},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"community_digest": {
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 3},
		},
		"reviews": {
			{Key: "entity_type", Label: "Entity Type", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All entities"}, {Value: "content", Label: "Content"}, {Value: "space", Label: "Space"}, {Value: "university", Label: "University"}}},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"comparisons": {
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"community_organisations": {
			{Key: "org_type", Label: "Organisation Type", Type: "select", Default: "", Options: []CatalogOption{{Value: "", Label: "All types"}, {Value: "association", Label: "Association"}, {Value: "club", Label: "Club"}, {Value: "society", Label: "Society"}, {Value: "network", Label: "Network"}}},
			{Key: "origin_community", Label: "Origin Community", Type: "text", Default: "", Help: "Filter by origin community name."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"mentor_spotlight": {
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"newsletter_signup": {
			{Key: "list_id", Label: "List ID", Type: "text", Default: "", Help: "Newsletter list ID for subscriptions."},
			{Key: "heading", Label: "Heading", Type: "text", Default: "Subscribe to our newsletter"},
			{Key: "subtext", Label: "Subtext", Type: "text", Default: ""},
			{Key: "button_text", Label: "Button Text", Type: "text", Default: "Subscribe"},
			{Key: "placeholder", Label: "Placeholder", Type: "text", Default: "Enter your email"},
		},
		"custom_html": {
			{Key: "html", Label: "HTML Content", Type: "text", Default: "", Help: "Raw HTML (sanitized on render)."},
		},
		"trending_tags": {
			{Key: "limit", Label: "Limit", Type: "number", Default: 10},
		},
		"space_members": {
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Space UUID (required for space_members)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"user_leaderboard": {
			{Key: "limit", Label: "Limit", Type: "number", Default: 10},
		},
		"upcoming_events": {
			{Key: "space_id", Label: "Space ID", Type: "text", Default: "", Help: "Filter by space UUID (optional)."},
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
		"featured_spaces": {
			{Key: "limit", Label: "Limit", Type: "number", Default: 5},
		},
	},
	CardTypes: []CardTypeOption{
		{Value: "grid", Icon: "▦", Label: "Grid", Desc: "Card grid with images and excerpts"},
		{Value: "list", Icon: "☰", Label: "List", Desc: "Bordered rows with title and excerpt"},
		{Value: "compact", Icon: "●", Label: "Compact", Desc: "Pill-style tags, minimal text"},
		{Value: "ghost", Icon: "✦", Label: "Ghost", Desc: "Borderless, text-forward blog style"},
		{Value: "magazine", Icon: "📰", Label: "Magazine", Desc: "Featured story + sidebar items"},
		{Value: "reddit", Icon: "⚡", Label: "Reddit", Desc: "Compact rows with vote score and meta"},
	},
	ContentWidgetTypes: []string{
		"latest_content", "featured_content", "blog_posts", "forum_threads",
		"job_listings", "achievements", "learning_tracks", "community_digest",
		"reviews", "comparisons", "community_organisations",
		"mentor_spotlight", "space_members", "user_leaderboard", "upcoming_events",
		"featured_spaces",
	},
}

// GetWidgetConfigOptions handles GET /api/v1/admin/pagebuilder/widget-config-options.
// Returns the canonical widget type list and per-type config schemas so the
// admin widgets page does not hardcode them (Rule 16).
func (h *Handlers) GetWidgetConfigOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}

	// Return a deep copy so a caller cannot mutate the shared catalog.
	out := WidgetConfigOptions{
		WidgetTypes:        make([]CatalogOption, len(widgetConfigCatalog.WidgetTypes)),
		ConfigSchemas:      make(map[string][]WidgetConfigField, len(widgetConfigCatalog.ConfigSchemas)),
		CardTypes:          make([]CardTypeOption, len(widgetConfigCatalog.CardTypes)),
		ContentWidgetTypes: make([]string, len(widgetConfigCatalog.ContentWidgetTypes)),
	}
	copy(out.WidgetTypes, widgetConfigCatalog.WidgetTypes)
	copy(out.CardTypes, widgetConfigCatalog.CardTypes)
	copy(out.ContentWidgetTypes, widgetConfigCatalog.ContentWidgetTypes)

	for typ, fields := range widgetConfigCatalog.ConfigSchemas {
		outFields := make([]WidgetConfigField, len(fields))
		for i, f := range fields {
			outFields[i] = f
			if len(f.Options) > 0 {
				outFields[i].Options = make([]CatalogOption, len(f.Options))
				copy(outFields[i].Options, f.Options)
			}
		}

		out.ConfigSchemas[typ] = outFields
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// GetCatalog handles GET /api/v1/admin/catalogs/{name}.
// It returns a single catalog of value/label options for admin UI dropdowns.
// The catalog is a canonical backend source of truth (Rule 16).
func (h *Handlers) GetCatalog(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if _, ok := requireTenant(w, r); !ok {
		return
	}

	name := chi.URLParam(r, "name")
	if name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_catalog", "Catalog name is required")
		return
	}

	opts, ok := adminCatalogs[name]
	if !ok {
		errpkg.WriteError(w, http.StatusNotFound, "catalog_not_found", "Unknown catalog: "+name)
		return
	}

	// Defensive copy so a caller cannot mutate the shared catalog.
	out := make([]CatalogOption, len(opts))
	copy(out, opts)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}
