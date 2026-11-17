package tips

import (
	"context"
	"errors"
	"testing"

	"github.com/jedafe/japa/internal/db/gen"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeBackedSettler struct {
	done    bool
	err     error
	calls   int
	gotNet  int64
	gotUser pgtype.UUID
}

func (f *fakeBackedSettler) SettleTipBacked(_ context.Context, _, user pgtype.UUID, net int64) (bool, error) {
	f.calls++
	f.gotUser = user
	f.gotNet = net
	return f.done, f.err
}

func fiatTip() gen.Tip {
	return gen.Tip{
		ID:              pgtype.UUID{Valid: true},
		Status:          StatusPending,
		RecipientType:   RecipientUser,
		RecipientUserID: pgtype.UUID{Valid: true},
		NetCredits:      950,
	}
}

func backedSvc(fs *fakeStore, backed BackedSettler) *Service {
	return &Service{
		store:         fs,
		features:      fakeFeatures{on: map[string]bool{"feature.stablecoin_wallet": true}},
		backedSettler: backed,
	}
}

// §128: with the rail live and a usable recipient wallet, a fiat tip marks
// completed via 'backed' and never touches the credits ledger.
func TestSettleFiatTip_BackedPath(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	settler := &fakeBackedSettler{done: true}
	svc := backedSvc(fs, settler)

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if settler.calls != 1 {
		t.Fatalf("backed settler calls = %d, want 1", settler.calls)
	}
	if settler.gotNet != 950 {
		t.Fatalf("backed settler net = %d, want 950", settler.gotNet)
	}
	if fs.fiatSettled {
		t.Fatal("credits settle must not run after a backed settle")
	}
	if len(fs.settledVia) != 1 || fs.settledVia[0] != "backed" {
		t.Fatalf("settledVia = %v, want [backed]", fs.settledVia)
	}
}

// Recipient without a usable wallet (unprovisioned/suspended) keeps the
// legacy credits settle.
func TestSettleFiatTip_BackedFallbackToCredits(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	settler := &fakeBackedSettler{done: false}
	svc := backedSvc(fs, settler)

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !fs.fiatSettled {
		t.Fatal("expected credits-ledger fallback")
	}
	if len(fs.settledVia) != 0 {
		t.Fatalf("settleTip must not run on fallback, got %v", fs.settledVia)
	}
}

// A backed-path error is retryable — never fall back or a retry could
// double-settle (onchain send + ledger credit).
func TestSettleFiatTip_BackedErrorNoFallback(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	settler := &fakeBackedSettler{err: errors.New("killswitch engaged")}
	svc := backedSvc(fs, settler)

	err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{})
	if err == nil {
		t.Fatal("expected error propagation")
	}
	if fs.fiatSettled || len(fs.settledVia) != 0 {
		t.Fatal("no settlement may run after a backed-path error")
	}
}

// Feature off → straight to the legacy credits settle, settler untouched.
func TestSettleFiatTip_FeatureFlagOff(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	settler := &fakeBackedSettler{done: true}
	svc := &Service{
		store:         fs,
		features:      fakeFeatures{on: map[string]bool{}},
		backedSettler: settler,
	}

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if settler.calls != 0 {
		t.Fatal("backed settler must not be consulted with the flag off")
	}
	if !fs.fiatSettled {
		t.Fatal("expected credits-ledger settle")
	}
}

// No seam wired → legacy behavior, no panic.
func TestSettleFiatTip_NilBackedSettler(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	svc := &Service{store: fs, features: fakeFeatures{on: map[string]bool{"feature.stablecoin_wallet": true}}}

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if !fs.fiatSettled {
		t.Fatal("expected credits-ledger settle")
	}
}
