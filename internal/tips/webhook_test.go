// webhook_test.go — §125 webhook-path regressions: Ko-fi minor-unit
// conversion, the OC webhook slug-only import path, and the generic-PUT
// collective-slug eligibility gate.
package tips

import (
	"context"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"
)

// kofiAmountToMinor must honour the ISO-4217 exponent — a fixed ×100
// inflates JPY-type donations 100× and truncates 3-decimal currencies.
func TestKofiAmountToMinor(t *testing.T) {
	t.Parallel()
	cases := []struct {
		amount, currency string
		want             int64
	}{
		{"5.00", "USD", 500},
		{"500", "JPY", 500},
		{"1.234", "KWD", 1234},
		{"12.50", "NGN", 1250},
	}
	for _, tc := range cases {
		got, err := kofiAmountToMinor(tc.amount, tc.currency)
		if err != nil {
			t.Fatalf("kofiAmountToMinor(%q, %q): %v", tc.amount, tc.currency, err)
		}
		if got != tc.want {
			t.Errorf("kofiAmountToMinor(%q, %q) = %d, want %d", tc.amount, tc.currency, got, tc.want)
		}
	}
	if _, err := kofiAmountToMinor("", "USD"); err == nil {
		t.Error("empty amount should error, not silently store 0")
	}
	if _, err := kofiAmountToMinor("abc", "USD"); err == nil {
		t.Error("unparseable amount should error")
	}
}

// Slug-only OC connects have no poller — the webhook payload is the only
// import source, so it must still land directly.
func TestImportOCWebhookSlugOnlyImports(t *testing.T) {
	fs := newFakeStore()
	ownerID := uid(t, "11111111-1111-1111-1111-111111111111")
	slug := "mycollective"
	fs.ocSecretOK = true
	fs.ocSecretRow = gen.TipSetting{OwnerType: RecipientUser, OwnerID: ownerID, OcCollectiveSlug: &slug}
	svc := &Service{store: fs}

	if err := svc.ImportOCWebhook(context.Background(), "sec", OCContribution{
		ID: "ord-9", AmountCents: 700, Currency: "USD", FromName: "Donor",
	}); err != nil {
		t.Fatalf("ImportOCWebhook: %v", err)
	}
	if len(fs.inserted) != 1 {
		t.Fatalf("expected 1 tip insert, got %d", len(fs.inserted))
	}
	if got := fs.inserted[0]; got.OcContributionID == nil || *got.OcContributionID != "ord-9" {
		t.Fatalf("webhook order id not recorded: %+v", got)
	}
}

// A non-empty oc_collective_slug on the generic settings PUT is a slug-only
// connect — it must require the same external eligibility as ConnectOC.
func TestUpdateSettingsSlugWriteRequiresExternalEligibility(t *testing.T) {
	fs := newFakeStore()
	uidV := uid(t, "22222222-2222-2222-2222-222222222222")
	slug := "collective"
	eligible := &Service{store: fs, perms: fakePerms{map[string]bool{"tips.receive": true}}}
	if _, err := eligible.UpdateSettingsFor(context.Background(), UserOwner(uidV), uidV, UpdateSettingsInput{OCCollectiveSlug: &slug}); err != nil {
		t.Fatalf("eligible user slug write should pass: %v", err)
	}

	// A mentor is writable (implicit CanReceive grant) but lacks the
	// tips.receive permission ensureExternalEligible requires — this
	// discriminates the slug gate from the earlier writability check.
	fs2 := newFakeStore()
	fs2.mentor = true
	denied := &Service{store: fs2, perms: fakePerms{map[string]bool{}}}
	if _, err := denied.UpdateSettingsFor(context.Background(), UserOwner(uidV), uidV, UpdateSettingsInput{OCCollectiveSlug: &slug}); err != ErrNotEligible {
		t.Fatalf("mentor-without-tips.receive slug write should be ErrNotEligible, got %v", err)
	}
}
