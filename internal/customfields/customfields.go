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
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// Field types supported by the custom field system.
const (
	FieldTypeText          = "text"
	FieldTypeTextarea      = "textarea"
	FieldTypeRichText      = "rich_text"
	FieldTypeNumber        = "number"
	FieldTypeCurrency      = "currency"
	FieldTypePercentage    = "percentage"
	FieldTypeBoolean       = "boolean"
	FieldTypeDate          = "date"
	FieldTypeDatetime      = "datetime"
	FieldTypeTime          = "time"
	FieldTypeDuration      = "duration"
	FieldTypeSelect        = "select"
	FieldTypeMultiselect   = "multiselect"
	FieldTypeURL           = "url"
	FieldTypeEmail         = "email"
	FieldTypePhone         = "phone"
	FieldTypeColor         = "color"
	FieldTypeRating        = "rating"
	FieldTypeImageSingle   = "image_single"
	FieldTypeImageMulti    = "image_multi"
	FieldTypeMedia         = "media"
	FieldTypePlatformEmbed = "platform_embed"
	FieldTypeReference     = "reference"
)

// FieldOption represents a single option in a select or multiselect field.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// validFieldTypes is the set of all supported field types.
var validFieldTypes = map[string]bool{
	FieldTypeText: true, FieldTypeTextarea: true, FieldTypeRichText: true,
	FieldTypeNumber: true, FieldTypeCurrency: true, FieldTypePercentage: true,
	FieldTypeBoolean: true, FieldTypeDate: true, FieldTypeDatetime: true,
	FieldTypeTime: true, FieldTypeDuration: true,
	FieldTypeSelect: true, FieldTypeMultiselect: true,
	FieldTypeURL: true, FieldTypeEmail: true, FieldTypePhone: true,
	FieldTypeColor: true, FieldTypeRating: true,
	FieldTypeImageSingle: true, FieldTypeImageMulti: true, FieldTypeMedia: true,
	FieldTypePlatformEmbed: true, FieldTypeReference: true,
}

// IsValidFieldType returns true if the given string is a recognized field type.
func IsValidFieldType(ft string) bool {
	return validFieldTypes[ft]
}

// FieldTypeValueColumn maps a field type to the custom_field_values column it
// expects to write to. Returns the column name and true, or "" and false for
// unknown types.
func FieldTypeValueColumn(ft string) (column string, ok bool) {
	switch ft {
	case FieldTypeText, FieldTypeTextarea, FieldTypeURL, FieldTypeEmail,
		FieldTypePhone, FieldTypeColor, FieldTypePlatformEmbed:
		return "value_text", true
	case FieldTypeRichText, FieldTypeSelect, FieldTypeMultiselect,
		FieldTypeMedia, FieldTypeImageSingle, FieldTypeImageMulti,
		FieldTypeReference:
		return "value_json", true
	case FieldTypeNumber, FieldTypeCurrency, FieldTypePercentage,
		FieldTypeRating:
		return "value_number", true
	case FieldTypeBoolean:
		return "value_bool", true
	case FieldTypeDate, FieldTypeDatetime, FieldTypeTime, FieldTypeDuration:
		return "value_date", true
	default:
		return "", false
	}
}

// ValidOptionsShape validates that the options JSONB matches the expected
// shape for the given field_type. Returns a user-facing error message and
// false when validation fails, or "" and true when it passes.
func ValidOptionsShape(fieldType string, opts []FieldOption) (msg string, ok bool) {
	switch fieldType {
	case FieldTypeSelect, FieldTypeMultiselect:
		if len(opts) == 0 {
			return "Select and multiselect fields require at least one option", false
		}
		for _, o := range opts {
			if o.Value == "" || o.Label == "" {
				return "Each option must have both value and label", false
			}
		}
	case FieldTypeNumber, FieldTypeCurrency, FieldTypePercentage:
		// Options are optional for number types; if present they must be
		// {min, max, is_decimal} or {currency_code}. We only validate that
		// options, when present, can be parsed sensibly — the detailed
		// structure is server-validated elsewhere.
	case FieldTypeImageSingle, FieldTypeImageMulti, FieldTypeMedia:
		// Options should have max_files, max_size_mb, aspect_ratio, or
		// allowed_types. Presence is optional; we accept anything the
		// admin provides because the upload handler enforces limits at
		// runtime.
	case FieldTypeReference:
		// Options should include ref_entity_class. Optional — the
		// resolver enforces the reference type at render time.
	case FieldTypeDuration:
		// Options may include display_format.
	}
	return "", true
}

// SupportedEmbedPlatforms is the list of platforms supported by the
// platform_embed field type.
var SupportedEmbedPlatforms = []string{
	"youtube", "github", "twitter", "vimeo", "spotify", "figma", "codepen", "google_maps",
}

// EntityTypeFromClass derives a simplified entity_type token from a full
// entity_class assignment string (Section 06). For example:
//   - "content_type:blog_post" → "content"
//   - "space_type:university"  → "space"
//   - "user_profile"           → "user_profile"
//   - "organization"           → "organization"
//   - "form:{uuid}"            → "form"
//   - "tag"                    → "tag"
//   - "ticket_type:{uuid}"     → "ticket_type"
//
// The simplified token is what the frontend uses to filter groups by entity
// type (e.g. the content composer filters on entity_type === "content").
func EntityTypeFromClass(entityClass string) string {
	// Take the part before the first colon.
	prefix := entityClass
	if idx := strings.Index(entityClass, ":"); idx > 0 {
		prefix = entityClass[:idx]
	}
	// Map the assignment prefix to the simplified entity_type token.
	switch prefix {
	case "content_type":
		return "content"
	case "space_type":
		return "space"
	case "ticket_type":
		return "ticket_type"
	default:
		return prefix
	}
}

// Service provides custom field operations with reader/writer pool
// separation. Reads go through the reader pool; writes go through the
// writer pool. This follows the same pattern as the existing db.DB struct.
type Service struct {
	read  *gen.Queries
	write *gen.Queries
	pool  *pgxpool.Pool
	cache valkeycompat.Cmdable // optional Valkey client for field-permission caching (BUG-016)
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		read:  gen.New(database.Reader),
		write: gen.New(database.Writer),
		pool:  database.Writer,
	}
}

// SetCache sets the Valkey client used for field-level RBAC caching
// (BUG-016). When set, ListVisibleFieldIDsForUser caches the result
// under jc:field_perm:{user_id} with a 5-minute TTL. When nil (the
// default), every call hits the DB directly — the pre-cache behavior.
func (s *Service) SetCache(rdb valkeycompat.Cmdable) {
	s.cache = rdb
}
