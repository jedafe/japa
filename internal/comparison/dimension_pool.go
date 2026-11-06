package comparison

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// dimensionField is the normalized shape dimension creation consumes —
// decoupled from which query produced the row (§30 Sub-spec 6).
type dimensionField struct {
	ID        pgtype.UUID
	GroupID   pgtype.UUID
	Label     string
	Key       string
	FieldType string
	Options   []byte
	Section   *string
}

func dimensionFieldsFromGroup(rows []gen.ListCustomFieldsByGroupRow) []dimensionField {
	out := make([]dimensionField, 0, len(rows))

	for _, r := range rows {
		out = append(out, dimensionField{
			ID:        r.ID,
			GroupID:   r.GroupID,
			Label:     r.Label,
			Key:       r.Key,
			FieldType: r.FieldType,
			Options:   r.Options,
			Section:   r.Section,
		})
	}

	return out
}

func parseUUIDList(ids []string) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(ids))
	seen := make(map[[16]byte]bool, len(ids))

	for _, s := range ids {
		id, err := resolver.ParseUUID(s)
		if err != nil {
			return nil, err
		}

		if seen[id.Bytes] {
			continue
		}

		seen[id.Bytes] = true
		out = append(out, id)
	}

	return out, nil
}

// poolFieldsByIDs resolves explicit field picks: every id must be a field in
// the type's dimension pool (§30 Sub-spec 6). Order follows the caller's
// array.
func (s *Service) poolFieldsByIDs(ctx context.Context, typeID pgtype.UUID, ids []string) ([]dimensionField, error) {
	parsed, err := parseUUIDList(ids)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid field_ids", errpkg.ErrInvalid)
	}

	n, err := s.store.countPoolFieldsByIDs(ctx, typeID, parsed)
	if err != nil {
		return nil, err
	}

	if int(n) != len(parsed) {
		return nil, fmt.Errorf("%w: field_ids contains fields outside this type's dimension pool", errpkg.ErrInvalid)
	}

	rows, err := s.store.listCustomFieldsByIDs(ctx, parsed)
	if err != nil {
		return nil, err
	}

	byID := make(map[[16]byte]gen.ListCustomFieldsByIDsRow, len(rows))

	for _, r := range rows {
		byID[r.ID.Bytes] = r
	}

	out := make([]dimensionField, 0, len(parsed))

	for _, id := range parsed {
		if r, ok := byID[id.Bytes]; ok {
			out = append(out, dimensionField{
				ID:        r.ID,
				GroupID:   r.GroupID,
				Label:     r.Label,
				Key:       r.Key,
				FieldType: r.FieldType,
				Options:   r.Options,
				Section:   r.Section,
			})
		}
	}

	return disambiguateFieldLabels(out, nil), nil
}

// poolFieldsByGroups expands requested groups after validating each is a pool
// member, then yields dims in pool order.
func (s *Service) poolFieldsByGroups(ctx context.Context, typeID pgtype.UUID, ids []string) ([]dimensionField, error) {
	parsed, err := parseUUIDList(ids)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid group_ids", errpkg.ErrInvalid)
	}

	pool, err := s.store.listComparisonTypeFieldGroups(ctx, typeID)
	if err != nil {
		return nil, err
	}

	inPool := make(map[[16]byte]string, len(pool))

	for _, g := range pool {
		label := g.GroupName
		if g.Label != nil && *g.Label != "" {
			label = *g.Label
		}

		inPool[g.GroupID.Bytes] = label
	}

	for _, id := range parsed {
		if _, ok := inPool[id.Bytes]; !ok {
			return nil, fmt.Errorf("%w: group_ids contains groups outside this type's dimension pool", errpkg.ErrInvalid)
		}
	}

	rows, err := s.store.listPoolFieldsByGroupIDs(ctx, typeID, parsed)
	if err != nil {
		return nil, err
	}

	out := make([]dimensionField, 0, len(rows))

	for _, r := range rows {
		out = append(out, dimensionField{
			ID:        r.ID,
			GroupID:   r.GroupID,
			Label:     r.Label,
			Key:       r.Key,
			FieldType: r.FieldType,
			Options:   r.Options,
			Section:   r.Section,
		})
	}

	return disambiguateFieldLabels(out, inPool), nil
}

// disambiguateFieldLabels suffixes " (group label)" when a label repeats —
// fields from different groups may share a label and would otherwise render
// identical matrix columns.
func disambiguateFieldLabels(fields []dimensionField, groupLabels map[[16]byte]string) []dimensionField {
	counts := make(map[string]int, len(fields))

	for _, f := range fields {
		counts[f.Label]++
	}

	for i, f := range fields {
		if counts[f.Label] > 1 {
			if gl := groupLabels[f.GroupID.Bytes]; gl != "" {
				fields[i].Label = f.Label + " (" + gl + ")"
			}
		}
	}

	return fields
}

// PoolField is one field entry in a type's dimension pool (§30 Sub-spec 6).
type PoolField struct {
	ID         string  `json:"id"`
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	FieldType  string  `json:"field_type"`
	Section    *string `json:"section,omitempty"`
	Comparable bool    `json:"comparable"`
}

// PoolGroup is one pool group with its fields for the /compare picker.
type PoolGroup struct {
	GroupID   string      `json:"group_id"`
	Slug      string      `json:"slug"`
	Label     string      `json:"label"`
	Category  *string     `json:"category,omitempty"`
	IsDefault bool        `json:"is_default"`
	Fields    []PoolField `json:"fields"`
}

// ListComparisonTypeFieldPool groups the type's dimension pool for the
// picker. Comparable = fieldToDimensionType != "" — non-comparable composites
// are returned flagged so the UI can disable rather than hide them.
func (s *Service) ListComparisonTypeFieldPool(ctx context.Context, typeID pgtype.UUID) ([]PoolGroup, error) {
	groups, err := s.store.listComparisonTypeFieldGroups(ctx, typeID)
	if err != nil {
		return nil, err
	}

	fields, err := s.store.listComparisonTypePoolFields(ctx, typeID)
	if err != nil {
		return nil, err
	}

	byGroup := make(map[[16]byte][]PoolField, len(groups))

	for _, f := range fields {
		byGroup[f.GroupID.Bytes] = append(byGroup[f.GroupID.Bytes], PoolField{
			ID:         f.ID.String(),
			Key:        f.Key,
			Label:      f.Label,
			FieldType:  f.FieldType,
			Section:    f.Section,
			Comparable: fieldToDimensionType(f.FieldType, f.Options) != "",
		})
	}

	out := make([]PoolGroup, 0, len(groups))

	for _, g := range groups {
		label := g.GroupName
		if g.Label != nil && *g.Label != "" {
			label = *g.Label
		}

		// Empty groups serialize as [] not null — the picker iterates fields.
		groupFields := byGroup[g.GroupID.Bytes]
		if groupFields == nil {
			groupFields = []PoolField{}
		}

		out = append(out, PoolGroup{
			GroupID:   g.GroupID.String(),
			Slug:      g.GroupSlug,
			Label:     label,
			Category:  g.Category,
			IsDefault: g.IsDefault,
			Fields:    groupFields,
		})
	}

	return out, nil
}

// replaceFieldGroupPool rewrites a type's dimension pool (Sub-spec 6): the
// bound field_group_id is always re-inserted as the default member.
func (s *Service) replaceFieldGroupPool(ctx context.Context, typeID pgtype.UUID, groupIDs []string) error {
	ids, err := parseUUIDList(groupIDs)
	if err != nil {
		return fmt.Errorf("%w: invalid field_group_ids", errpkg.ErrInvalid)
	}

	td, err := s.store.getComparisonTypeByID(ctx, typeID)
	if err != nil {
		return fmt.Errorf("resolving comparison type: %w", err)
	}

	return db.WithTx(ctx, s.db.Writer, func(q *gen.Queries) error {
		if err := q.DeleteComparisonTypeFieldGroups(ctx, typeID); err != nil {
			return fmt.Errorf("clearing dimension pool: %w", err)
		}

		if td.FieldGroupID.Valid {
			if err := q.CreateComparisonTypeFieldGroup(ctx, gen.CreateComparisonTypeFieldGroupParams{
				TypeID:    typeID,
				GroupID:   td.FieldGroupID,
				Position:  -1,
				IsDefault: true,
			}); err != nil {
				return fmt.Errorf("seeding default pool group: %w", err)
			}
		}

		for i, gid := range ids {
			if td.FieldGroupID.Valid && gid.Bytes == td.FieldGroupID.Bytes {
				continue
			}

			if err := q.CreateComparisonTypeFieldGroup(ctx, gen.CreateComparisonTypeFieldGroupParams{
				TypeID:   typeID,
				GroupID:  gid,
				Position: int32(i),
			}); err != nil {
				return fmt.Errorf("adding pool group %d: %w", i, err)
			}
		}

		return nil
	})
}
