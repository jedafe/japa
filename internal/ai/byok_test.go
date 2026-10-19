package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// fakeRow implements the row Scan seam used by the byok scan helpers so the
// column-mapping logic is testable without a database.
type fakeRow struct {
	vals []any
	err  error
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.vals) {
		return fmt.Errorf("scan arity: got %d dest, want %d", len(dest), len(r.vals))
	}
	for i, v := range r.vals {
		switch d := dest[i].(type) {
		case *pgtype.UUID:
			*d = v.(pgtype.UUID)
		case *string:
			*d = v.(string)
		case *bool:
			*d = v.(bool)
		case *int:
			*d = v.(int)
		case *[]byte:
			if v == nil {
				*d = nil
			} else {
				*d = v.([]byte)
			}
		case *time.Time:
			*d = v.(time.Time)
		default:
			return fmt.Errorf("unsupported dest type %T", d)
		}
	}
	return nil
}

func TestByokProviderHasModel(t *testing.T) {
	t.Parallel()

	p := ByokProvider{Models: []ModelInfo{{ID: "gpt-4o", Label: "GPT-4o"}, {ID: "gpt-4o-mini"}}}
	if !p.HasModel("gpt-4o") {
		t.Error("expected gpt-4o to be allowed")
	}
	if p.HasModel("claude-3") {
		t.Error("expected claude-3 to be rejected")
	}
	if p.HasModel("") {
		t.Error("empty model must not match")
	}
}

func TestScanByokProvider(t *testing.T) {
	t.Parallel()

	row := &fakeRow{vals: []any{
		mustUUID(t, "11111111-2222-3333-4444-555555555555"), "openai", "OpenAI", "https://ref.example/openai",
		[]byte(`[{"id":"gpt-4o","label":"GPT-4o"}]`), true, 3,
		time.Now(), time.Now(),
	}}
	bp, err := scanByokProvider(row)
	if err != nil {
		t.Fatal(err)
	}
	if bp.DisplayName != "OpenAI" || bp.ProviderType != "openai" {
		t.Errorf("bad scan: %+v", bp)
	}
	if !bp.HasModel("gpt-4o") {
		t.Error("models jsonb not decoded")
	}
	if bp.ReferralClicks != 3 {
		t.Errorf("referral_clicks: got %d, want 3", bp.ReferralClicks)
	}
}

func TestScanByokProviderEmptyModels(t *testing.T) {
	t.Parallel()

	row := &fakeRow{vals: []any{
		mustUUID(t, "11111111-2222-3333-4444-555555555555"), "groq", "Groq", "",
		nil, false, 0, time.Now(), time.Now(),
	}}
	bp, err := scanByokProvider(row)
	if err != nil {
		t.Fatal(err)
	}
	if bp.Models == nil {
		t.Error("models must default to empty slice, not nil")
	}
}

func TestSupportedByokProviderTypes(t *testing.T) {
	t.Parallel()

	types := SupportedByokProviderTypes()
	if len(types) != 4 {
		t.Fatalf("expected 4 BYOK provider types, got %v", types)
	}
	for _, pt := range types {
		cfg := ProviderConfig{ProviderType: pt}
		if _, err := NewProviderFromConfig(cfg, "k"); err != nil {
			t.Errorf("BYOK type %q not constructible via NewProviderFromConfig: %v", pt, err)
		}
	}
}
