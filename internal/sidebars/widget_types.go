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
	// DefaultProps are the initial props assigned when a new widget is inserted.
	DefaultProps map[string]any `json:"default_props"`
}

// widgetRegistry holds every sidebar widget type available.
var widgetRegistry = []WidgetType{
	{
		Type:        "search",
		Name:        "Search",
		Description: "Search autocomplete input — lets users search content, spaces, tags, and users",
		Icon:        "search",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "theme_toggle",
		Name:        "Theme Toggle",
		Description: "Light/dark theme toggle button",
		Icon:        "sun-moon",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "language_toggle",
		Name:        "Language Toggle",
		Description: "Language switcher button (shows when more than one language is enabled)",
		Icon:        "languages",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "user_dropdown",
		Name:        "User Dropdown",
		Description: "User avatar with dropdown menu (profile, settings, logout)",
		Icon:        "user",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "trending_tags",
		Name:        "Trending Tags",
		Description: "Shows trending tags as clickable chips — fetched from /suggestions/tags",
		Icon:        "trending-up",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit": 8,
			"title": "Trending Tags",
		},
	},
	{
		Type:        "suggested_spaces",
		Name:        "Suggested Spaces",
		Description: "Shows suggested spaces with member counts — fetched from /suggestions/spaces",
		Icon:        "users",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit": 5,
			"title": "Suggested Spaces",
		},
	},
	{
		Type:        "ad_slot",
		Name:        "Ad Slot",
		Description: "Advertisement slot (sidebar_top or sidebar_bottom format). Suppressed on dashboard routes per ADR-060.",
		Icon:        "megaphone",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMonetization,
		DefaultProps: map[string]any{
			"slot":   "sidebar_top",
			"format": "square",
		},
	},
	{
		Type:        "rich_text",
		Name:        "Rich Text",
		Description: "Custom rich text content (TipTap JSON). Use for announcements, welcome messages, or custom HTML.",
		Icon:        "type",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		DefaultProps: map[string]any{
			"content": "",
		},
	},
	{
		Type:        "custom_html",
		Name:        "Custom HTML",
		Description: "Raw HTML/JS snippet. Rendered in a sandboxed container. Use for embeds, badges, or third-party widgets.",
		Icon:        "code",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		DefaultProps: map[string]any{
			"html": "",
		},
	},
	{
		Type:        "recent_content",
		Name:        "Recent Content",
		Description: "Shows a feed of recent content posts — fetched from /content?sort=recent",
		Icon:        "file-text",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		DefaultProps: map[string]any{
			"limit":  5,
			"title":  "Recent Posts",
			"space_id": "",
		},
	},
	{
		Type:        "space_list",
		Name:        "Space List",
		Description: "Shows a list of spaces (filtered by type or category) — fetched from /spaces",
		Icon:        "layers",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit":  5,
			"title":  "Spaces",
			"level":  "",
		},
	},
	// ── Learning widgets (Learn app) ────────────────────────────────────────
	{
		Type:        "continue_learning",
		Name:        "Continue Learning",
		Description: "Shows the user's most recent enrollment with progress bar and next-lesson link. CSR-only, auth-required.",
		Icon:        "play-circle",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		DefaultProps: map[string]any{
			"title": "Continue Learning",
		},
	},
	{
		Type:        "learning_enrollments",
		Name:        "Learning Enrollments",
		Description: "Shows a list of enrolled tracks with thumbnails, progress bars, and resume buttons. CSR-only, auth-required.",
		Icon:        "graduation-cap",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "My Learning",
		},
	},
	{
		Type:        "progress_ring",
		Name:        "Progress Ring",
		Description: "Circular progress indicator showing overall learning completion percentage. CSR-only, auth-required.",
		Icon:        "circle-progress",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		DefaultProps: map[string]any{
			"title": "My Progress",
		},
	},
	{
		Type:        "track_card",
		Name:        "Featured Track",
		Description: "Shows a featured learning track card with thumbnail, level, instructor, and enroll button.",
		Icon:        "book-open",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		DefaultProps: map[string]any{
			"track_slug": "",
			"title":      "Featured Track",
		},
	},
	{
		Type:        "curriculum_rail",
		Name:        "Curriculum Rail",
		Description: "Module→lesson tree navigation with completion states. Rendered on lesson player pages.",
		Icon:        "list-tree",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryLearning,
		DefaultProps: map[string]any{},
	},
	// ── Dashboard widgets (Main app dashboard) ──────────────────────────────
	{
		Type:        "metric_card",
		Name:        "Metric Card",
		Description: "Stat card with label, value, and optional delta indicator. Admin-configured or fetched from a metrics endpoint.",
		Icon:        "bar-chart-3",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryDashboard,
		DefaultProps: map[string]any{
			"label":   "Metric",
			"value":   "",
			"delta":   "",
			"icon":    "bar-chart-3",
		},
	},
	{
		Type:        "notification_centre",
		Name:        "Notification Centre",
		Description: "Notification dropdown with unread badge, list, and mark-all-read. Auth-required.",
		Icon:        "bell",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	// ── Docs widgets ────────────────────────────────────────────────────────
	{
		Type:        "table_of_contents",
		Name:        "Table of Contents",
		Description: "Auto-generated TOC from page headings with active-section tracking via IntersectionObserver.",
		Icon:        "list",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{
			"title": "On this page",
		},
	},
	{
		Type:        "search_launcher",
		Name:        "Search Launcher",
		Description: "Search button with ⌘K shortcut that opens a command palette. Docs/Learn variant of the search widget.",
		Icon:        "search",
		Slot:        WidgetSlotTopbar,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{},
	},
	{
		Type:        "helpful_widget",
		Name:        "Helpful Widget",
		Description: "Was-this-helpful voting widget with last-reviewed date. For docs pages.",
		Icon:        "thumbs-up",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		DefaultProps: map[string]any{},
	},
	// ── Social / community widgets ──────────────────────────────────────────
	{
		Type:        "related_discussions",
		Name:        "Related Discussions",
		Description: "Shows related forum threads based on tags or space. Fetched from /forum/related.",
		Icon:        "message-square",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit": 5,
			"title": "Related Discussions",
		},
	},
	{
		Type:        "leaderboard",
		Name:        "Leaderboard",
		Description: "Shows top contributors by karma/posts/rep. Fetched from /users/leaderboard.",
		Icon:        "trophy",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit":  5,
			"title":  "Top Contributors",
			"metric": "karma",
		},
	},
	{
		Type:        "tag_cloud",
		Name:        "Tag Cloud",
		Description: "Weighted tag cloud with font sizes proportional to usage. Fetched from /suggestions/tags.",
		Icon:        "hash",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit": 20,
			"title": "Popular Tags",
		},
	},
	{
		Type:        "featured_content",
		Name:        "Featured Content",
		Description: "Shows pinned/featured posts selected by admins. Fetched from /content?featured=true.",
		Icon:        "star",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryContent,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "Featured",
		},
	},
	{
		Type:        "bookmark_folders",
		Name:        "Bookmark Folders",
		Description: "Shows the user's bookmark folders for quick navigation. Auth-required, CSR-only.",
		Icon:        "bookmark",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryNavigation,
		DefaultProps: map[string]any{
			"title": "Bookmarks",
		},
	},
	// ── Marketing / engagement widgets ──────────────────────────────────────
	{
		Type:        "newsletter_signup",
		Name:        "Newsletter Signup",
		Description: "Email signup form for the newsletter. Shows on public pages for guests and authed users.",
		Icon:        "mail",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryMonetization,
		DefaultProps: map[string]any{
			"title":       "Subscribe",
			"placeholder": "your@email.com",
			"button_text": "Subscribe",
		},
	},
	{
		Type:        "social_links",
		Name:        "Social Links",
		Description: "Row of social media icon links (Twitter, Facebook, Instagram, etc.). Admin-configured.",
		Icon:        "share-2",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		DefaultProps: map[string]any{
			"title": "Follow Us",
			"links": []map[string]any{
				{"platform": "twitter", "url": ""},
				{"platform": "facebook", "url": ""},
				{"platform": "instagram", "url": ""},
			},
		},
	},
	{
		Type:        "cta_button",
		Name:        "Call to Action Button",
		Description: "A prominent button linking to any URL. Use for signups, downloads, or promotions.",
		Icon:        "mouse-pointer-click",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategoryCustom,
		DefaultProps: map[string]any{
			"label": "Get Started",
			"url":   "/signup",
			"variant": "primary",
		},
	},
	{
		Type:        "upcoming_events",
		Name:        "Upcoming Events",
		Description: "Shows upcoming community events with dates. The /events endpoint is not yet implemented in JEOS; the widget renders an empty state until the events section is added.",
		Icon:        "calendar",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"limit": 3,
			"title": "Upcoming Events",
		},
	},
	{
		Type:        "user_card",
		Name:        "User Card",
		Description: "Mini user profile card with avatar, name, and bio. Shown for featured users or on profile pages.",
		Icon:        "user-circle",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"user_id": "",
			"title":   "Featured User",
		},
	},
	{
		Type:        "space_categories",
		Name:        "Space Categories",
		Description: "Browseable list of space categories/types. Fetched from /spaces/categories.",
		Icon:        "folder-tree",
		Slot:        WidgetSlotContent,
		Category:    WidgetCategorySocial,
		DefaultProps: map[string]any{
			"title": "Browse Spaces",
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
