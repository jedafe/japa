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

	inserted, err := svc.ImportContribution(context.Background(), owner, OCContribution{ID: "oc-42"})
	if err != nil {
		t.Fatalf("ImportContribution: %v", err)
	}
	if inserted {
		t.Fatal("duplicate import should no-op")
	}

	cur := "USD"
	amt := int64(5000)
	fs.inserted = nil
	inserted, err = svc.ImportContribution(context.Background(), owner, OCContribution{
		ID: "oc-43", AmountCents: 5000, Currency: "USD", FromName: "Donor",
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

func TestImportContributionEmailMatch(t *testing.T) {
	fs := newFakeStore()
	matched := uid(t, "33333333-3333-3333-3333-333333333333")
	fs.emailToUser = map[string]pgtype.UUID{"donor@example.com": matched}
	svc := &Service{store: fs}
	owner := UserOwner(uid(t, "11111111-1111-1111-1111-111111111111"))

	_, err := svc.ImportContribution(context.Background(), owner, OCContribution{
		ID: "oc-99", AmountCents: 100, Currency: "EUR", FromEmail: "donor@example.com",
	})
	if err != nil {
		t.Fatalf("ImportContribution: %v", err)
	}
	if fs.inserted[0].SenderUserID != matched {
		t.Fatalf("expected sender matched by email, got %v", fs.inserted[0].SenderUserID)
	}
}
