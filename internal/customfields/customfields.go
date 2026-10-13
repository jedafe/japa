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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	FieldTypeText       = "text"
	FieldTypeTextarea   = "textarea"
	FieldTypeRichText   = "rich_text"
	FieldTypeNumber     = "number"
	FieldTypeCurrency   = "currency"
	FieldTypePercentage = "percentage"
	FieldTypeBoolean    = "boolean"
	FieldTypeDate       = "date"
	FieldTypeDatetime   = "datetime"
	FieldTypeTime       = "time"
	FieldTypeDuration   = "duration"
	FieldTypeSelect     = "select"
	// FieldTypeSuggest is a type-ahead single-value field: renders an
	// autocomplete over the options list and accepts free text when the
	// typed value is not an option (§06; used by story_fields for
	// visa_category and profession).
	FieldTypeSuggest       = "suggest"
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
	// FieldTypePriceRange is a composite price datum — one field carries an
	// item's {"avg","min","max"} object in value_json (§80 Hierarchical UX &
	// Itemized Pricing amendment): each field is one row of an itemized
	// prices table, and the shared page-template group keeps the keys
	// comparable across spaces for JCE.
	FieldTypePriceRange = "price_range"
	// FieldTypePriceParts is a multi-part composite price datum — one field
	// carries {"parts": {"<part_key>": <number|string>, ...}} in value_json
	// (§80 Wiki Topic Pages amendment): the field's options declare the
	// column schema (options.parts) so one field renders as one row of a
	// heterogeneously-columned item table (e.g. Taxis: Service | Base Fare
	// | per km | per min | Minimum).
	FieldTypePriceParts = "price_parts"
)

// FieldOption represents a single option in a select or multiselect field.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// PriceParts part kinds — how one column of a price_parts value coerces and
// formats (§80 Wiki Topic Pages).
const (
	PricePartKindMoney  = "money"  // numeric, formatted via the currency chain
	PricePartKindNumber = "number" // numeric, bare
	PricePartKindText   = "text"   // free text column
)

// PricePartsPart is one column of a price_parts field's options.parts
// schema: key is the part's key in value_json.parts, kind governs coercion
// (money/number → numeric, text → string) and rendering. An empty kind
// defaults to number.
type PricePartsPart struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind,omitempty"`
}

// PricePartsOptions is the object-shaped options payload of a price_parts
// field: parts declares the column schema, primary_part names the
// comparable part for JCE/rollup projection, currency_code denominates
// money parts (§58 fallback: host space's country currency), and emphasize
// marks a total row (e.g. "Total Estimated Monthly Cost").
type PricePartsOptions struct {
	Parts        []PricePartsPart `json:"parts"`
	PrimaryPart  string           `json:"primary_part,omitempty"`
	CurrencyCode string           `json:"currency_code,omitempty"`
	Emphasize    bool             `json:"emphasize,omitempty"`
}

// PricePartsOptionsFromJSON decodes a price_parts field's options column;
// nil on absent/malformed options.
func PricePartsOptionsFromJSON(options []byte) *PricePartsOptions {
	if len(options) == 0 {
		return nil
	}
	var o PricePartsOptions
	if err := json.Unmarshal(options, &o); err != nil || len(o.Parts) == 0 {
		return nil
	}
	return &o
}

// PrimaryPartFromOptions returns the field's declared comparable part key —
// "" when the field declares none (non-comparable, §80 §7) or options are
// malformed.
func PrimaryPartFromOptions(options []byte) string {
	if o := PricePartsOptionsFromJSON(options); o != nil {
		return o.PrimaryPart
	}
	return ""
}

// PricePartKind returns the declared kind for a part key — "number" when
// the part is undeclared or carries no kind (the default).
func (o *PricePartsOptions) PricePartKind(key string) string {
	if o != nil {
		for _, p := range o.Parts {
			if p.Key == key && p.Kind != "" {
				return p.Kind
			}
		}
	}
	return PricePartKindNumber
}

// CurrencyCodeFromOptions extracts the ISO 4217 code from a field's options
// JSON ({currency_code: "USD"}) — the binding that denominates currency and
// price_range field values (§58). Empty when the field declares none or the
// JSON is malformed.
func CurrencyCodeFromOptions(options []byte) string {
	if len(options) == 0 {
		return ""
	}
	var o struct {
		CurrencyCode string `json:"currency_code"`
	}
	if err := json.Unmarshal(options, &o); err != nil {
		return ""
	}
	return o.CurrencyCode
}

// validFieldTypes is the set of all supported field types.
var validFieldTypes = map[string]bool{
	FieldTypeText: true, FieldTypeTextarea: true, FieldTypeRichText: true,
	FieldTypeNumber: true, FieldTypeCurrency: true, FieldTypePercentage: true,
	FieldTypeBoolean: true, FieldTypeDate: true, FieldTypeDatetime: true,
	FieldTypeTime: true, FieldTypeDuration: true,
	FieldTypeSelect: true, FieldTypeSuggest: true, FieldTypeMultiselect: true,
	FieldTypeURL: true, FieldTypeEmail: true, FieldTypePhone: true,
	FieldTypeColor: true, FieldTypeRating: true,
	FieldTypeImageSingle: true, FieldTypeImageMulti: true, FieldTypeMedia: true,
	FieldTypePlatformEmbed: true, FieldTypeReference: true,
	FieldTypeRepeater: true, FieldTypeGroup: true,
	FieldTypePriceRange: true, FieldTypePriceParts: true,
}

// IsValidFieldType returns true if the given string is a recognized field type.
func IsValidFieldType(ft string) bool {
	return validFieldTypes[ft]
}

// FieldTypeInfo describes one field type for admin pickers — the §06
// catalog surface. NeedsOptions mirrors the create/update validation that
// requires an options array for select/multiselect.
type FieldTypeInfo struct {
	Value        string `json:"value"`
	Label        string `json:"label"`
	NeedsOptions bool   `json:"needs_options"`
}

// FieldTypeCatalog returns every valid field type with its admin label, in
// a stable display order. Single source of truth for the admin field-type
// dropdown — the frontend consumes GET /admin/field-types rather than
// hardcoding this list (Rule 16).
func FieldTypeCatalog() []FieldTypeInfo {
	return []FieldTypeInfo{
		{FieldTypeText, "Text", false},
		{FieldTypeTextarea, "Textarea", false},
		{FieldTypeRichText, "Rich Text", false},
		{FieldTypeNumber, "Number", false},
		{FieldTypeCurrency, "Currency", false},
		{FieldTypePercentage, "Percentage", false},
		{FieldTypeBoolean, "Boolean (Toggle)", false},
		{FieldTypeDate, "Date", false},
		{FieldTypeDatetime, "Date & Time", false},
		{FieldTypeTime, "Time", false},
		{FieldTypeDuration, "Duration", false},
		{FieldTypeSelect, "Select (Single)", true},
		{FieldTypeSuggest, "Suggest (Single with Suggestions)", false},
		{FieldTypeMultiselect, "Multi-Select", true},
		{FieldTypeURL, "URL", false},
		{FieldTypeEmail, "Email", false},
		{FieldTypePhone, "Phone", false},
		{FieldTypeColor, "Color Picker", false},
		{FieldTypeRating, "Rating (1-5)", false},
		{FieldTypeImageSingle, "Image (Single Upload)", false},
		{FieldTypeImageMulti, "Images (Multi Upload)", false},
		{FieldTypeMedia, "Media (File Upload)", false},
		{FieldTypePlatformEmbed, "Platform Embed", false},
		{FieldTypeReference, "Reference (Link to Entity)", false},
		{FieldTypeRepeater, "Repeater (Add Multiple Items)", false},
		{FieldTypeGroup, "Group (Visual Container)", false},
		{FieldTypePriceRange, "Price Range (Min-Max)", false},
		{FieldTypePriceParts, "Itemized Price Parts", false},
	}
}

// SectionTokenPattern constrains custom_fields.section — the §80
// itemized-pricing sub-grouping token — to a URL/query-safe identifier.
var SectionTokenPattern = regexp.MustCompile(`^[a-z0-9_]+$`)

// ValidSectionToken reports whether s is a usable section token; the empty
// string is valid (it means "no section").
func ValidSectionToken(s string) bool {
	return s == "" || SectionTokenPattern.MatchString(s)
}

// ProposableFieldTypes returns the field types community members may propose
// through §80 field ops — the leaf value-carrying types plus the upload and
// structural types whose options are optional at proposal time (image and
// media uploads, repeater, group). platform_embed and reference stay
// excluded: an embed is equivalent to a URL for community purposes, and a
// reference requires an entity-class binding a text proposal cannot express.
func ProposableFieldTypes() []string {
	return []string{
		FieldTypeText, FieldTypeTextarea, FieldTypeRichText,
		FieldTypeNumber, FieldTypeCurrency, FieldTypePercentage,
		FieldTypeBoolean, FieldTypeDate, FieldTypeDatetime, FieldTypeTime,
		FieldTypeDuration,
		FieldTypeSelect, FieldTypeSuggest, FieldTypeMultiselect,
		FieldTypeURL, FieldTypeEmail, FieldTypePhone,
		FieldTypeColor, FieldTypeRating,
		FieldTypeImageSingle, FieldTypeImageMulti, FieldTypeMedia,
		FieldTypeRepeater, FieldTypeGroup,
		FieldTypePriceRange, FieldTypePriceParts,
	}
}

// FieldTypeValueColumn maps a field type to the custom_field_values column it
// expects to write to. Returns the column name and true, or "" and false for
// unknown types.
func FieldTypeValueColumn(ft string) (column string, ok bool) {
	switch ft {
	case FieldTypeText, FieldTypeTextarea, FieldTypeURL, FieldTypeEmail,
		FieldTypePhone, FieldTypeColor, FieldTypePlatformEmbed, FieldTypeSuggest:
		return "value_text", true
	case FieldTypeRichText, FieldTypeSelect, FieldTypeMultiselect,
		FieldTypeMedia, FieldTypeImageSingle, FieldTypeImageMulti,
		FieldTypeReference, FieldTypeRepeater, FieldTypePriceRange,
		FieldTypePriceParts:
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
// shape for the given field_type. opts is the raw request/storage JSON:
// select/multiselect carry a [{value,label}] array, price_parts carries the
// object-shaped {parts, primary_part, currency_code} schema (§80 Wiki Topic
// Pages), and other types carry free-form objects or nothing. Returns a
// user-facing error message and false when validation fails, or "" and true
// when it passes.
func ValidOptionsShape(fieldType string, opts json.RawMessage) (msg string, ok bool) {
	empty := len(opts) == 0 || string(opts) == "null"

	switch fieldType {
	case FieldTypeSelect, FieldTypeMultiselect:
		if empty {
			return "Select and multiselect fields require at least one option", false
		}
		var list []FieldOption
		if err := json.Unmarshal(opts, &list); err != nil {
			return "Options must be an array of {value, label}", false
		}
		if len(list) == 0 {
			return "Select and multiselect fields require at least one option", false
		}

		for _, o := range list {
			if o.Value == "" || o.Label == "" {
				return "Each option must have both value and label", false
			}
		}
	case FieldTypePriceParts:
		return validPricePartsOptions(opts)
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

// validPricePartsOptions enforces the price_parts options contract
// (§80 Wiki Topic Pages): a non-empty parts array of {key,label,kind}
// columns — key matching ^[a-z0-9_]+$ and unique, kind ∈ money|number|text
// — and an optional primary_part naming a declared numeric part (the JCE /
// rollup projection point). price_parts without a parts schema cannot
// coerce values, so options are required.
func validPricePartsOptions(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "price_parts fields require options.parts — the column schema", false
	}

	var o PricePartsOptions
	if err := json.Unmarshal(raw, &o); err != nil {
		return "price_parts options must be an object with a parts array", false
	}
	if len(o.Parts) == 0 {
		return "price_parts requires at least one part in options.parts", false
	}

	seen := make(map[string]bool, len(o.Parts))
	kinds := make(map[string]string, len(o.Parts))
	for _, p := range o.Parts {
		if !SectionTokenPattern.MatchString(p.Key) {
			return "Each part key must match ^[a-z0-9_]+$", false
		}
		if p.Label == "" {
			return "Each part must have both key and label", false
		}
		if seen[p.Key] {
			return "Part keys must be unique", false
		}
		seen[p.Key] = true

		kind := p.Kind
		if kind == "" {
			kind = PricePartKindNumber
		}
		if kind != PricePartKindMoney && kind != PricePartKindNumber && kind != PricePartKindText {
			return "Part kind must be money, number, or text", false
		}
		kinds[p.Key] = kind
	}

	if o.PrimaryPart != "" {
		kind, declared := kinds[o.PrimaryPart]
		if !declared {
			return "primary_part must name a declared part", false
		}
		if kind == PricePartKindText {
			return "primary_part must name a money or number part — text is not comparable", false
		}
	}

	return "", true
}

// SupportedEmbedPlatforms is the list of platforms supported by the
// platform_embed field type.
var SupportedEmbedPlatforms = []string{
	"youtube", "github", "twitter", "vimeo", "spotify", "figma", "codepen", "google_maps",
}

// EntityTypeFromClass derives a simplified entity_type token from a full
// entity_class assignment string (Section 06 / CF17). This is the single
// entity-class resolver for customfields-side reads — promoted from the
// wiki package's entityClassesFor convention: wiki resolves item_ref types
// to assignment-class tokens for schema reads, and this resolves those
// class tokens back to the frontend-facing entity_type. CF handlers call
// this (not a local copy) so both sides share the mapping matrix:
//
//	entity_class                    → entity_type
//	content_type[:slug]             → content
//	content_type:guide:{slug}       → guide_category:{slug}
//	space_type[:slug|:id]           → space
//	ticket_type[:…]                 → ticket_type
//	user_profile|organization|...   → itself
//
// For example:
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

// entityTypeSpace is the custom_field_values entity_type under which all
// space-backed entities store their values (§06).
const entityTypeSpace = "space"

// ValueEntityType normalizes an entity type to the custom_field_values
// entity_type convention (§06/CF11): space-backed types all store under
// 'space'. This is the single resolver for the entity-class question on the
// customfields side — wiki's entityClassesFor maps item_ref types to
// assignment-class tokens, and this maps them to the value bucket. The
// canonical-per-entity-type table:
//
//	space/university/country/state/city → bucket 'space' (shared cfv rows)
//	organization/bank                   → bucket 'organization'
//	content (+ content:{slug})          → bucket 'content'
//	guide_category:{slug}               → bucket 'guide_category:{slug}'
//	everything else                     → bucket 'jce_entity:{type}' read,
//	                                      stored under its own entity_type
//
// Read precedence: normalized bucket first (canonical), denormalized
// per-entity field_values columns are read-cache mirrors only and must never
// override a cfv row (CF11; divergence resolves in favor of cfv).
func ValueEntityType(entityType string) string {
	switch entityType {
	case entityTypeSpace, "university", "country", "state", "city":
		return entityTypeSpace
	default:
		return entityType
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

// GetCustomFieldByKey looks up a custom field definition by its key within
// a platform. Used by the organizations handler to read the org_type select
// field's options from the custom field system (Section 07 + 08).
func (s *Service) GetCustomFieldByKey(ctx context.Context, key string) (gen.GetCustomFieldByKeyRow, error) {
	return s.read.GetCustomFieldByKey(ctx,
		key,
	)
}

// ListGuideCategories returns all guide categories for a platform. Guide
// categories are stored in custom_field_entity_types with value prefix
// 'guide_category:'. The slug is the part after the prefix. Also enriches
// each category with its field group ID so the admin panel can link to the
// field editor.
func (s *Service) ListGuideCategories(ctx context.Context) ([]GuideCategory, error) {
	rows, err := s.read.ListCustomFieldEntityTypesByPrefix(ctx,

		"guide_category:%",
	)
	if err != nil {
		return nil, fmt.Errorf("listing guide categories: %w", err)
	}

	cats := make([]GuideCategory, 0, len(rows))

	for _, row := range rows {
		slug := strings.TrimPrefix(row.Value, "guide_category:")
		cat := GuideCategory{Slug: slug, Label: row.Label, IsSystem: row.IsSystem}

		// Try to find the associated field group (slug = "guide_{slug}_fields").
		groupSlug := "guide_" + slug + "_fields"

		group, err := s.read.GetCustomFieldGroupBySlug(ctx,

			groupSlug,
		)
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
func (s *Service) CreateGuideCategory(ctx context.Context, input CreateGuideCategoryInput) (GuideCategory, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GuideCategory{}, fmt.Errorf("beginning tx: %w", err)
	}
	defer tx.Rollback(ctx)

	q := gen.New(tx)

	value := "guide_category:" + input.Slug

	_, err = q.CreateCustomFieldEntityType(ctx, gen.CreateCustomFieldEntityTypeParams{

		Value:     value,
		Label:     input.Label,
		SortOrder: 0,
	})
	if err != nil {
		return GuideCategory{}, fmt.Errorf("creating entity type: %w", err)
	}

	groupSlug := "guide_" + input.Slug + "_fields"

	group, err := q.CreateCustomFieldGroup(ctx, gen.CreateCustomFieldGroupParams{

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
func (s *Service) DeleteGuideCategory(ctx context.Context, slug string) error {
	value := "guide_category:" + slug

	// Find the entity type to get its ID and check is_system.
	et, err := s.read.GetCustomFieldEntityTypeByValue(ctx,

		value,
	)
	if err != nil {
		return fmt.Errorf("finding guide category: %w", err)
	}

	if et.IsSystem {
		return errors.New("cannot delete system guide category")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning tx: %w", err)
	}

	defer tx.Rollback(ctx)
	q := gen.New(tx)

	// Delete the field group (cascades to fields and assignments).
	groupSlug := "guide_" + slug + "_fields"

	group, err := q.GetCustomFieldGroupBySlug(ctx,

		groupSlug,
	)
	if err == nil {
		if err := q.DeleteCustomFieldGroup(ctx,
			group.ID,
		); err != nil {
			return fmt.Errorf("deleting field group: %w", err)
		}
	}

	// Delete the entity type.
	if _, err := q.DeleteCustomFieldEntityType(ctx,
		et.ID,
	); err != nil {
		return fmt.Errorf("deleting entity type: %w", err)
	}

	return tx.Commit(ctx)
}

func ptr[T any](v T) *T { return &v }
