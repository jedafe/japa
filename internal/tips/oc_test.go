// oc_test.go — §125 OC import: dedupe by oc_contribution_id, sender matching
// by email, and recipient-type column mapping.
package tips

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

func TestImportContributionDedupeAndRecipient(t *testing.T) {
	fs := newFakeStore()
	existing := gen.Tip{}
	var ocID *string
	oid := "oc-42"
	ocID = &oid
	existing.OcContributionID = ocID
	fs.ocTips = map[string]gen.Tip{oid: existing}

	svc := &Service{store: fs}
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))

	inserted, err := svc.ImportContribution(context.Background(), owner, ExternalContribution{
		ID: "oc-42", AmountMinor: 100, Currency: "USD",
	})
	if err != nil {
		t.Fatalf("ImportContribution: %v", err)
	}
	if inserted {
		t.Fatal("duplicate import should no-op")
	}

	cur := "USD"
	amt := int64(5000)
	fs.inserted = nil
	inserted, err = svc.ImportContribution(context.Background(), owner, ExternalContribution{
		ID: "oc-43", AmountMinor: 5000, Currency: "USD", FromName: "Donor",
	})
	if err != nil {
		t.Fatalf("ImportContribution: %v", err)
	}
	if !inserted {
		t.Fatal("new contribution should insert")
	}
	if len(fs.inserted) != 1 {
		t.Fatalf("expected 1 insert, got %d", len(fs.inserted))
	}
	got := fs.inserted[0]
	if got.Method != MethodOpenCollective || got.Status != StatusCompleted {
		t.Fatalf("wrong method/status: %+v", got)
	}
	if got.RecipientUserID != owner.ID {
		t.Fatalf("recipient_user_id not mapped: %v", got.RecipientUserID)
	}
	if *got.FiatAmount != amt || *got.FiatCurrency != cur {
		t.Fatalf("fiat fields wrong: %+v", got)
	}
	if got.SenderUserID.Valid {
		t.Fatal("unmatched email should leave sender NULL")
	}
}

// The import boundary rejects non-positive amounts — a caller bug or
// malformed provider row must not record a zero-value completed tip.
func TestImportContributionRejectsNonPositive(t *testing.T) {
	fs := newFakeStore()
	svc := &Service{store: fs}
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))

	for _, amt := range []int64{0, -500} {
		inserted, err := svc.ImportContribution(context.Background(), owner, ExternalContribution{
			ID: "oc-zero", AmountMinor: amt, Currency: "USD",
		})
		if err != nil {
			t.Fatalf("ImportContribution(%d): %v", amt, err)
		}
		if inserted {
			t.Fatalf("amount %d should not import a tip", amt)
		}
	}
	if len(fs.inserted) != 0 {
		t.Fatalf("no rows should be inserted, got %d", len(fs.inserted))
	}
}

// Org OC imports snapshot the org owner's wallet into
// recipient_wallet_user_id — the received ledger and clawbacks resolve it
// even after an ownership transfer (BUG-037/038).
func TestImportContributionOrgWalletSnapshot(t *testing.T) {
	fs := newFakeStore()
	ownerID := uid(t, "44444444-4444-4444-4444-444444444444")
	orgID := uid(t, "55555555-5555-5555-5555-555555555555")
	fs.orgOwner = ownerID
	svc := &Service{store: fs}

	inserted, err := svc.ImportContribution(context.Background(), OrgOwner(orgID), ExternalContribution{
		ID: "oc-org-1", AmountMinor: 1200, Currency: "USD",
	})
	if err != nil || !inserted {
		t.Fatalf("ImportContribution: inserted=%v err=%v", inserted, err)
	}
	got := fs.inserted[0]
	if got.RecipientOrganizationID != orgID || got.RecipientUserID.Valid {
		t.Fatalf("org recipient columns wrong: %+v", got)
	}
	if got.RecipientWalletUserID != ownerID {
		t.Fatalf("wallet snapshot should be the org owner: %v", got.RecipientWalletUserID)
	}
}

func TestImportContributionEmailMatch(t *testing.T) {
	fs := newFakeStore()
	matched := uid(t, "33333333-3333-3333-3333-333333333333")
	fs.emailToUser = map[string]pgtype.UUID{"donor@example.com": matched}
	svc := &Service{store: fs}
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))

	_, err := svc.ImportContribution(context.Background(), owner, ExternalContribution{
		ID: "oc-99", AmountMinor: 100, Currency: "EUR", FromEmail: "donor@example.com",
	})
	if err != nil {
		t.Fatalf("ImportContribution: %v", err)
	}
	if fs.inserted[0].SenderUserID != matched {
		t.Fatalf("expected sender matched by email, got %v", fs.inserted[0].SenderUserID)
	}
}
