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
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
)

// GuideCategory is a evergreen guide category (Visa, Travel, Scholarship, etc.).
// Defined here to avoid an import cycle (content imports the interface,
// customfields provides the implementation, server.go wires them together).
type GuideCategory struct {
	Slug      string
	Label     string
	IsSystem  bool   `json:"is_system"`
	GroupID   string `json:"group_id,omitempty"`
	GroupSlug string `json:"group_slug,omitempty"`
}

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
	FieldTypeRepeater      = "repeater"
	FieldTypeGroup         = "group"
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
	FieldTypeRepeater: true, FieldTypeGroup: true,
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
		FieldTypeReference, FieldTypeRepeater:
		return "value_json", true
	case FieldTypeNumber, FieldTypeCurrency, FieldTypePercentage,
		FieldTypeRating:
		return "value_number", true
	case FieldTypeBoolean:
		return "value_bool", true
	case FieldTypeDate, FieldTypeDatetime, FieldTypeTime, FieldTypeDuration:
		return "value_date", true
	case FieldTypeGroup:
		// Group fields have no value — they are visual containers only.
		return "", false
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
//   - "content_type:blog_post"            → "content"
//   - "content_type:discussion"           → "content"
//   - "content_type:guide:health_insurance" → "guide_category:health_insurance"
//   - "space_type:university"             → "space"
//   - "space_type"                        → "space"
//   - "user_profile"                      → "user_profile"
//   - "organization"                      → "organization"
//   - "form:{uuid}"                       → "form"
//   - "tag"                               → "tag"
//   - "ticket_type:{uuid}"                → "ticket_type"
//
// The simplified token is what the frontend uses to filter groups by entity
// type (e.g. the content composer filters on entity_type === "content").
// Guide sub-categories map to the "guide_category:{slug}" token so they
// match the values in the custom_field_entity_types table and can be
// filtered individually in the admin panel.
func EntityTypeFromClass(entityClass string) string {
	// Take the part before the first colon.
	prefix := entityClass
	if idx := strings.Index(entityClass, ":"); idx > 0 {
		prefix = entityClass[:idx]
	}
	// Map the assignment prefix to the simplified entity_type token.
	switch prefix {
	case "content_type":
		// Guide sub-categories (content_type:guide:{slug}) have their
		// own entity_type tokens in the custom_field_entity_types table
		// (guide_category:{slug}) so they can be filtered individually.
		if rest := entityClass[len("content_type:"):]; strings.HasPrefix(rest, "guide:") {
			return "guide_category:" + rest[len("guide:"):]
		}
		return "content"
	case "space_type":
		return "space"
	case "ticket_type":
		return "ticket_type"
	default:
		return prefix
	}
}

// EntityClassFromType is the reverse of EntityTypeFromClass: it maps a
// simplified entity_type token back to the generic entity_class prefix
// used in custom_field_group_assignments. When creating a group from the
// admin panel, the admin selects an entity_type (e.g. "content") and this
// function derives the entity_class ("content_type") for the assignment
// row. entity_id is left NULL so the assignment applies to all entities of
// that class.
func EntityClassFromType(entityType string) string {
	switch entityType {
	case "content":
		return "content_type"
	case "space":
		return "space_type"
	}
	// Guide sub-categories: "guide_category:visa" → "content_type:guide:visa"
	if strings.HasPrefix(entityType, "guide_category:") {
		return "content_type:guide:" + entityType[len("guide_category:"):]
	}
	// ticket_type, user_profile, organization, tag, form — the entity_type
	// token IS the entity_class prefix (no colon-suffix mapping needed).
	return entityType
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

// GetCustomFieldByKey looks up a custom field definition by its key within
// a tenant. Used by the organizations handler to read the org_type select
// field's options from the custom field system (Section 07 + 08).
func (s *Service) GetCustomFieldByKey(ctx context.Context, key string, tenantID pgtype.UUID) (gen.GetCustomFieldByKeyRow, error) {
	return s.read.GetCustomFieldByKey(ctx, gen.GetCustomFieldByKeyParams{
		Key:      key,
		TenantID: tenantID,
	})
}

// ListGuideCategories returns all guide categories for a tenant. Guide
// categories are stored in custom_field_entity_types with value prefix
// 'guide_category:'. The slug is the part after the prefix. Also enriches
// each category with its field group ID so the admin panel can link to the
// field editor.
func (s *Service) ListGuideCategories(ctx context.Context, tenantID pgtype.UUID) ([]GuideCategory, error) {
	rows, err := s.read.ListCustomFieldEntityTypesByPrefix(ctx, gen.ListCustomFieldEntityTypesByPrefixParams{
		TenantID: tenantID,
		Prefix:   "guide_category:%",
	})
	if err != nil {
		return nil, fmt.Errorf("listing guide categories: %w", err)
	}
	cats := make([]GuideCategory, 0, len(rows))
	for _, row := range rows {
		slug := strings.TrimPrefix(row.Value, "guide_category:")
		cat := GuideCategory{Slug: slug, Label: row.Label, IsSystem: row.IsSystem}

		// Try to find the associated field group (slug = "guide_{slug}_fields").
		groupSlug := "guide_" + slug + "_fields"
		group, err := s.read.GetCustomFieldGroupBySlug(ctx, gen.GetCustomFieldGroupBySlugParams{
			TenantID: tenantID,
			Slug:     groupSlug,
		})
		if err == nil {
			cat.GroupID = group.ID.String()
			cat.GroupSlug = group.Slug
		}
		cats = append(cats, cat)
	}
	return cats, nil
}

// CreateGuideCategoryInput holds parameters for creating a guide category.
type CreateGuideCategoryInput struct {
	Slug  string // e.g. "visa", "scholarship"
	Label string // e.g. "Visa Guide"
}

// CreateGuideCategory creates a new guide category: inserts a
// custom_field_entity_types row (value = "guide_category:{slug}"), creates a
// custom_field_groups row for the category's fields, and assigns the group to
// the "content_type:guide:{slug}" entity_class so the composer can load
// category-specific fields. All three inserts run in a single transaction.
func (s *Service) CreateGuideCategory(ctx context.Context, tenantID pgtype.UUID, input CreateGuideCategoryInput) (GuideCategory, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GuideCategory{}, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := gen.New(tx)

	value := "guide_category:" + input.Slug
	_, err = q.CreateCustomFieldEntityType(ctx, gen.CreateCustomFieldEntityTypeParams{
		TenantID:  tenantID,
		Value:     value,
		Label:     input.Label,
		SortOrder: 0,
	})
	if err != nil {
		return GuideCategory{}, fmt.Errorf("creating entity type: %w", err)
	}

	groupSlug := "guide_" + input.Slug + "_fields"
	group, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{
		TenantID:    tenantID,
		Name:        input.Label + " Fields",
		Slug:        groupSlug,
		Description: ptr("Custom fields for " + input.Label),
		IsSystem:    false,
	})
	if err != nil {
		return GuideCategory{}, fmt.Errorf("creating field group: %w", err)
	}

	_, err = q.CreateAssignment(ctx, gen.CreateAssignmentParams{
		GroupID:     group.ID,
		EntityClass: "content_type:guide:" + input.Slug,
		EntityID:    pgtype.UUID{},
		Position:    0,
	})
	if err != nil {
		return GuideCategory{}, fmt.Errorf("creating assignment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return GuideCategory{}, fmt.Errorf("committing tx: %w", err)
	}

	return GuideCategory{Slug: input.Slug, Label: input.Label}, nil
}

// DeleteGuideCategory removes a guide category and its associated custom field
// group. The entity_type, field group, and assignment are all deleted. System
// categories (is_system = true) cannot be deleted.
func (s *Service) DeleteGuideCategory(ctx context.Context, tenantID pgtype.UUID, slug string) error {
	value := "guide_category:" + slug

	// Find the entity type to get its ID and check is_system.
	et, err := s.read.GetCustomFieldEntityTypeByValue(ctx, gen.GetCustomFieldEntityTypeByValueParams{
		TenantID: tenantID,
		Value:    value,
	})
	if err != nil {
		return fmt.Errorf("finding guide category: %w", err)
	}
	if et.IsSystem {
		return fmt.Errorf("cannot delete system guide category")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := gen.New(tx)

	// Delete the field group (cascades to fields and assignments).
	groupSlug := "guide_" + slug + "_fields"
	group, err := q.GetCustomFieldGroupBySlug(ctx, gen.GetCustomFieldGroupBySlugParams{
		TenantID: tenantID,
		Slug:     groupSlug,
	})
	if err == nil {
		if err := q.DeleteCustomFieldGroup(ctx, gen.DeleteCustomFieldGroupParams{
			ID:       group.ID,
			TenantID: tenantID,
		}); err != nil {
			return fmt.Errorf("deleting field group: %w", err)
		}
	}

	// Delete the entity type.
	if _, err := q.DeleteCustomFieldEntityType(ctx, gen.DeleteCustomFieldEntityTypeParams{
		ID:       et.ID,
		TenantID: tenantID,
	}); err != nil {
		return fmt.Errorf("deleting entity type: %w", err)
	}

	return tx.Commit(ctx)
}

func ptr[T any](v T) *T { return &v }
