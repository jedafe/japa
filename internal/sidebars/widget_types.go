// Package sidebars — Sidebar Widget Type Registry (Section 59, ADR-066).
//
// The registry defines the sidebar widget types available in the sidebar
// manager. Each widget carries sensible default props so the admin UI can
// instantiate a new widget without a round-trip. The slot field determines
// where the widget renders: "topbar" widgets go in the sticky top bar
// (search, theme toggle, user dropdown), "content" widgets go in the
// scrollable sidebar body (trending tags, suggested spaces, ad slots).
package sidebars

// Widget slots.
const (
	WidgetSlotTopbar  = "topbar"  // sticky top bar (search, theme toggle, etc.)
	WidgetSlotContent = "content" // scrollable sidebar body
)

// Widget categories used to group widgets in the admin picker.
const (
	WidgetCategoryNavigation   = "navigation"
	WidgetCategoryContent      = "content"
	WidgetCategorySocial       = "social"
	WidgetCategoryMonetization = "monetization"
	WidgetCategoryLearning     = "learning"
	WidgetCategoryDashboard    = "dashboard"
	WidgetCategoryMarketing    = "marketing"
	WidgetCategoryCustom       = "custom"
)

// WidgetType describes a single sidebar widget type.
type WidgetType struct {
	// Type is the unique identifier stored in sidebar_widgets.widget_type.
	Type string `json:"type"`
	// Name is the human-readable label shown in the admin picker.
	Name string `json:"name"`
	// Description explains what the widget does, shown as tooltip text.
	Description string `json:"description"`
	// Icon is a Lucide icon name for the picker UI.
	Icon string `json:"icon"`
	// Slot determines where the widget renders: "topbar" or "content".
	Slot string `json:"slot"`
	// Category groups the widget in the admin picker.
	Category string `json:"category"`
	// Apps lists the frontend apps whose SidebarWidgetRenderer actually
	// implements this type ("main", "docs", "learn"). An empty slice means
	// all apps. The admin picker filters by this and the service rejects
	// widget creation for apps that cannot render the type — otherwise an
	// admin can place a widget that renders as a permanent stub on a surface
	// that has no implementation for it.
	Apps []string `json:"apps"`
	// DefaultProps are the initial props assigned when a new widget is inserted.
	DefaultProps map[string]any `json:"default_props"`
}

// appAll is a convenience value for widget types rendered by every app.
var appAll = []string{string(AppMain), string(AppDocs), string(AppLearn)}

// appMainOnly scopes a widget type to the Main app — these widgets consume
// main-app data surfaces (spaces, trending, content, orgs) that have no
// renderer implementation on Docs/Learn.
var appMainOnly = []string{string(AppMain)}

// appMainLearn scopes learning widgets to the two surfaces that implement
// them (Main sidebar + Learn sidebar).
var appMainLearn = []string{string(AppMain), string(AppLearn)}

// appDocsLearn scopes the docs-style search launcher to Docs and Learn.
var appDocsLearn = []string{string(AppDocs), string(AppLearn)}

// appDocsOnly scopes docs-only widgets (TOC, helpful vote) to the Docs app.
var appDocsOnly = []string{string(AppDocs)}

// appLearnOnly scopes learn-only widgets (curriculum rail) to the Learn app.
var appLearnOnly = []string{string(AppLearn)}

// widgetRegistry holds every sidebar widget type available.
var widgetRegistry = []WidgetType{
	{
		Type:        "search",
		Name:        "Search",
		Description: "Search input scoped to the current app",
		Icon:        "search",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"placeholder": "",
		},
	},
	{
		Type:        "search_launcher",
		Name:        "Search Launcher",
		Description: "Search launcher button for Docs/Learn (opens cmd+k palette or inline search)",
		Icon:        "search",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		Apps:        appDocsLearn,
		DefaultProps: map[string]any{
			"placeholder": "",
		},
	},
	{
		Type:         "theme_toggle",
		Name:         "Theme Toggle",
		Description:  "Light/dark mode toggle button",
		Icon:         "sun-moon",
		Slot:         WidgetSlotTopbar,
		Category:     WidgetCategoryNavigation,
		Apps:         appAll,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "language_toggle",
		Name:        "Language Toggle",
		Description: "Language switcher dropdown",
		Icon:        "globe",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"languages": "en,fr",
		},
	},
	{
		Type:         "user_dropdown",
		Name:         "User Dropdown",
		Description:  "Authenticated user menu (profile, settings, logout)",
		Icon:         "user",
		Slot:         WidgetSlotTopbar,
		Category:     WidgetCategoryNavigation,
		Apps:         appAll,
		DefaultProps: map[string]any{},
	},
	{
		Type:         "notification_centre",
		Name:         "Notification Centre",
		Description:  "Notification bell + dropdown",
		Icon:         "bell",
		Slot:         WidgetSlotTopbar,
		Category:     WidgetCategoryNavigation,
		Apps:         appMainOnly,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "trending_tags",
		Name:        "Trending Tags",
		Description: "Trending hashtag list — data fetched from /trending/tags",
		Icon:        "hash",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 8,
			"title": "Trending Tags",
		},
	},
	{
		Type:        "suggested_spaces",
		Name:        "Suggested Spaces",
		Description: "Suggested spaces to join — data fetched from /suggestions/spaces (authenticated users only)",
		Icon:        "users",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 5,
			"title": "Suggested Spaces",
		},
	},
	{
		Type:        "suggested_content",
		Name:        "Suggested Content",
		Description: "Personalised content suggestions — data fetched from /suggestions/content (authenticated users only)",
		Icon:        "sparkles",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 5,
			"title": "For You",
		},
	},
	{
		Type:        "ad_slot",
		Name:        "Ad Slot",
		Description: "Advertisement slot — data fetched from /ads/serve?slot= (requires the feature.ads flag; renders nothing when no campaign is served)",
		Icon:        "megaphone",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMonetization,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"slot":   "sidebar_top",
			"format": "square",
		},
	},
	{
		Type:        "rich_text",
		Name:        "Rich Text",
		Description: "Free-form HTML/text block (sanitized server-side)",
		Icon:        "type",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"content": "",
		},
	},
	{
		Type:        "custom_html",
		Name:        "Custom HTML / Embed",
		Description: "Raw HTML or iframe embed (sanitized server-side; scripts stripped unless embed_enabled)",
		Icon:        "code",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"html":          "",
			"embed_enabled": false,
		},
	},
	{
		Type:        "recent_content",
		Name:        "Recent Content",
		Description: "Latest published content items — data fetched from /content (optionally scoped via the space_id prop)",
		Icon:        "file-text",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit":    5,
			"title":    "Recent Posts",
			"space_id": "",
		},
	},
	{
		Type:        "space_list",
		Name:        "Space List",
		Description: "List of spaces — data fetched from /spaces (optionally filtered via the level prop)",
		Icon:        "layout-grid",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 5,
			"level": "",
			"title": "Browse Spaces",
		},
	},
	{
		Type:        "continue_learning",
		Name:        "Continue Learning",
		Description: "User's most recent enrollment with progress — data fetched from /me/enrollments (authenticated users only)",
		Icon:        "play-circle",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		Apps:        appMainLearn,
		DefaultProps: map[string]any{
			"title": "Continue Learning",
		},
	},
	{
		Type:        "learning_enrollments",
		Name:        "Learning Enrollments",
		Description: "User's active learning tracks with progress — data fetched from /me/enrollments (authenticated users only)",
		Icon:        "graduation-cap",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		Apps:        appMainLearn,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "My Learning",
		},
	},
	{
		Type:        "progress_ring",
		Name:        "Learning Progress Ring",
		Description: "Aggregate learning progress indicator — data fetched from /me/enrollments (authenticated users only)",
		Icon:        "circle-dashed",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		Apps:        appMainLearn,
		DefaultProps: map[string]any{
			"title": "My Progress",
		},
	},
	{
		Type:         "curriculum_rail",
		Name:         "Curriculum Rail",
		Description:  "Module/lesson rail for the Learn lesson player (rendered by the lesson player route)",
		Icon:         "list",
		Slot:         WidgetSlotContent,
		Category:     WidgetCategoryLearning,
		Apps:         appLearnOnly,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "track_card",
		Name:        "Featured Track Card",
		Description: "Featured learning track card — data fetched from /learn/university/tracks/{slug} (track_slug prop) or /learn/university/catalog (fallback)",
		Icon:        "book-open",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		Apps:        appMainLearn,
		DefaultProps: map[string]any{
			"track_slug": "",
			"title":      "Featured Track",
		},
	},
	{
		Type:         "table_of_contents",
		Name:         "Table of Contents",
		Description:  "Auto-generated TOC for docs pages (rendered by the docs page route)",
		Icon:         "list",
		Slot:         WidgetSlotContent,
		Category:     WidgetCategoryNavigation,
		Apps:         appDocsOnly,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "related_discussions",
		Name:        "Related Discussions",
		Description: "Contextual forum/comment threads — data fetched from /organizations/{id}/content, /users/{id}/content, or /content/{id}/discussions depending on page context",
		Icon:        "message-square",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "Related Discussions",
		},
	},
	{
		Type:        "leaderboard",
		Name:        "Leaderboard",
		Description: "Top contributors — data fetched from /trending/users",
		Icon:        "trophy",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 5,
			"title": "Top Contributors",
		},
	},
	{
		Type:        "helpful_widget",
		Name:        "Was this helpful?",
		Description: "Docs feedback widget — posts to /docs/{audience}/{slug}/feedback (authenticated users only)",
		Icon:        "thumbs-up",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryNavigation,
		Apps:        appDocsOnly,
		DefaultProps: map[string]any{
			"question": "Was this page helpful?",
		},
	},
	{
		Type:        "tag_cloud",
		Name:        "Tag Cloud",
		Description: "Visual tag cloud — uses the same trending tag data as the trending_tags widget",
		Icon:        "cloud",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 20,
			"title": "Popular Tags",
		},
	},
	{
		Type:        "metric_card",
		Name:        "Metric Card",
		Description: "Small stat card (icon, value, label)",
		Icon:        "bar-chart-3",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryDashboard,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"icon":  "bar-chart-3",
			"value": "0",
			"label": "Metric",
		},
	},
	{
		Type:        "featured_content",
		Name:        "Featured Content",
		Description: "Curated featured posts — data fetched from /blog/featured",
		Icon:        "star",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "Featured",
		},
	},
	{
		Type:        "bookmark_folders",
		Name:        "Bookmark Folders",
		Description: "User's bookmark folders — data fetched from /me/bookmark-folders (authenticated users only)",
		Icon:        "bookmark",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryDashboard,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 6,
			"title": "My Bookmarks",
		},
	},
	{
		Type:        "upcoming_events",
		Name:        "Upcoming Events",
		Description: "Calendar upcoming events — data fetched from /calendar?view=list (requires the calendar.enabled feature flag; hidden when the calendar is disabled or empty)",
		Icon:        "calendar",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "Upcoming Events",
		},
	},
	{
		Type:        "user_card",
		Name:        "User / Profile Card",
		Description: "User or organization profile card — data fetched from /users/{username} or /organizations/{slug} based on page context",
		Icon:        "user-circle",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"username": "",
		},
	},
	{
		Type:        "space_categories",
		Name:        "Space Categories",
		Description: "Space type/category chips — data fetched from /space-types",
		Icon:        "folder-tree",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		Apps:        appMainOnly,
		DefaultProps: map[string]any{
			"title": "Categories",
		},
	},
	{
		Type:        "newsletter_signup",
		Name:        "Newsletter Signup",
		Description: "Email newsletter subscription form — posts to /newsletter/subscribe (requires the feature.newsletter flag; hidden when disabled)",
		Icon:        "mail",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMarketing,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"title":       "Stay in the loop",
			"description": "Get weekly updates in your inbox.",
		},
	},
	{
		Type:        "social_links",
		Name:        "Social Links",
		Description: "Row of social profile links",
		Icon:        "share-2",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMarketing,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"links": "",
		},
	},
	{
		Type:        "cta_button",
		Name:        "Call to Action",
		Description: "Prominent CTA button",
		Icon:        "mouse-pointer-click",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMarketing,
		Apps:        appAll,
		DefaultProps: map[string]any{
			"label":   "Get Started",
			"url":     "",
			"variant": "primary",
		},
	},
}

// GetWidgetTypes returns all registered sidebar widget types.
func GetWidgetTypes() []WidgetType {
	return widgetRegistry
}

// IsValidWidgetType returns true if the given type is a registered widget type.
func IsValidWidgetType(widgetType string) bool {
	for _, wt := range widgetRegistry {
		if wt.Type == widgetType {
			return true
		}
	}

	return false
}

// WidgetTypeAllowedForApp reports whether the registered widget type is
// implemented by the given frontend app's SidebarWidgetRenderer. Widget types
// with an empty Apps list are rendered by all apps. This prevents admins from
// placing widgets that a surface renders as a permanent stub or no-op.
func WidgetTypeAllowedForApp(widgetType, app string) bool {
	wt := LookupWidgetType(widgetType)
	if wt == nil {
		return false
	}
	if len(wt.Apps) == 0 {
		return true
	}
	for _, a := range wt.Apps {
		if a == app {
			return true
		}
	}
	return false
}

// LookupWidgetType returns the registered WidgetType for the given type string,
// or nil if the type is not registered. Used to populate the `slot` field on
// resolved widgets so the frontend can split topbar/content slots without
// re-deriving it from a hardcoded type set. Ref: Section 59 §6.
func LookupWidgetType(widgetType string) *WidgetType {
	for i := range widgetRegistry {
		if widgetRegistry[i].Type == widgetType {
			return &widgetRegistry[i]
		}
	}

	return nil
}
