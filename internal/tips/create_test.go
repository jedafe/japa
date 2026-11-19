// create_test.go — §125 CreateTip validation + wallet-settlement paths:
// eligibility, recipient resolution, self-tip, method acceptance, minimums,
// retired points method, fee math, idempotent replay.
package tips

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

type fakeSettings struct{ m map[string]any }

func (f fakeSettings) GetIntOr(_ context.Context, k string, fb int) int {
	if v, ok := f.m[k].(int); ok {
		return v
	}
	return fb
}
func (f fakeSettings) GetBoolOr(_ context.Context, k string, fb bool) bool {
	if v, ok := f.m[k].(bool); ok {
		return v
	}
	return fb
}
func (f fakeSettings) GetStringOr(_ context.Context, k, fb string) string {
	if v, ok := f.m[k].(string); ok {
		return v
	}
	return fb
}

var (
	senderID    = mustUID("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	recipientID = mustUID("bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb")
	contentID   = mustUID("cccccccc-cccc-cccc-cccc-cccccccccccc")
	orgID       = mustUID("dddddddd-dddd-dddd-dddd-dddddddddddd")
)

func mustUID(s string) pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func enabledRecipient(fs *fakeStore, owner Owner) gen.TipSetting {
	row := gen.TipSetting{
		ID:        mustUID("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		OwnerType: owner.Type, OwnerID: owner.ID, TipsEnabled: true,
		AcceptCredits: true, AcceptFiat: true,
	}
	fs.settings[key(owner.Type, owner.ID)] = row
	return row
}

func baseSvc(fs *fakeStore) *Service {
	return &Service{
		store:    fs,
		settings: fakeSettings{m: map[string]any{"tips.enabled": true}},
		perms:    fakePerms{map[string]bool{"tips.send": true, "tips.receive": true}},
	}
}

func walletInput() CreateTipInput {
	return CreateTipInput{RecipientType: RecipientUser, RecipientUserID: recipientID, Method: MethodCredits, CreditsAmount: 100}
}

func TestCreateTipFeatureOff(t *testing.T) {
	fs := newFakeStore()
	svc := &Service{store: fs, settings: fakeSettings{m: map[string]any{"tips.enabled": false}}, perms: fakePerms{map[string]bool{"tips.send": true, "tips.receive": true}}}
	if _, err := svc.CreateTip(context.Background(), senderID, walletInput(), ""); !errors.Is(err, ErrTipsDisabled) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipSelfTip(t *testing.T) {
	fs := newFakeStore()
	enabledRecipient(fs, UserOwner(senderID))
	svc := baseSvc(fs)
	in := CreateTipInput{RecipientType: RecipientUser, RecipientUserID: senderID, Method: MethodCredits, CreditsAmount: 10}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); !errors.Is(err, ErrSelfTip) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipRecipientDisabled(t *testing.T) {
	fs := newFakeStore()
	svc := baseSvc(fs)
	if _, err := svc.CreateTip(context.Background(), senderID, walletInput(), ""); !errors.Is(err, ErrRecipientNoTips) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipMethodRejected(t *testing.T) {
	fs := newFakeStore()
	owner := UserOwner(recipientID)
	row := enabledRecipient(fs, owner)
	row.AcceptCredits = false
	fs.settings[key(owner.Type, owner.ID)] = row
	svc := baseSvc(fs)
	if _, err := svc.CreateTip(context.Background(), senderID, walletInput(), ""); !errors.Is(err, ErrMethodNotAccepted) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipBelowMinimum(t *testing.T) {
	fs := newFakeStore()
	owner := UserOwner(recipientID)
	row := enabledRecipient(fs, owner)
	row.MinTipAmount = 200
	fs.settings[key(owner.Type, owner.ID)] = row
	svc := baseSvc(fs)
	if _, err := svc.CreateTip(context.Background(), senderID, walletInput(), ""); !errors.Is(err, ErrBelowMinimum) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipPlatformWalletRejected(t *testing.T) {
	fs := newFakeStore()
	enabledRecipient(fs, PlatformOwner())
	svc := baseSvc(fs)
	in := CreateTipInput{RecipientType: RecipientPlatform, Method: MethodCredits, CreditsAmount: 10}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); !errors.Is(err, ErrMethodNotAccepted) {
		t.Fatalf("got %v", err)
	}
}

func TestCreateTipCreditsSettles(t *testing.T) {
	fs := newFakeStore()
	enabledRecipient(fs, UserOwner(recipientID))
	svc := &Service{
		store:    fs,
		settings: fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 10}},
		perms:    fakePerms{map[string]bool{"tips.send": true, "tips.receive": true}},
	}
	res, err := svc.CreateTip(context.Background(), senderID, walletInput(), "")
	if err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if res.Duplicate {
		t.Fatal("unexpected duplicate")
	}
	if len(fs.settled) != 1 {
		t.Fatalf("expected settle call, got %d", len(fs.settled))
	}
	p := fs.settled[0]
	if p.gross != 100 || p.fee != 10 || p.net != 90 || p.ledgerType != "credits" {
		t.Fatalf("fee math wrong: %+v", p)
	}
	if p.recipientUserID != recipientID || p.walletOwnerID != recipientID {
		t.Fatalf("recipient mapping wrong: %+v", p)
	}
}

func TestCreateTipPointsMethodRejected(t *testing.T) {
	fs := newFakeStore()
	enabledRecipient(fs, UserOwner(recipientID))
	svc := baseSvc(fs)
	// ADR-136: points are retired as a tip method.
	in := CreateTipInput{RecipientType: RecipientUser, RecipientUserID: recipientID, Method: MethodPoints, CreditsAmount: 20}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); !errors.Is(err, ErrInvalidMethod) {
		t.Fatalf("expected invalid method, got %v", err)
	}
}

func TestCreateTipContentRecipient(t *testing.T) {
	fs := newFakeStore()
	fs.contentAuthor = recipientID
	enabledRecipient(fs, UserOwner(recipientID))
	svc := baseSvc(fs)
	in := CreateTipInput{ContentID: contentID, Method: MethodCredits, CreditsAmount: 50}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if fs.settled[0].recipientUserID != recipientID || fs.settled[0].contentID != contentID {
		t.Fatalf("content recipient mapping wrong: %+v", fs.settled[0])
	}
}

func TestCreateTipOrgOwnerSettlement(t *testing.T) {
	fs := newFakeStore()
	fs.orgOwner = recipientID
	enabledRecipient(fs, OrgOwner(orgID))
	svc := baseSvc(fs)
	in := CreateTipInput{RecipientType: RecipientOrganization, RecipientOrgID: orgID, Method: MethodCredits, CreditsAmount: 50}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	p := fs.settled[0]
	if p.walletOwnerID != recipientID || p.recipientOrgID != orgID || p.recipientUserID.Valid {
		t.Fatalf("org settlement wrong: %+v", p)
	}
}

// Org tips snapshot the wallet owner at write time — after an ownership
// transfer a clawback must still debit the wallet that received the
// credits, not the new owner's (BUG-038). Rows predating the snapshot
// fall back to live owner resolution.
func TestWalletOwnerForOrgSnapshot(t *testing.T) {
	fs := newFakeStore()
	oldOwner := recipientID
	newOwner := mustUID("77777777-7777-7777-7777-777777777777")
	fs.orgOwner = newOwner // ownership transferred after the tip settled
	svc := baseSvc(fs)

	snapTip := gen.Tip{
		RecipientType:           RecipientOrganization,
		RecipientOrganizationID: orgID,
		RecipientWalletUserID:   oldOwner,
	}
	got, err := svc.walletOwnerFor(context.Background(), snapTip)
	if err != nil || got != oldOwner {
		t.Fatalf("snapshot should win: got %v err %v", got, err)
	}

	legacyTip := gen.Tip{
		RecipientType:           RecipientOrganization,
		RecipientOrganizationID: orgID,
	}
	got, err = svc.walletOwnerFor(context.Background(), legacyTip)
	if err != nil || got != newOwner {
		t.Fatalf("legacy rows should fall back to live owner: got %v err %v", got, err)
	}
}

func TestCreateTipIdempotentReplay(t *testing.T) {
	fs := newFakeStore()
	enabledRecipient(fs, UserOwner(recipientID))
	fs.idemTip = gen.Tip{Method: MethodCredits, Status: StatusCompleted}
	fs.hasIdemTip = true
	svc := baseSvc(fs)
	res, err := svc.CreateTip(context.Background(), senderID, walletInput(), "key-1")
	if err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if !res.Duplicate || len(fs.settled) != 0 {
		t.Fatalf("replay should short-circuit, got %+v settled %d", res, len(fs.settled))
	}
}

// Space tips must store recipient_space_id AND a recipient_user_id snapshot
// of the wallet owner — tips_recipient_check rejects inserts missing either.
func TestCreateTipSpaceRecipientSnapshot(t *testing.T) {
	fs := newFakeStore()
	spaceID := mustUID("ffffffff-ffff-ffff-ffff-ffffffffffff")
	ownerID := mustUID("99999999-9999-9999-9999-999999999999")
	fs.spaceRecipient = ownerID
	enabledRecipient(fs, SpaceOwner(spaceID))
	svc := baseSvc(fs)

	in := CreateTipInput{RecipientType: RecipientSpace, RecipientSpaceID: spaceID, Method: MethodCredits, CreditsAmount: 50}
	if _, err := svc.CreateTip(context.Background(), senderID, in, "k-space"); err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	if len(fs.settled) != 1 {
		t.Fatalf("expected 1 settlement, got %d", len(fs.settled))
	}
	p := fs.settled[0]
	if p.recipientType != RecipientSpace || p.recipientSpaceID != spaceID ||
		p.recipientUserID != ownerID || p.walletOwnerID != ownerID {
		t.Fatalf("space settlement wrong: %+v", p)
	}
}

// The wire `amount` maps per-method: credits+stablecoin → CreditsAmount,
// fiat providers → FiatAmount. Regression test for the stablecoin path
// landing in FiatAmount (which failed every onchain tip at validation).
func TestCreateTipInputFromRequest(t *testing.T) {
	for _, tc := range []struct {
		method   string
		wantCr   int64
		wantFiat int64
	}{
		{MethodCredits, 100, 0},
		{MethodStablecoin, 42, 0},
		{"stripe", 0, 500},
		{"paystack", 0, 7500},
		{"manual_transfer", 0, 1000},
	} {
		in, err := createTipInputFromRequest(createTipRequest{Method: tc.method, Amount: tc.wantCr + tc.wantFiat})
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.method, err)
		}
		if in.CreditsAmount != tc.wantCr || in.FiatAmount != tc.wantFiat {
			t.Fatalf("%s: got credits=%d fiat=%d, want credits=%d fiat=%d",
				tc.method, in.CreditsAmount, in.FiatAmount, tc.wantCr, tc.wantFiat)
		}
	}

	// Recipient id routes by type; content_id scans independently.
	orgIDStr := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	in, err := createTipInputFromRequest(createTipRequest{
		Method: MethodCredits, Amount: 5, RecipientType: RecipientOrganization, RecipientID: orgIDStr,
		ContentID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
	})
	if err != nil {
		t.Fatalf("org recipient: %v", err)
	}
	if in.RecipientOrgID.String() != orgIDStr || !in.ContentID.Valid {
		t.Fatalf("org/content mapping wrong: %+v", in)
	}
	if _, err := createTipInputFromRequest(createTipRequest{Method: MethodCredits, Amount: 5, RecipientID: "not-a-uuid"}); err == nil {
		t.Fatal("expected invalid recipient_id error")
	}
	if _, err := createTipInputFromRequest(createTipRequest{Method: MethodCredits, Amount: 5, ContentID: "not-a-uuid"}); err == nil {
		t.Fatal("expected invalid content_id error")
	}
}
