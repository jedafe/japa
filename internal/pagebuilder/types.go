// Package pagebuilder implements the Page Builder admin CRUD system (Section 16).
// // Provides database-backed management of blocks, templates, themes, and pages
// for the drag-and-drop page builder. The static BlockTemplateRegistry in
// content/block_templates.go supplies the built-in block types; this package
// manages tenant-customizable blocks, page templates, visual themes, and
// published page instances.
package pagebuilder

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

// Page status values.
const (
	PageStatusDraft     = "draft"
	PageStatusPublished = "published"
	PageStatusArchived  = "archived"
)

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ---------------------------------------------------------------------------
// Block types
// ---------------------------------------------------------------------------

// Block represents a pagebuilder_blocks row.
type Block struct {
	ID            pgtype.UUID `json:"id"`
	TenantID      pgtype.UUID
	Name          string             `json:"name"`
	Slug          string             `json:"slug"`
	Category      string             `json:"category"`
	Component     string             `json:"component"`
	DefaultConfig json.RawMessage    `json:"default_config"`
	ConfigSchema  json.RawMessage    `json:"config_schema"`
	PreviewImage  *string            `json:"preview_image"`
	IsSystem      bool               `json:"is_system"`
	IsEnabled     bool               `json:"is_enabled"`
	CreatedAt     pgtype.Timestamptz `json:"created_at"`
	UpdatedAt     pgtype.Timestamptz `json:"updated_at"`
}

// CreateBlockInput holds parameters for creating a block.
type CreateBlockInput struct {
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Category      string          `json:"category"`
	Component     string          `json:"component"`
	DefaultConfig json.RawMessage `json:"default_config"`
	ConfigSchema  json.RawMessage `json:"config_schema"`
	PreviewImage  *string         `json:"preview_image"`
	IsEnabled     bool            `json:"is_enabled"`
}

// UpdateBlockInput holds parameters for updating a block.
type UpdateBlockInput struct {
	Name          string          `json:"name"`
	Slug          string          `json:"slug"`
	Category      string          `json:"category"`
	Component     string          `json:"component"`
	DefaultConfig json.RawMessage `json:"default_config"`
	ConfigSchema  json.RawMessage `json:"config_schema"`
	PreviewImage  *string         `json:"preview_image"`
	IsEnabled     bool            `json:"is_enabled"`
}

// ---------------------------------------------------------------------------
// Template types
// ---------------------------------------------------------------------------

// Template represents a pagebuilder_templates row.
type Template struct {
	ID               pgtype.UUID `json:"id"`
	TenantID         pgtype.UUID
	Name             string             `json:"name"`
	Slug             string             `json:"slug"`
	Description      *string            `json:"description"`
	PageType         string             `json:"page_type"`
	Blocks           json.RawMessage    `json:"blocks"`
	IsActive         bool               `json:"is_active"`
	ThumbnailMediaID *pgtype.UUID       `json:"thumbnail_media_id"`
	SchemaVersion    int32              `json:"schema_version"`
	IsSystem         bool               `json:"is_system"`
	CreatedBy        *pgtype.UUID       `json:"created_by"`
	CreatedAt        pgtype.Timestamptz `json:"created_at"`
	UpdatedAt        pgtype.Timestamptz `json:"updated_at"`
}

// CreateTemplateInput holds parameters for creating a template.
type CreateTemplateInput struct {
	Name             string          `json:"name"`
	Slug             string          `json:"slug"`
	Description      *string         `json:"description"`
	PageType         string          `json:"page_type"`
	Blocks           json.RawMessage `json:"blocks"`
	IsActive         bool            `json:"is_active"`
	ThumbnailMediaID *pgtype.UUID    `json:"thumbnail_media_id"`
	CreatedBy        *pgtype.UUID    `json:"created_by"`
}

// UpdateTemplateInput holds parameters for updating a template.
type UpdateTemplateInput struct {
	Name             string          `json:"name"`
	Slug             string          `json:"slug"`
	Description      *string         `json:"description"`
	PageType         string          `json:"page_type"`
	Blocks           json.RawMessage `json:"blocks"`
	IsActive         *bool           `json:"is_active"`
	ThumbnailMediaID *pgtype.UUID    `json:"thumbnail_media_id"`
}

// ---------------------------------------------------------------------------
// Theme types
// ---------------------------------------------------------------------------

// Theme represents a pagebuilder_themes row.
type Theme struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID
	Name        string             `json:"name"`
	Slug        string             `json:"slug"`
	Description *string            `json:"description"`
	Colors      json.RawMessage    `json:"colors"`
	Typography  json.RawMessage    `json:"typography"`
	Spacing     json.RawMessage    `json:"spacing"`
	IsDefault   bool               `json:"is_default"`
	CreatedAt   pgtype.Timestamptz `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz `json:"updated_at"`
}

// CreateThemeInput holds parameters for creating a theme.
type CreateThemeInput struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	Colors      json.RawMessage `json:"colors"`
	Typography  json.RawMessage `json:"typography"`
	Spacing     json.RawMessage `json:"spacing"`
	IsDefault   bool            `json:"is_default"`
}

// UpdateThemeInput holds parameters for updating a theme.
type UpdateThemeInput struct {
	Name        string          `json:"name"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	Colors      json.RawMessage `json:"colors"`
	Typography  json.RawMessage `json:"typography"`
	Spacing     json.RawMessage `json:"spacing"`
	IsDefault   bool            `json:"is_default"`
}

// ---------------------------------------------------------------------------
// Page types
// ---------------------------------------------------------------------------

// Page represents a pages row.
type Page struct {
	ID          pgtype.UUID `json:"id"`
	TenantID    pgtype.UUID
	Title       string              `json:"title"`
	Slug        string              `json:"slug"`
	Description *string             `json:"description"`
	TemplateID  *pgtype.UUID        `json:"template_id"`
	ThemeID     *pgtype.UUID        `json:"theme_id"`
	Blocks      json.RawMessage     `json:"blocks"`
	Meta        json.RawMessage     `json:"meta"`
	Status      string              `json:"status"`
	PageType    string              `json:"page_type"`
	PublishedAt *pgtype.Timestamptz `json:"published_at"`
	CreatedBy   *pgtype.UUID        `json:"created_by"`
	CreatedAt   pgtype.Timestamptz  `json:"created_at"`
	UpdatedAt   pgtype.Timestamptz  `json:"updated_at"`
}

// CreatePageInput holds parameters for creating a page.
type CreatePageInput struct {
	Title       string          `json:"title"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	TemplateID  *pgtype.UUID    `json:"template_id"`
	ThemeID     *pgtype.UUID    `json:"theme_id"`
	Blocks      json.RawMessage `json:"blocks"`
	Meta        json.RawMessage `json:"meta"`
	Status      string          `json:"status"`
	PageType    string          `json:"page_type"`
}

// UpdatePageInput holds parameters for updating a page.
type UpdatePageInput struct {
	Title       string          `json:"title"`
	Slug        string          `json:"slug"`
	Description *string         `json:"description"`
	TemplateID  *pgtype.UUID    `json:"template_id"`
	ThemeID     *pgtype.UUID    `json:"theme_id"`
	Blocks      json.RawMessage `json:"blocks"`
	Meta        json.RawMessage `json:"meta"`
	Status      string          `json:"status"`
	PageType    string          `json:"page_type"`
}

// now returns the current time as a pgtype.Timestamptz.
