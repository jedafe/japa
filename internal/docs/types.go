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
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
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
	CreatedAt    string  `json:"created_at"`
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
}

// UpdateDocPageRequest is the JSON body for PATCH /api/v1/admin/docs/{id}.
type UpdateDocPageRequest struct {
	Title        string          `json:"title"`
	Content      json.RawMessage `json:"content"`
	ReviewStatus string          `json:"review_status"`
}

// ValidAudiences lists accepted audience slugs.
var ValidAudiences = map[string]bool{
	"users":          true,
	"premium":        true,
	"organizations":  true,
	"moderators":     true,
	"api":            true,
	"admin":          true,
	"superadmin":     true,
}

// ValidReviewStatuses lists accepted review_status values.
var ValidReviewStatuses = map[string]bool{
	"draft":     true,
	"in_review": true,
	"approved":  true,
	"published": true,
}
