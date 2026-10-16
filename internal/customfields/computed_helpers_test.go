package customfields

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/db/gen"
)

func testUUID(b byte) pgtype.UUID { return pgtype.UUID{Valid: true, Bytes: [16]byte{b}} }

func numericVal(t *testing.T, s string) pgtype.Numeric {
	t.Helper()

	var n pgtype.Numeric
	if err := n.Scan(s); err != nil {
		t.Fatalf("scan numeric: %v", err)
	}

	return n
}

func TestComputedEligibleField(t *testing.T) {
	spec := []byte(`{"computed":{"metric":"avg","sources":[{"group":"g","key":"k"}]}}`)

	cases := []struct {
		name     string
		typ      string
		options  []byte
		expected bool
	}{
		{"currency eligible", FieldTypeCurrency, spec, true},
		{"price_range eligible", FieldTypePriceRange, spec, true},
		{"number eligible", FieldTypeNumber, spec, true},
		{"text ineligible", FieldTypeText, spec, false},
		{"price_parts no primary_part", FieldTypePriceParts, spec, false},
		{"price_parts with primary_part", FieldTypePriceParts, []byte(`{"primary_part":"price","parts":[{"key":"price","kind":"money"}],"computed":{"metric":"avg","sources":[{"group":"g","key":"k"}]}}`), true},
		{"no spec", FieldTypeCurrency, []byte(`{"currency_code":"CAD"}`), false},
		{"nil options", FieldTypeCurrency, nil, false},
	}
	for _, c := range cases {
		if got := ComputedEligibleField(c.typ, c.options); got != c.expected {
			t.Errorf("%s: got %v want %v", c.name, got, c.expected)
		}
	}
}

func TestComputedScalarFromValue(t *testing.T) {
	ppOpts := []byte(`{"primary_part":"price","parts":[{"key":"price","kind":"money"},{"key":"details"}]}`)

	cases := []struct {
		name    string
		typ     string
		options []byte
		json    []byte
		num     pgtype.Numeric
		want    float64
		ok      bool
	}{
		{"currency value_number", FieldTypeCurrency, nil, nil, numericVal(t, "42.5"), 42.5, true},
		{"price_range avg", FieldTypePriceRange, nil, []byte(`{"min":1,"avg":2,"max":3}`), pgtype.Numeric{}, 2, true},
		{"price_range min fallback", FieldTypePriceRange, nil, []byte(`{"min":1,"max":3}`), pgtype.Numeric{}, 1, true},
		{"price_range empty", FieldTypePriceRange, nil, []byte(`{}`), pgtype.Numeric{}, 0, false},
		{"price_parts primary", FieldTypePriceParts, ppOpts, []byte(`{"parts":{"price":15,"details":"x"}}`), pgtype.Numeric{}, 15, true},
		{"price_parts missing part", FieldTypePriceParts, ppOpts, []byte(`{"parts":{"other":15}}`), pgtype.Numeric{}, 0, false},
		{"text no scalar", FieldTypeText, nil, nil, pgtype.Numeric{}, 0, false},
	}
	for _, c := range cases {
		got, ok := ComputedScalarFromValue(c.typ, c.options, c.json, c.num)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestFoldComputedMetric(t *testing.T) {
	xs := []float64{10, 20, 30}
	if got := FoldComputedMetric("avg", xs); got != 20 {
		t.Errorf("avg: got %v", got)
	}

	if got := FoldComputedMetric("sum", xs); got != 60 {
		t.Errorf("sum: got %v", got)
	}

	if got := FoldComputedMetric("min", xs); got != 10 {
		t.Errorf("min: got %v", got)
	}

	if got := FoldComputedMetric("max", xs); got != 30 {
		t.Errorf("max: got %v", got)
	}
}

// The hydration overlay folds an entity's own cfv rows per spec and
// replaces the keyed display value — including deleting a stale stored
// value when no source resolves (never falls back to stored).
func TestComputedOverlayApply(t *testing.T) {
	gidFood, gidUtil := testUUID(0xA0), testUUID(0xB0)

	defs := []gen.ListComputedFieldDefsRow{
		{Key: "exp_food", FieldType: FieldTypeCurrency, Options: []byte(
			`{"computed":{"metric":"sum","sources":[{"group":"food","key":"milk_1l"},{"group":"food","key":"bread_500g"}]}}`)},
		{Key: "exp_net", FieldType: FieldTypeCurrency, Options: []byte(
			`{"computed":{"metric":"avg","sources":[{"group":"util","key":"internet_60"}]}}`)},
		{Key: "exp_none", FieldType: FieldTypeCurrency, Options: []byte(
			`{"computed":{"metric":"avg","sources":[{"group":"util","key":"nonexistent"}]}}`)},
	}
	o := &computedOverlay{
		defs:     defs,
		groupIDs: map[string]pgtype.UUID{"food": gidFood, "util": gidUtil},
	}

	sources := []cfvComputedSource{
		{groupID: gidFood, key: "milk_1l", typ: FieldTypePriceRange, valueJSON: []byte(`{"avg":4.29,"min":3.29,"max":5.29}`)},
		{groupID: gidFood, key: "bread_500g", typ: FieldTypePriceRange, valueJSON: []byte(`{"avg":2.65}`)},
		{groupID: gidUtil, key: "internet_60", typ: FieldTypePriceRange, valueJSON: []byte(`{"avg":69,"min":55,"max":85}`)},
	}
	values := map[string]any{
		"exp_food": 999.0, // stale stored value — must be replaced
		"exp_none": 777.0, // stale stored value with no sources — must be deleted
		"other":    "keep",
	}

	o.apply(sources, values)

	if got, ok := values["exp_food"].(float64); !ok || got < 6.93 || got > 6.95 {
		t.Errorf("exp_food: got %v want ~6.94 (sum of avgs)", values["exp_food"])
	}

	if got := values["exp_net"]; got != 69.0 {
		t.Errorf("exp_net: got %v want 69 (avg)", got)
	}

	if _, ok := values["exp_none"]; ok {
		t.Error("exp_none: stale stored value must be deleted when no source resolves")
	}

	if values["other"] != "keep" {
		t.Error("non-computed key must survive")
	}
}

func TestComputedDisplayValue(t *testing.T) {
	if got := ComputedDisplayValue(FieldTypeCurrency, nil, 5.0); got != 5.0 {
		t.Errorf("scalar: got %v", got)
	}

	pr := ComputedDisplayValue(FieldTypePriceRange, nil, 5.0).(map[string]any)
	if pr["avg"] != 5.0 {
		t.Errorf("price_range: got %v", pr)
	}

	pp := ComputedDisplayValue(FieldTypePriceParts, []byte(`{"primary_part":"price","parts":[{"key":"price","kind":"money"}]}`), 5.0).(map[string]any)
	if pp["parts"].(map[string]any)["price"] != 5.0 {
		t.Errorf("price_parts: got %v", pp)
	}
}
