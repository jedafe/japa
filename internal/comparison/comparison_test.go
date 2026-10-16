package comparison

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestDimensionTypeConstants(t *testing.T) {
	t.Parallel()

	expected := map[string]string{
		DimensionTypeText:     "text",
		DimensionTypeRating:   "rating",
		DimensionTypeBoolean:  "boolean",
		DimensionTypeNumber:   "number",
		DimensionTypeCurrency: "currency",
	}
	for got, want := range expected {
		if got != want {
			t.Errorf("dimension type constant = %q, want %q", got, want)
		}
	}
}

func TestPaginationDefaults(t *testing.T) {
	t.Parallel()

	if DefaultPageSize != 20 {
		t.Errorf("DefaultPageSize = %d, want 20", DefaultPageSize)
	}

	if MaxPageSize != 100 {
		t.Errorf("MaxPageSize = %d, want 100", MaxPageSize)
	}
}

// ---------------------------------------------------------------------------
// Dimension type validation
// ---------------------------------------------------------------------------

func TestIsValidDimensionType(t *testing.T) {
	t.Parallel()

	validTypes := []string{
		DimensionTypeText,
		DimensionTypeRating,
		DimensionTypeBoolean,
		DimensionTypeNumber,
		DimensionTypeCurrency,
	}
	for _, dt := range validTypes {
		if !IsValidDimensionType(dt) {
			t.Errorf("expected %q to be a valid dimension type", dt)
		}
	}

	invalidTypes := []string{"", "image", "select", "date", "rating2"}
	for _, dt := range invalidTypes {
		if IsValidDimensionType(dt) {
			t.Errorf("expected %q to be an invalid dimension type", dt)
		}
	}
}

// ---------------------------------------------------------------------------
// Default comparison types
// ---------------------------------------------------------------------------

func TestDefaultComparisonTypes(t *testing.T) {
	t.Parallel()

	if len(DefaultComparisonTypes) == 0 {
		t.Error("expected at least one default comparison type")
	}

	slugs := make(map[string]bool)

	for _, ct := range DefaultComparisonTypes {
		if ct.Name == "" {
			t.Error("default comparison type name should not be empty")
		}

		if ct.Slug == "" {
			t.Error("default comparison type slug should not be empty")
		}

		if ct.ItemLabel == "" {
			t.Error("default comparison type item_label should not be empty")
		}

		if slugs[ct.Slug] {
			t.Errorf("duplicate slug in default types: %s", ct.Slug)
		}

		slugs[ct.Slug] = true
	}
}

func TestDefaultComparisonTypesContent(t *testing.T) {
	t.Parallel()
	// Verify the expected default types exist.
	expectedSlugs := map[string]bool{
		"universities": false,
		"visa-types":   false,
		"banks":        false,
		"courses":      false,
	}
	for _, ct := range DefaultComparisonTypes {
		if _, ok := expectedSlugs[ct.Slug]; ok {
			expectedSlugs[ct.Slug] = true
		}
	}

	for slug, found := range expectedSlugs {
		if !found {
			t.Errorf("expected default type with slug %q", slug)
		}
	}
}

// ---------------------------------------------------------------------------
// Input validation
// ---------------------------------------------------------------------------

func TestCreateComparisonTypeInputValidation(t *testing.T) {
	t.Parallel()
	// Empty name should be invalid.
	input := CreateComparisonTypeInput{Name: "", Slug: "test"}
	if input.Name != "" {
		t.Error("expected empty name")
	}

	// Empty slug should be invalid.
	input2 := CreateComparisonTypeInput{Name: "Test", Slug: ""}
	if input2.Slug != "" {
		t.Error("expected empty slug")
	}
}

func TestCreateComparisonItemInputValidation(t *testing.T) {
	t.Parallel()
	// Empty title should be invalid.
	input := CreateComparisonItemInput{Title: "", ContentID: pgtype.UUID{}}
	if input.Title != "" {
		t.Error("expected empty title")
	}
}

func TestCreateComparisonDimensionInputValidation(t *testing.T) {
	t.Parallel()
	// Empty label should be invalid.
	input := CreateComparisonDimensionInput{Label: "", DimensionType: DimensionTypeText}
	if input.Label != "" {
		t.Error("expected empty label")
	}

	// Invalid dimension type should be caught by IsValidDimensionType.
	if IsValidDimensionType(input.DimensionType) != true {
		t.Error("expected 'text' to be valid")
	}
}

func TestUpsertComparisonValueInputDefaults(t *testing.T) {
	t.Parallel()
	// ValueRating and ValueBoolean should be nil by default.
	input := UpsertComparisonValueInput{
		DimensionID: pgtype.UUID{},
		ItemID:      pgtype.UUID{},
		Value:       "test",
	}
	if input.ValueRating != nil {
		t.Error("expected nil ValueRating")
	}

	if input.ValueBoolean != nil {
		t.Error("expected nil ValueBoolean")
	}
}

// ---------------------------------------------------------------------------
// Comparison matrix types
// ---------------------------------------------------------------------------

func TestComparisonMatrixItemStructure(t *testing.T) {
	t.Parallel()

	mi := ComparisonMatrixItem{
		Dimensions: []ComparisonMatrixDimension{},
	}
	if len(mi.Dimensions) != 0 {
		t.Error("expected empty dimensions slice")
	}
}

func TestComparisonMatrixDimensionDefaults(t *testing.T) {
	t.Parallel()

	md := ComparisonMatrixDimension{
		DimensionID: pgtype.UUID{},
		Label:       "Cost",
		Type:        DimensionTypeCurrency,
	}
	if md.Value != nil {
		t.Error("expected nil Value")
	}

	if md.ValueRating != nil {
		t.Error("expected nil ValueRating")
	}

	if md.ValueBoolean != nil {
		t.Error("expected nil ValueBoolean")
	}
}

// ---------------------------------------------------------------------------
// Comparison type access (§30 — can_create grants)
// ---------------------------------------------------------------------------

func TestAccessGrantsCreate(t *testing.T) {
	t.Parallel()

	var groupA, groupB pgtype.UUID
	if err := groupA.Scan("11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatal(err)
	}

	if err := groupB.Scan("22222222-2222-2222-2222-222222222222"); err != nil {
		t.Fatal(err)
	}

	memberSet := map[string]bool{groupA.String(): true}

	tests := []struct {
		name     string
		access   []gen.ListComparisonTypeAccessRow
		groupSet map[string]bool
		want     bool
	}{
		{name: "no access rows is unrestricted", access: nil, groupSet: memberSet, want: true},
		{
			name:     "wildcard grant allows any caller",
			access:   []gen.ListComparisonTypeAccessRow{{CanCreate: true}},
			groupSet: memberSet,
			want:     true,
		},
		{
			// The seeded cost_of_living shape: a single NULL-group row with
			// can_create=false (system/AI-managed type) must deny everyone.
			name:     "wildcard deny-only row blocks all users",
			access:   []gen.ListComparisonTypeAccessRow{{CanCreate: false}},
			groupSet: memberSet,
			want:     false,
		},
		{
			name:     "wildcard deny-only row blocks anonymous callers",
			access:   []gen.ListComparisonTypeAccessRow{{CanCreate: false}},
			groupSet: nil,
			want:     false,
		},
		{
			name:     "matching group grant allows member",
			access:   []gen.ListComparisonTypeAccessRow{{UserGroupID: groupA, CanCreate: true}},
			groupSet: memberSet,
			want:     true,
		},
		{
			name:     "non-matching group grant denies member",
			access:   []gen.ListComparisonTypeAccessRow{{UserGroupID: groupB, CanCreate: true}},
			groupSet: memberSet,
			want:     false,
		},
		{
			name: "deny row for one group does not block a grant for another",
			access: []gen.ListComparisonTypeAccessRow{
				{UserGroupID: groupB, CanCreate: false},
				{UserGroupID: groupA, CanCreate: true},
			},
			groupSet: memberSet,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := accessGrantsCreate(tt.access, tt.groupSet); got != tt.want {
				t.Errorf("accessGrantsCreate() = %v, want %v", got, tt.want)
			}
		})
	}
}
