package comparison

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// ---------------------------------------------------------------------------
// Constants and configuration
// ---------------------------------------------------------------------------

func TestDimensionTypeConstants(t *testing.T) {
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
	// Empty title should be invalid.
	input := CreateComparisonItemInput{Title: "", ContentID: pgtype.UUID{}}
	if input.Title != "" {
		t.Error("expected empty title")
	}
}

func TestCreateComparisonDimensionInputValidation(t *testing.T) {
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
	mi := ComparisonMatrixItem{
		Dimensions: []ComparisonMatrixDimension{},
	}
	if len(mi.Dimensions) != 0 {
		t.Error("expected empty dimensions slice")
	}
}

func TestComparisonMatrixDimensionDefaults(t *testing.T) {
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
