// Package content — Block Template Registry for the Page Builder (Section 16).
//
// The registry defines the 11 block types available in the page builder
// front-end. Each block carries sensible default props so the editor can
// instantiate a new block without a round-trip. Categories group blocks
// in the picker UI.
package content

// Block categories used to group blocks in the editor picker.
const (
	BlockCategoryLayout      = "layout"
	BlockCategoryContent     = "content"
	BlockCategoryMedia       = "media"
	BlockCategoryInteractive = "interactive"
	BlockCategoryData        = "data"
)

// BlockTemplate describes a single block type available in the page builder.
type BlockTemplate struct {
	// Type is the unique identifier stored in field_values blocks (e.g. "hero").
	Type string `json:"type"`
	// Name is the human-readable label shown in the editor picker.
	Name string `json:"name"`
	// Description explains what the block does, shown as tooltip text.
	Description string `json:"description"`
	// Icon is a Lucide icon name (e.g. "layout", "type") for the picker UI.
	Icon string `json:"icon"`
	// DefaultProps are the initial props assigned when a new block is inserted.
	DefaultProps map[string]any `json:"default_props"`
	// Category groups the block in the editor picker (layout, content, media,
	// interactive, data).
	Category string `json:"category"`
}

// BlockTemplateRegistry holds every block type available in the page builder.
// It is constructed once at package level and returned by GetBlockTemplates.
type BlockTemplateRegistry struct {
	templates []BlockTemplate
}

// defaultRegistry is the package-level registry initialised with all 11 block
// types. Constructed via newDefaultRegistry at package init time.
var defaultRegistry = newDefaultRegistry()

// newDefaultRegistry builds the registry with all 11 block types and their
// default props.
func newDefaultRegistry() *BlockTemplateRegistry {
	return &BlockTemplateRegistry{
		templates: []BlockTemplate{
			{
				Type:        "hero",
				Name:        "Hero Section",
				Description: "Hero section with title, subtitle, background image, and call-to-action",
				Icon:        "layout",
				DefaultProps: map[string]any{
					"title":           "",
					"subtitle":        "",
					"background_image": "",
					"cta_text":        "",
					"cta_url":         "",
					"overlay_opacity": 0.4,
				},
				Category: BlockCategoryLayout,
			},
			{
				Type:        "rich_text",
				Name:        "Rich Text",
				Description: "Rich text content powered by TipTap editor",
				Icon:        "type",
				DefaultProps: map[string]any{
					"content": "",
				},
				Category: BlockCategoryContent,
			},
			{
				Type:        "image",
				Name:        "Image / Gallery",
				Description: "Single image or image gallery",
				Icon:        "image",
				DefaultProps: map[string]any{
					"images":  []any{},
					"caption": "",
					"layout":  "single",
				},
				Category: BlockCategoryMedia,
			},
			{
				Type:        "cta",
				Name:        "Call to Action",
				Description: "Call-to-action button or button group",
				Icon:        "mouse-pointer-click",
				DefaultProps: map[string]any{
					"text":    "",
					"url":     "",
					"style":   "primary",
					"variant": "filled",
				},
				Category: BlockCategoryInteractive,
			},
			{
				Type:        "stats",
				Name:        "Statistics",
				Description: "Statistics display with numbers and labels",
				Icon:        "bar-chart-3",
				DefaultProps: map[string]any{
					"items": []any{},
					"layout": "horizontal",
				},
				Category: BlockCategoryData,
			},
			{
				Type:        "faq",
				Name:        "FAQ Accordion",
				Description: "Frequently asked questions in an accordion layout",
				Icon:        "help-circle",
				DefaultProps: map[string]any{
					"questions": []any{},
					"title":     "",
				},
				Category: BlockCategoryInteractive,
			},
			{
				Type:        "testimonial",
				Name:        "Testimonial",
				Description: "Testimonial quote with attribution",
				Icon:        "quote",
				DefaultProps: map[string]any{
					"quote":     "",
					"author":    "",
					"role":      "",
					"avatar_url": "",
				},
				Category: BlockCategoryContent,
			},
			{
				Type:        "comparison",
				Name:        "Comparison",
				Description: "Comparison embed linking to the comparison system (Section 30)",
				Icon:        "git-compare",
				DefaultProps: map[string]any{
					"comparison_id": "",
					"display_mode":  "table",
				},
				Category: BlockCategoryData,
			},
			{
				Type:        "form",
				Name:        "Form Embed",
				Description: "Form embed linking to the custom form system (Section 06)",
				Icon:        "file-text",
				DefaultProps: map[string]any{
					"form_id":     "",
					"show_title":  true,
					"show_border": true,
				},
				Category: BlockCategoryInteractive,
			},
			{
				Type:        "spacer",
				Name:        "Spacer / Divider",
				Description: "Vertical spacer or visual divider between blocks",
				Icon:        "minus",
				DefaultProps: map[string]any{
					"height": 32,
					"style":  "space",
				},
				Category: BlockCategoryLayout,
			},
			{
				Type:        "dynamic_listing",
				Name:        "Dynamic Listing",
				Description: "Dynamic content listing driven by filters",
				Icon:        "list",
				DefaultProps: map[string]any{
					"content_type": "",
					"space_id":     "",
					"limit":        10,
					"sort_by":      "created_at",
					"sort_order":   "desc",
				},
				Category: BlockCategoryData,
			},
		},
	}
}

// GetBlockTemplates returns all registered block templates. The returned slice
// is a copy; callers may not mutate the registry through it.
func GetBlockTemplates() []BlockTemplate {
	out := make([]BlockTemplate, len(defaultRegistry.templates))
	for i, t := range defaultRegistry.templates {
		out[i] = t
		out[i].DefaultProps = deepCopyProps(t.DefaultProps)
	}
	return out
}

// GetBlockTemplateByType returns the template for the given block type and a
// boolean indicating whether it was found.
func GetBlockTemplateByType(blockType string) (BlockTemplate, bool) {
	for _, t := range defaultRegistry.templates {
		if t.Type == blockType {
			t.DefaultProps = deepCopyProps(t.DefaultProps)
			return t, true
		}
	}
	return BlockTemplate{}, false
}

// BlockTypeCount returns the number of registered block types.
func BlockTypeCount() int {
	return len(defaultRegistry.templates)
}

// deepCopyProps returns a shallow copy of the props map to prevent callers
// from mutating the global registry's default values.
func deepCopyProps(props map[string]any) map[string]any {
	if props == nil {
		return nil
	}
	cp := make(map[string]any, len(props))
	for k, v := range props {
		cp[k] = v
	}
	return cp
}
