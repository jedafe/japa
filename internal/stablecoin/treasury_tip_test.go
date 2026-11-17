package stablecoin

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

// §128: a fiat tip settles to the recipient's backed wallet — treasury hot
// sends the USDC equivalent, the credits ledger is never touched.

func seedTipRecipient(t *testing.T, st *fakeStore, status string) pgtype.UUID {
	t.Helper()
	uid := newUUID()
	w := &Wallet{
		ID: newUUID().String(), UserID: uid.String(), Status: status,
		Chain: "solana", Address: "rcpt-addr", Locator: "rcpt-loc",
	}
	st.walletByUser[uid.String()] = w
	st.walletByAddr["rcpt-addr"] = w
	st.walletByID[w.ID] = w
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{
		ID: "tw-1", Role: TreasuryHot, Chain: "solana",
		Address: "hot-addr", Locator: "hot-loc",
	}
	return uid
}

func TestSettleTipBacked_TransfersFromTreasury(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	uid := seedTipRecipient(t, st, WalletStatusActive)
	tipID := newUUID()

	done, err := svc.SettleTipBacked(context.Background(), tipID, uid, 950)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v", done, err)
	}
	if len(p.transfers) != 1 {
		t.Fatalf("transfers = %d, want 1", len(p.transfers))
	}
	tr := p.transfers[0]
	if tr.AmountMinor != 9_500_000 { // 950 credits × 10_000 (§58: 100cr = $1)
		t.Fatalf("amount = %d, want 9500000", tr.AmountMinor)
	}
	if tr.RecipientAddr != "rcpt-addr" || tr.Asset != AssetUSDC {
		t.Fatalf("bad transfer leg: %+v", tr)
	}
	if tr.Idempotency != "tip:fiat:"+tipID.String() {
		t.Fatalf("idempotency = %q", tr.Idempotency)
	}
	if tr.Signer != "" {
		t.Fatal("treasury send must not carry a delegated signer")
	}
	if len(st.inserted) != 1 || st.inserted[0].Kind != ActivityTipIn {
		t.Fatalf("mirror rows = %v", st.inserted)
	}
}

// Unprovisioned recipients are auto-provisioned — the counterfactual
// wallet costs nothing dormant and the tip lands backed, never credits.
func TestSettleTipBacked_UnprovisionedAutoProvisions(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	uid := newUUID()
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{
		ID: "tw-1", Role: TreasuryHot, Chain: "solana",
		Address: "hot-addr", Locator: "hot-loc",
	}

	done, err := svc.SettleTipBacked(context.Background(), newUUID(), uid, 950)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v — auto-provision must settle", done, err)
	}
	if _, ok := st.walletByUser[uid.String()]; !ok {
		t.Fatal("recipient wallet was not provisioned")
	}
	if len(p.transfers) != 1 {
		t.Fatalf("transfers = %d, want 1", len(p.transfers))
	}
}

// Suspended wallets are retryable — the tip stays pending until the
// wallet reactivates or the settle TTL fails it (donor refunds). Never
// degrade a suspended member's tip into spendable bonus credits.
func TestSettleTipBacked_SuspendedIsRetryable(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	uid := seedTipRecipient(t, st, WalletStatusSuspended)

	done, err := svc.SettleTipBacked(context.Background(), newUUID(), uid, 950)
	if !errors.Is(err, ErrWalletSuspended) {
		t.Fatalf("err = %v, want ErrWalletSuspended", err)
	}
	if done || len(p.transfers) != 0 {
		t.Fatal("no transfer may fire for a suspended wallet")
	}
}

func TestSettleTipBacked_KillswitchIsRetryable(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	vk := newFakeVK()
	svc := newSvc(st, p, vk, &fakeSettings{})
	uid := seedTipRecipient(t, st, WalletStatusActive)

	if err := svc.SetKillswitch(context.Background(), pgtype.UUID{}, true); err != nil {
		t.Fatal(err)
	}
	done, err := svc.SettleTipBacked(context.Background(), newUUID(), uid, 950)
	if !errors.Is(err, ErrKillswitch) {
		t.Fatalf("err = %v, want ErrKillswitch", err)
	}
	if done || len(p.transfers) != 0 {
		t.Fatal("killswitch must halt the send, not fall back")
	}
}

func TestSettleTipBacked_ProvisionedWalletEligible(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	uid := seedTipRecipient(t, st, WalletStatusProvisioned)

	done, err := svc.SettleTipBacked(context.Background(), newUUID(), uid, 950)
	if err != nil || !done {
		t.Fatalf("done=%v err=%v — provisioned wallets receive onchain", done, err)
	}
}

func TestSettleTipBacked_ZeroNetSkips(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	uid := seedTipRecipient(t, st, WalletStatusActive)

	done, err := svc.SettleTipBacked(context.Background(), newUUID(), uid, 0)
	if err != nil || done {
		t.Fatalf("done=%v err=%v — zero net must skip", done, err)
	}
	if len(p.transfers) != 0 {
		t.Fatal("no transfer for a zero-value tip")
	}
}
