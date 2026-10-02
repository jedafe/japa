// Package wiki — fieldops.go defines the field-ops vocabulary for wiki
// community-field revisions (ops JSONB array applied in order on approval,
// inside one transaction) plus the structural (no-DB) validation run at
// submit time. Semantic checks — target field exists, value coerces to the
// field type's value column, submitter's can_edit permission — live in the
// service layer where the database is available.
// // Ref: docs/superpowers/specs/2026-09-24-wiki-community-fields-design.md §5
package wiki

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/jedafe/japa/internal/customfields"
)

// Field-op names accepted in a revision's ops payload (§5).
const (
	OpSetValue     = "set_value"
	OpProposeField = "propose_field"
	OpProposeGroup = "propose_group"
	OpUpsertField  = "upsert_field"
	OpAssignGroup  = "assign_group"
)

// FieldOp is a single entry in a revision's ops array. Fields are
// op-dependent — e.g. set_value uses entity_type/entity_id/field_key/value,
// while propose_group uses group + entity_class. All optional members are
// omitempty so a decoded op round-trips without phantom keys.
type FieldOp struct {
	Op          string          `json:"op"`
	EntityType  string          `json:"entity_type,omitempty"`
	EntityID    string          `json:"entity_id,omitempty"`
	FieldKey    string          `json:"field_key,omitempty"`
	GroupSlug   string          `json:"group_slug,omitempty"`
	EntityClass string          `json:"entity_class,omitempty"`
	SourceURL   string          `json:"source_url,omitempty"`
	Value       json.RawMessage `json:"value,omitempty"`
	Field       *FieldDefInput  `json:"field,omitempty"`
	Group       *GroupDefInput  `json:"group,omitempty"`
}

// FieldDefInput is the field definition payload carried by propose_field,
// upsert_field, and each entry of a propose_group's fields list.
type FieldDefInput struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	FieldType string `json:"field_type"`
	// Options is the raw options payload — a [{value,label}] array for
	// select/multiselect, the object-shaped {parts,primary_part} schema for
	// price_parts (§80 Wiki Topic Pages), free-form objects for the rest.
	Options      json.RawMessage `json:"options,omitempty"`
	IsRequired   bool            `json:"is_required,omitempty"`
	Position     int32           `json:"position,omitempty"`
	Placeholder  string          `json:"placeholder,omitempty"`
	DefaultValue string          `json:"default_value,omitempty"`
	// Section is the §80 itemized-pricing sub-grouping token written to
	// custom_fields.section — fields sharing a token render as one item
	// table (e.g. 'transportation'). Empty = no section.
	Section string `json:"section,omitempty"`
}

// GroupDefInput is the group definition payload carried by propose_group.
type GroupDefInput struct {
	Slug        string          `json:"slug"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Fields      []FieldDefInput `json:"fields,omitempty"`
}

// ParseOps decodes a raw ops JSONB payload into a FieldOp slice. A nil,
// empty, or "[]" payload yields an empty slice and no error — revisions with
// no ops are plain content revisions.
func ParseOps(raw []byte) ([]FieldOp, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return []FieldOp{}, nil
	}

	var ops []FieldOp
	if err := json.Unmarshal(raw, &ops); err != nil {
		return nil, fmt.Errorf("parsing ops payload: %w", err)
	}

	if ops == nil {
		ops = []FieldOp{}
	}

	return ops, nil
}

// ValidateOps performs structural validation on an ops array — no database
// access. maxOps bounds the submission (wiki.suggest.max_ops_per_revision);
// maxOps <= 0 means unlimited (tests only).
func ValidateOps(ops []FieldOp, maxOps int) error {
	if maxOps > 0 && len(ops) > maxOps {
		return fmt.Errorf("ops count %d exceeds maximum %d", len(ops), maxOps)
	}

	for i, op := range ops {
		if err := validateOp(op); err != nil {
			return fmt.Errorf("ops[%d]: %w", i, err)
		}
	}

	return nil
}

// validateOp checks one op's required members per the §5 vocabulary.
func validateOp(op FieldOp) error {
	switch op.Op {
	case OpSetValue:
		if op.EntityType == "" {
			return fmt.Errorf("set_value requires entity_type")
		}

		if op.EntityID == "" {
			return fmt.Errorf("set_value requires entity_id")
		}

		if _, err := uuid.Parse(op.EntityID); err != nil {
			return fmt.Errorf("set_value entity_id is not a UUID: %w", err)
		}

		if op.FieldKey == "" {
			return fmt.Errorf("set_value requires field_key")
		}

		if len(bytes.TrimSpace(op.Value)) == 0 {
			return fmt.Errorf("set_value requires a non-empty value")
		}

		return nil

	case OpProposeField, OpUpsertField:
		if op.GroupSlug == "" {
			return fmt.Errorf("%s requires group_slug", op.Op)
		}

		return validateFieldDef(op.Field, op.Op+" field", op.Op == OpProposeField)

	case OpProposeGroup:
		if op.Group == nil {
			return fmt.Errorf("propose_group requires a group payload")
		}

		if op.Group.Slug == "" {
			return fmt.Errorf("propose_group requires group.slug")
		}

		if op.Group.Name == "" {
			return fmt.Errorf("propose_group requires group.name")
		}

		if op.EntityClass == "" {
			return fmt.Errorf("propose_group requires entity_class")
		}

		for j := range op.Group.Fields {
			f := op.Group.Fields[j]
			if err := validateFieldDef(&f, fmt.Sprintf("group.fields[%d]", j), true); err != nil {
				return err
			}
		}

		return nil

	case OpAssignGroup:
		if op.EntityClass == "" {
			return fmt.Errorf("assign_group requires entity_class")
		}

		if op.GroupSlug == "" {
			return fmt.Errorf("assign_group requires group_slug")
		}

		return nil

	default:
		return fmt.Errorf("unknown op %q", op.Op)
	}
}

// validateFieldDef checks a field definition payload: required members, a
// recognized field_type, and options shaped correctly for that type. When
// proposable is true (propose-tier ops), the field type must also be in
// customfields.ProposableFieldTypes — the same list GET /wiki/field-types
// advertises.
func validateFieldDef(f *FieldDefInput, where string, proposable bool) error {
	if f == nil {
		return fmt.Errorf("%s is required", where)
	}

	if f.Key == "" {
		return fmt.Errorf("%s requires key", where)
	}

	if f.Label == "" {
		return fmt.Errorf("%s requires label", where)
	}

	if !customfields.IsValidFieldType(f.FieldType) {
		return fmt.Errorf("%s has unknown field_type %q", where, f.FieldType)
	}

	if proposable && !slices.Contains(customfields.ProposableFieldTypes(), f.FieldType) {
		return fmt.Errorf("%s field_type %q is not proposable", where, f.FieldType)
	}

	if msg, ok := customfields.ValidOptionsShape(f.FieldType, f.Options); !ok {
		return fmt.Errorf("%s: %s", where, msg)
	}

	if !customfields.ValidSectionToken(f.Section) {
		return fmt.Errorf("%s has invalid section %q (want ^[a-z0-9_]+$)", where, f.Section)
	}

	return nil
}

// DeriveKind picks the revision kind from whether content and ops are both
// present: content-only → "content", ops-only → "fields", both → "mixed"
// (§7 route contract).
func DeriveKind(hasContent bool, ops []FieldOp) string {
	hasOps := len(ops) > 0

	switch {
	case hasContent && hasOps:
		return "mixed"
	case hasOps:
		return "fields"
	default:
		return "content"
	}
}
