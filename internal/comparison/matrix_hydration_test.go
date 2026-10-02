package comparison

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/comparison/resolver"
	"github.com/jedafe/japa/internal/db/gen"
)

// matrixFakeRows implements pgx.Rows for Query tests, assigning values to scan
// destinations by reflection (same technique as the resolver package tests).
type matrixFakeRows struct {
	rows   [][]any
	pos    int
	closed bool
}

func (r *matrixFakeRows) Close()                                       { r.closed = true }
func (r *matrixFakeRows) Err() error                                   { return nil }
func (r *matrixFakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *matrixFakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *matrixFakeRows) Next() bool {
	if r.closed {
		return false
	}

	r.pos++

	return r.pos <= len(r.rows)
}

func (r *matrixFakeRows) Scan(dest ...any) error {
	if r.pos < 1 || r.pos > len(r.rows) {
		return errors.New("no current row")
	}

	row := r.rows[r.pos-1]
	if len(dest) != len(row) {
		return fmt.Errorf("destination count %d does not match value count %d", len(dest), len(row))
	}

	for i, v := range row {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(v))
	}

	return nil
}

func (r *matrixFakeRows) Values() ([]any, error) { return r.rows[r.pos-1], nil }
func (r *matrixFakeRows) RawValues() [][]byte    { return nil }
func (r *matrixFakeRows) Conn() *pgx.Conn        { return nil }

// matrixFakeDB routes the three matrix queries to programmed row sets by
// matching the FROM clause of each generated query.
type matrixFakeDB struct {
	itemRows [][]any
	dimRows  [][]any
	valRows  [][]any
	queries  []string
}

func (f *matrixFakeDB) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (f *matrixFakeDB) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	f.queries = append(f.queries, sql)

	switch {
	case strings.Contains(sql, "FROM comparison_items"):
		return &matrixFakeRows{rows: f.itemRows}, nil
	case strings.Contains(sql, "FROM comparison_dimensions"):
		return &matrixFakeRows{rows: f.dimRows}, nil
	case strings.Contains(sql, "FROM comparison_values"):
		return &matrixFakeRows{rows: f.valRows}, nil
	}

	return &matrixFakeRows{}, nil
}

func (f *matrixFakeDB) QueryRow(context.Context, string, ...any) pgx.Row { return nil }

func (f *matrixFakeDB) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

// stubResolver is a fake resolver.Resolver returning a fixed item.
type stubResolver struct {
	item resolver.ResolvedItem
}

func (s stubResolver) Resolve(context.Context, map[string]any) (resolver.ResolvedItem, bool, error) {
	return s.item, true, nil
}

func (s stubResolver) Search(context.Context, string, map[string]any, int32, int32) ([]resolver.ResolvedItem, error) {
	return nil, nil
}

func matrixUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()

	u, err := uuid.Parse(s)
	if err != nil {
		t.Fatalf("bad test uuid %q: %v", s, err)
	}

	return pgtype.UUID{Bytes: u, Valid: true}
}

func strPtr(s string) *string { return &s }

func TestGetComparisonMatrix_HydratesFromEntityField(t *testing.T) {
	t.Parallel()

	contentID := matrixUUID(t, "00000000-0000-0000-0000-00000000c001")
	itemID := matrixUUID(t, "00000000-0000-0000-0000-00000000a001")
	uniID := "00000000-0000-0000-0000-00000000b001"

	dimRatingID := matrixUUID(t, "00000000-0000-0000-0000-00000000d001")
	dimTextID := matrixUUID(t, "00000000-0000-0000-0000-00000000d002")
	dimBoolID := matrixUUID(t, "00000000-0000-0000-0000-00000000d003")
	dimMissingID := matrixUUID(t, "00000000-0000-0000-0000-00000000d004")

	fieldID := matrixUUID(t, "00000000-0000-0000-0000-00000000f001")
	ts := pgtype.Timestamptz{}

	itemRef := []byte(`{"type":"university","id":"` + uniID + `"}`)

	fake := &matrixFakeDB{
		// id, content_id, type_id, title, description, image_id, geo_space_id,
		// position, item_ref, created_at, deleted_at
		itemRows: [][]any{
			{itemID, contentID, pgtype.UUID{}, "Uni A", (*string)(nil), pgtype.UUID{}, pgtype.UUID{}, int32(0), itemRef, ts, ts},
		},
		// id, content_id, type_id, label, dimension_type, position,
		// source_field_id, source_field_key, source_field_options,
		// created_at, deleted_at
		dimRows: [][]any{
			{dimRatingID, contentID, pgtype.UUID{}, "QS Ranking", DimensionTypeRating, int32(0), fieldID, strPtr("qs_ranking"), []byte(nil), ts, ts},
			{dimTextID, contentID, pgtype.UUID{}, "Overview", DimensionTypeText, int32(1), fieldID, strPtr("overview_text"), []byte(nil), ts, ts},
			{dimBoolID, contentID, pgtype.UUID{}, "Scholarships", DimensionTypeBoolean, int32(2), fieldID, strPtr("scholarship_available"), []byte(nil), ts, ts},
			{dimMissingID, contentID, pgtype.UUID{}, "Missing", DimensionTypeText, int32(3), fieldID, strPtr("not_a_field"), []byte(nil), ts, ts},
		},
		// dimension_id, item_id, value, value_rating, value_boolean,
		// created_at, metadata, updated_at, data_source_id
		valRows: [][]any{
			{dimTextID, itemID, strPtr("persisted overview"), pgtype.Numeric{}, (*bool)(nil), ts, []byte(nil), ts, pgtype.UUID{}},
		},
	}

	reg := resolver.NewRegistry()
	reg.Register("university", stubResolver{item: resolver.ResolvedItem{
		Type:  "university",
		ID:    uniID,
		Title: "Uni A",
		Metadata: map[string]any{
			"field_values": map[string]any{
				"qs_ranking":            350.0,
				"overview_text":         "hydrated overview",
				"scholarship_available": true,
			},
		},
	}})

	svc := &Service{
		store:    NewStore(gen.New(fake), gen.New(fake)),
		resolver: reg,
	}

	matrix, err := svc.GetComparisonMatrix(t.Context(), contentID)
	if err != nil {
		t.Fatalf("GetComparisonMatrix failed: %v", err)
	}

	if len(matrix) != 1 {
		t.Fatalf("expected 1 matrix item, got %d", len(matrix))
	}

	dims := matrix[0].Dimensions
	if len(dims) != 4 {
		t.Fatalf("expected 4 dimensions, got %d", len(dims))
	}

	// Rating dimension: hydrated from field_values.qs_ranking.
	if dims[0].ValueRating == nil || *dims[0].ValueRating != 350.0 {
		t.Fatalf("expected hydrated rating 350, got %+v", dims[0].ValueRating)
	}
	if dims[0].HydratedFrom != "entity_field" {
		t.Fatalf("expected hydrated_from=entity_field, got %q", dims[0].HydratedFrom)
	}

	// Text dimension: persisted comparison_values row wins over hydration.
	if dims[1].Value == nil || *dims[1].Value != "persisted overview" {
		t.Fatalf("expected persisted value to win, got %+v", dims[1].Value)
	}
	if dims[1].HydratedFrom != "" {
		t.Fatalf("persisted cell must not be marked hydrated, got %q", dims[1].HydratedFrom)
	}

	// Boolean dimension: hydrated from field_values.scholarship_available.
	if dims[2].ValueBoolean == nil || *dims[2].ValueBoolean != true {
		t.Fatalf("expected hydrated boolean true, got %+v", dims[2].ValueBoolean)
	}
	if dims[2].HydratedFrom != "entity_field" {
		t.Fatalf("expected hydrated_from=entity_field, got %q", dims[2].HydratedFrom)
	}

	// Missing source key in field_values: cell stays empty, no hydration flag.
	if dims[3].Value != nil || dims[3].ValueRating != nil || dims[3].ValueBoolean != nil {
		t.Fatalf("expected empty cell for unknown field key, got %+v", dims[3])
	}
	if dims[3].HydratedFrom != "" {
		t.Fatalf("unexpected hydrated_from for missing field, got %q", dims[3].HydratedFrom)
	}
}

// TestGetComparisonMatrix_HydrationSkipsUnresolvableItem verifies that a
// resolver error on an item leaves its cells empty instead of failing the
// whole matrix.
func TestGetComparisonMatrix_HydrationSkipsUnresolvableItem(t *testing.T) {
	t.Parallel()

	contentID := matrixUUID(t, "00000000-0000-0000-0000-00000000c001")
	itemID := matrixUUID(t, "00000000-0000-0000-0000-00000000a002")
	dimID := matrixUUID(t, "00000000-0000-0000-0000-00000000d001")
	fieldID := matrixUUID(t, "00000000-0000-0000-0000-00000000f001")
	ts := pgtype.Timestamptz{}

	itemRef := []byte(`{"type":"university","id":"00000000-0000-0000-0000-00000000b404"}`)

	fake := &matrixFakeDB{
		itemRows: [][]any{
			{itemID, contentID, pgtype.UUID{}, "Gone Uni", (*string)(nil), pgtype.UUID{}, pgtype.UUID{}, int32(0), itemRef, ts, ts},
		},
		dimRows: [][]any{
			{dimID, contentID, pgtype.UUID{}, "QS Ranking", DimensionTypeRating, int32(0), fieldID, strPtr("qs_ranking"), []byte(nil), ts, ts},
		},
	}

	// A resolver that reports every ref as missing: Registry.Resolve turns
	// found=false into an error, exercising the "must not fail" path.
	reg := resolver.NewRegistry()
	reg.Register("university", stubNotFoundResolver{})

	svc := &Service{
		store:    NewStore(gen.New(fake), gen.New(fake)),
		resolver: reg,
	}

	matrix, err := svc.GetComparisonMatrix(t.Context(), contentID)
	if err != nil {
		t.Fatalf("GetComparisonMatrix must not fail on unresolvable items: %v", err)
	}

	if len(matrix) != 1 || len(matrix[0].Dimensions) != 1 {
		t.Fatalf("unexpected matrix shape: %+v", matrix)
	}

	d := matrix[0].Dimensions[0]
	if d.Value != nil || d.ValueRating != nil || d.ValueBoolean != nil || d.HydratedFrom != "" {
		t.Fatalf("expected empty unhydrated cell, got %+v", d)
	}
}

// stubNotFoundResolver reports every ref as missing.
type stubNotFoundResolver struct{}

func (stubNotFoundResolver) Resolve(context.Context, map[string]any) (resolver.ResolvedItem, bool, error) {
	return resolver.ResolvedItem{}, false, nil
}

func (stubNotFoundResolver) Search(context.Context, string, map[string]any, int32, int32) ([]resolver.ResolvedItem, error) {
	return nil, nil
}
