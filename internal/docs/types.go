// Package docs provides HTTP handlers and business logic for the documentation
// system (Section 44). Documentation pages are RBAC-gated by audience and
// reviewed through a HITL workflow before publication.
package docs

import "encoding/json"

// DocPage is the API response for a documentation page.
type DocPage struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Slug           string          `json:"slug"`
	Audience       string          `json:"audience"`
	Content        json.RawMessage `json:"content"`
	AIGenerated    bool            `json:"ai_generated"`
	ReviewStatus   string          `json:"review_status"`
	LastReviewedBy *string         `json:"last_reviewed_by,omitempty"`
	LastReviewedAt *string         `json:"last_reviewed_at,omitempty"`
	ParentID       *string         `json:"parent_id,omitempty"`
	Position       int32           `json:"position"`
	Category       *string         `json:"category,omitempty"`
	Subcategory    *string         `json:"subcategory,omitempty"`
	HelpfulYes     int32           `json:"helpful_yes"`
	HelpfulNo      int32           `json:"helpful_no"`
	ContentLang    string          `json:"content_lang"`
	// §44 amendment 5: media-library feature image — FeatureImageURL is the
	// computed served URL (imgproxy), "" when unset.
	FeatureImageID  *string `json:"feature_image_id,omitempty"`
	FeatureImageURL string  `json:"feature_image_url"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// DocPageSummary is a lightweight listing item (no content body).
type DocPageSummary struct {
	ID           string  `json:"id"`
	Title        string  `json:"title"`
	Slug         string  `json:"slug"`
	Audience     string  `json:"audience"`
	ReviewStatus string  `json:"review_status"`
	ParentID     *string `json:"parent_id,omitempty"`
	Position     int32   `json:"position"`
	Category     *string `json:"category,omitempty"`
	Subcategory  *string `json:"subcategory,omitempty"`
	CreatedAt    string  `json:"created_at"`
	// §44 amendment 5: listing cards render the feature image thumbnail.
	FeatureImageID  *string `json:"feature_image_id,omitempty"`
	FeatureImageURL string  `json:"feature_image_url"`
	// Excerpt is a plain-text preview derived from content (≤140 runes).
	Excerpt string `json:"excerpt"`
}

// CreateDocPageRequest is the JSON body for POST /api/v1/admin/docs.
type CreateDocPageRequest struct {
	Title        string          `json:"title"`
	Slug         string          `json:"slug"`
	Audience     string          `json:"audience"`
	Content      json.RawMessage `json:"content"`
	AIGenerated  bool            `json:"ai_generated"`
	ReviewStatus string          `json:"review_status"`
	ParentID     *string         `json:"parent_id,omitempty"`
	Position     int32           `json:"position"`
	Category     *string         `json:"category,omitempty"`
	Subcategory  *string         `json:"subcategory,omitempty"`
	// §44 amendment 5: media id for the page's feature image; Summary is
	// recorded on the created revision.
	FeatureImageID *string `json:"feature_image_id,omitempty"`
	Summary        *string `json:"summary,omitempty"`
}

// UpdateDocPageRequest is the JSON body for PATCH /api/v1/admin/docs/{id}.
// Pointer fields follow preserve-on-absent semantics: nil keeps the stored
// value, "" clears it (Category, Subcategory, ParentID), and any other value
// replaces it (§44 amendment 2). ReviewStatus accepts only draft/in_review —
// approved/published go through the dedicated approve/publish endpoints.
// ExpectedUpdatedAt is an optimistic-lock precondition (RFC3339); a mismatch
// returns 409 stale_write.
type UpdateDocPageRequest struct {
	Title             string          `json:"title"`
	Content           json.RawMessage `json:"content"`
	ReviewStatus      string          `json:"review_status"`
	Slug              *string         `json:"slug,omitempty"`
	ParentID          *string         `json:"parent_id,omitempty"`
	Position          *int32          `json:"position,omitempty"`
	Audience          *string         `json:"audience,omitempty"`
	Category          *string         `json:"category,omitempty"`
	Subcategory       *string         `json:"subcategory,omitempty"`
	ExpectedUpdatedAt *string         `json:"expected_updated_at,omitempty"`
	// §44 amendment 5: media id — nil preserves, "" clears. Summary is
	// recorded on the revision row when this write versions.
	FeatureImageID *string `json:"feature_image_id,omitempty"`
	Summary        *string `json:"summary,omitempty"`
}

// DocRevision is one snapshot in a page's content history (§44 amendment 5).
// Content is populated only by the single-revision endpoint.
type DocRevision struct {
	ID             string          `json:"id"`
	DocPageID      string          `json:"doc_page_id"`
	Version        int32           `json:"version"`
	Title          string          `json:"title"`
	Summary        *string         `json:"summary,omitempty"`
	FeatureImageID *string         `json:"feature_image_id,omitempty"`
	CreatedBy      *string         `json:"created_by,omitempty"`
	CreatedAt      string          `json:"created_at"`
	Excerpt        string          `json:"excerpt"`
	IsLatest       bool            `json:"is_latest"`
	Content        json.RawMessage `json:"content,omitempty"`
}

// DocFeedback is the response for GET .../feedback — aggregate counts plus
// the caller's recorded vote (§44 amendment 2).
type DocFeedback struct {
	PageID     string  `json:"page_id"`
	HelpfulYes int32   `json:"helpful_yes"`
	HelpfulNo  int32   `json:"helpful_no"`
	MyVote     *string `json:"my_vote"`
}

// DocNavItem is a single navigation entry (a published page), optionally
// nesting child pages one level deep.
type DocNavItem struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Slug        string       `json:"slug"`
	Audience    string       `json:"audience"`
	ParentID    *string      `json:"parent_id,omitempty"`
	Position    int32        `json:"position"`
	Category    *string      `json:"category,omitempty"`
	Subcategory *string      `json:"subcategory,omitempty"`
	Children    []DocNavItem `json:"children,omitempty"`
	// §44 amendment 5: listing cards render the feature image thumbnail.
	FeatureImageURL string `json:"feature_image_url,omitempty"`
}

// DocNavTree groups navigation items by audience section.
type DocNavTree struct {
	Audience string       `json:"audience"`
	Items    []DocNavItem `json:"items"`
}

// DocSearchResult is a lightweight search hit for the docs sidebar.
type DocSearchResult struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Slug        string  `json:"slug"`
	Audience    string  `json:"audience"`
	Category    *string `json:"category,omitempty"`
	Subcategory *string `json:"subcategory,omitempty"`
	Excerpt     string  `json:"excerpt"`
}

// AudienceOrder is the canonical display order for audience sections.
var AudienceOrder = []string{"users", "premium", "organizations", "moderators", "api", "admin", "superadmin"}

// ValidAudiences lists accepted audience slugs.
var ValidAudiences = map[string]bool{
	"users":         true,
	"premium":       true,
	"organizations": true,
	"moderators":    true,
	"api":           true,
	"admin":         true,
	"superadmin":    true,
}

// ValidReviewStatuses lists accepted review_status values.
var ValidReviewStatuses = map[string]bool{
	"draft":     true,
	"in_review": true,
	"approved":  true,
	"published": true,
}

// StatusOrder is the canonical display order for review_status values.
var StatusOrder = []string{"draft", "in_review", "approved", "published"}
