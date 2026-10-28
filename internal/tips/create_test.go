// create_test.go — §125 CreateTip validation + wallet-settlement paths:
// eligibility, recipient resolution, self-tip, method acceptance, minimums,
// daily points cap, fee math, idempotent replay.
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
		ID: mustUID("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"),
		OwnerType: owner.Type, OwnerID: owner.ID, TipsEnabled: true,
		AcceptPoints: true, AcceptCredits: true, AcceptFiat: true,
	}
	fs.settings[key(owner.Type, owner.ID)] = row
	return row
}

func baseSvc(fs *fakeStore) *Service {
	return &Service{
		store:    fs,
		settings: fakeSettings{m: map[string]any{"tips.enabled": true}},
		perms:    fakePerms{map[string]bool{"tips.send": true}},
	}
}

func walletInput() CreateTipInput {
	return CreateTipInput{RecipientType: RecipientUser, RecipientUserID: recipientID, Method: MethodCredits, CreditsAmount: 100}
}

func TestCreateTipFeatureOff(t *testing.T) {
	fs := newFakeStore()
	svc := &Service{store: fs, settings: fakeSettings{m: map[string]any{"tips.enabled": false}}, perms: fakePerms{map[string]bool{"tips.send": true}}}
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
		perms:    fakePerms{map[string]bool{"tips.send": true}},
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

func TestCreateTipPointsNoFeeAndDailyCap(t *testing.T) {
	fs := newFakeStore()
	fs.pointsToday = 490
	enabledRecipient(fs, UserOwner(recipientID))
	svc := &Service{
		store:    fs,
		settings: fakeSettings{m: map[string]any{"tips.enabled": true, "tips.fee_percent": 50, "tips.points_daily_limit": 500}},
		perms:    fakePerms{map[string]bool{"tips.send": true}},
	}
	in := CreateTipInput{RecipientType: RecipientUser, RecipientUserID: recipientID, Method: MethodPoints, PointsAmount: 20}
	if _, err := svc.CreateTip(context.Background(), senderID, in, ""); !errors.Is(err, ErrDailyPointsLimit) {
		t.Fatalf("expected daily limit, got %v", err)
	}
	fs.pointsToday = 100
	res, err := svc.CreateTip(context.Background(), senderID, in, "")
	if err != nil {
		t.Fatalf("CreateTip: %v", err)
	}
	_ = res
	if fs.settled[0].fee != 0 || fs.settled[0].net != 20 {
		t.Fatalf("points should pass through whole: %+v", fs.settled[0])
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
