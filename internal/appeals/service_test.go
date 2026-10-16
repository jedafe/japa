// service_test.go — unit coverage for appeal subject labelling helpers.
package appeals

import "testing"

// subject_label is user-facing (ticket subject + appeal lists) — it must
// never degenerate to a raw entity UUID.
func TestNamedLabel(t *testing.T) {
	t.Parallel()

	if got := namedLabel("Removed post", "Visa Help Lagos"); got != "Removed post — Visa Help Lagos" {
		t.Fatalf("namedLabel with name = %q", got)
	}

	if got := namedLabel("Removed post", ""); got != "Removed post" {
		t.Fatalf("namedLabel empty name = %q", got)
	}

	if got := namedLabel("Removed post", "   "); got != "Removed post" {
		t.Fatalf("namedLabel blank name = %q", got)
	}
}

func TestNamedLabelPtr(t *testing.T) {
	t.Parallel()

	name := "Acme Relocation"
	if got := namedLabelPtr("Rejected organization claim", &name); got != "Rejected organization claim — Acme Relocation" {
		t.Fatalf("namedLabelPtr with name = %q", got)
	}

	if got := namedLabelPtr("Rejected organization claim", nil); got != "Rejected organization claim" {
		t.Fatalf("namedLabelPtr nil = %q", got)
	}
}
