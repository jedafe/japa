package comparison

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestParseUUIDList(t *testing.T) {
	t.Parallel()

	valid := "11111111-1111-1111-1111-111111111111"

	got, err := parseUUIDList([]string{valid, valid})
	if err != nil {
		t.Fatalf("parseUUIDList valid: %v", err)
	}

	if len(got) != 2 || !got[0].Valid {
		t.Fatalf("parseUUIDList = %+v, want 2 valid UUIDs", got)
	}

	if _, err := parseUUIDList([]string{"not-a-uuid"}); err == nil {
		t.Fatal("parseUUIDList invalid input: expected error, got nil")
	}
}

func TestDimensionFieldsFromGroup(t *testing.T) {
	t.Parallel()

	rows := []gen.ListCustomFieldsByGroupRow{
		{Label: "Provider", Key: "provider", FieldType: "text"},
	}

	fields := dimensionFieldsFromGroup(rows)
	if len(fields) != 1 || fields[0].Label != "Provider" || fields[0].Key != "provider" {
		t.Fatalf("dimensionFieldsFromGroup = %+v", fields)
	}
}

func TestDisambiguateFieldLabels(t *testing.T) {
	t.Parallel()

	g1 := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	g2 := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	fields := []dimensionField{
		{GroupID: g1, Label: "Provider"},
		{GroupID: g1, Label: "Unique Field"},
		{GroupID: g2, Label: "Provider"},
	}

	labels := map[[16]byte]string{
		g1.Bytes: "Overview",
		g2.Bytes: "Accounts & Fees",
	}

	out := disambiguateFieldLabels(fields, labels)

	if out[0].Label != "Provider (Overview)" {
		t.Errorf("out[0].Label = %q", out[0].Label)
	}

	if out[1].Label != "Unique Field" {
		t.Errorf("out[1].Label = %q — unique labels must not change", out[1].Label)
	}

	if out[2].Label != "Provider (Accounts & Fees)" {
		t.Errorf("out[2].Label = %q", out[2].Label)
	}
}

func TestDisambiguateFieldLabelsNilMap(t *testing.T) {
	t.Parallel()

	fields := []dimensionField{
		{Label: "Provider"},
		{Label: "Provider"},
	}

	out := disambiguateFieldLabels(fields, nil)

	// No group labels available — labels stay as-is rather than crashing.
	if out[0].Label != "Provider" || out[1].Label != "Provider" {
		t.Fatalf("labels = %q, %q", out[0].Label, out[1].Label)
	}
}
