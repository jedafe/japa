// Package customfields implements the Universal Custom Field Architecture
// for JapaConnect (Section 06).
//
// One field definition system underlies everything on the platform: Space Types,
// Content Types, User Profiles, Organization Profiles, Forms, Tags, and Tickets
// all pull from the same custom_field_groups + custom_fields tables. Every field
// is RBAC-scoped — visibility, editability, and filterable-in-frontend are all
// per-UserGroup.
//
// The Service struct wraps two sqlc-generated Queries instances: one reading
// from the reader pool and one writing to the writer pool. This follows the
// existing db.Reader/db.Writer split used throughout the codebase.
package customfields

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Field types supported by the custom field system.
const (
	FieldTypeText          = "text"
	FieldTypeTextarea      = "textarea"
	FieldTypeNumber        = "number"
	FieldTypeBoolean       = "boolean"
	FieldTypeDate          = "date"
	FieldTypeDatetime      = "datetime"
	FieldTypeSelect        = "select"
	FieldTypeMultiselect   = "multiselect"
	FieldTypeURL           = "url"
	FieldTypeEmail         = "email"
	FieldTypePhone         = "phone"
	FieldTypeMedia         = "media"
	FieldTypeRichText      = "rich_text"
	FieldTypePlatformEmbed = "platform_embed"
	FieldTypeColor         = "color"
	FieldTypeRating        = "rating"
)

// FieldOption represents a single option in a select or multiselect field.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// validFieldTypes is the set of all supported field types.
var validFieldTypes = map[string]bool{
	FieldTypeText: true, FieldTypeTextarea: true, FieldTypeNumber: true,
	FieldTypeBoolean: true, FieldTypeDate: true, FieldTypeDatetime: true,
	FieldTypeSelect: true, FieldTypeMultiselect: true, FieldTypeURL: true,
	FieldTypeEmail: true, FieldTypePhone: true, FieldTypeMedia: true,
	FieldTypeRichText: true, FieldTypePlatformEmbed: true, FieldTypeColor: true,
	FieldTypeRating: true,
}

// IsValidFieldType returns true if the given string is a recognized field type.
func IsValidFieldType(ft string) bool {
	return validFieldTypes[ft]
}

// SupportedEmbedPlatforms is the list of platforms supported by the
// platform_embed field type.
var SupportedEmbedPlatforms = []string{
	"youtube", "github", "twitter", "vimeo", "spotify", "figma", "codepen", "google_maps",
}

// Service provides custom field operations with reader/writer pool
// separation. Reads go through the reader pool; writes go through the
// writer pool. This follows the same pattern as the existing db.DB struct.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
	pool  *pgxpool.Pool
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		pool:  database.Writer,
	}
}
