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
		Method:          "stripe",
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

// §125 hardening: the payment-webhook seam refuses lifecycle ops when the
// triggering payment isn't the tip's own §17 row — a stray payment (its
// purpose_id names the tip, but payment_id is the authoritative link) must
// not settle, fail, or claw back a tip it didn't pay for. The admin path
// (invalid paymentID) skips the link check so credits/stablecoin rows stay
// refundable.
func TestPaymentLinkGuards(t *testing.T) {
	ctx := context.Background()

	mkTip := func() gen.Tip {
		tip := fiatTip()
		tip.PaymentID = pgtype.UUID{Valid: true, Bytes: [16]byte{9}}
		return tip
	}
	strayPayment := pgtype.UUID{Valid: true, Bytes: [16]byte{7}}

	t.Run("settle refuses foreign payment", func(t *testing.T) {
		fs := newFakeStore()
		fs.tip, fs.hasTip = mkTip(), true
		if err := (&Service{store: fs}).SettleFiatTip(ctx, fs.tip.ID, strayPayment); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("err = %v, want ErrInvalidStatus", err)
		}
	})

	t.Run("settle refuses non-fiat method", func(t *testing.T) {
		tip := mkTip()
		tip.Method = MethodStablecoin
		fs := newFakeStore()
		fs.tip, fs.hasTip = tip, true
		if err := (&Service{store: fs}).SettleFiatTip(ctx, tip.ID, tip.PaymentID); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("err = %v, want ErrInvalidStatus", err)
		}
	})

	t.Run("fail refuses foreign payment + stablecoin method", func(t *testing.T) {
		fs := newFakeStore()
		fs.tip, fs.hasTip = mkTip(), true
		svc := &Service{store: fs}
		if err := svc.FailTip(ctx, fs.tip.ID, strayPayment); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("stray fail err = %v, want ErrInvalidStatus", err)
		}
		tip := mkTip()
		tip.Method = MethodStablecoin
		fs.tip = tip
		if err := svc.FailTip(ctx, tip.ID, tip.PaymentID); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("stablecoin fail err = %v, want ErrInvalidStatus", err)
		}
	})

	t.Run("fail accepts own payment", func(t *testing.T) {
		fs := newFakeStore()
		fs.tip, fs.hasTip = mkTip(), true
		if err := (&Service{store: fs}).FailTip(ctx, fs.tip.ID, fs.tip.PaymentID); err != nil {
			t.Fatalf("own-payment fail err = %v", err)
		}
	})

	t.Run("webhook clawback refuses foreign payment", func(t *testing.T) {
		tip := mkTip()
		tip.Status = StatusCompleted
		fs := newFakeStore()
		fs.tip, fs.hasTip = tip, true
		if err := (&Service{store: fs}).ClawbackTip(ctx, tip.ID, strayPayment); !errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("err = %v, want ErrInvalidStatus", err)
		}
	})

	t.Run("admin clawback skips link check on non-fiat tip", func(t *testing.T) {
		tip := mkTip()
		tip.Method = MethodStablecoin
		tip.PaymentID = pgtype.UUID{} // onchain rows never carry a §17 link
		fs := newFakeStore()
		fs.tip, fs.hasTip = tip, true
		// invalid paymentID = admin path → past the gate it hits the
		// pending arm (refundPendingTip) — anything but ErrInvalidStatus
		// from the gate proves the guard skipped correctly.
		err := (&Service{store: fs}).ClawbackTip(ctx, tip.ID, pgtype.UUID{})
		if errors.Is(err, ErrInvalidStatus) {
			t.Fatalf("admin-path err = %v, want no gate error", err)
		}
	})
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

// done=false means there is no member wallet at all (platform recipient /
// zero-value row) — the bookkeeping settle marks the tip complete with no
// ledger write.
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

// The claim is the money-movement gate: losing it (terminal state reached
// between read and claim — e.g. a concurrent settle already completed)
// must short-circuit before the provider send or the ledger write.
func TestSettleFiatTip_ClaimLost_NoSend(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	fs.claimDenied = true
	settler := &fakeBackedSettler{done: true}
	svc := backedSvc(fs, settler)

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("claim-lost must be an idempotent no-op, got %v", err)
	}
	if settler.calls != 0 {
		t.Fatal("provider send must not run after losing the claim")
	}
	if fs.fiatSettled || len(fs.settledVia) != 0 {
		t.Fatal("no ledger/settle write may run after losing the claim")
	}
}

// A 'settling' tip is a claim held by a prior attempt — a retried task
// re-enters through the same claim path rather than ErrInvalidStatus.
func TestSettleFiatTip_RetryFromSettling(t *testing.T) {
	fs := newFakeStore()
	fs.tip, fs.hasTip = fiatTip(), true
	fs.tip.Status = StatusSettling
	settler := &fakeBackedSettler{done: true}
	svc := backedSvc(fs, settler)

	if err := svc.SettleFiatTip(context.Background(), fs.tip.ID, pgtype.UUID{}); err != nil {
		t.Fatalf("settling retry must proceed, got %v", err)
	}
	if settler.calls != 1 {
		t.Fatalf("backed settler calls = %d, want 1", settler.calls)
	}
	if len(fs.settledVia) != 1 || fs.settledVia[0] != "backed" {
		t.Fatalf("settledVia = %v, want [backed]", fs.settledVia)
	}
}
