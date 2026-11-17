package stablecoin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ── Fakes ────────────────────────────────────────────────────────────────

type fakeSettings struct {
	strs map[string]string
	ints map[string]int
	bols map[string]bool
}

func (f *fakeSettings) GetStringOr(_ context.Context, k, fb string) string {
	if v, ok := f.strs[k]; ok {
		return v
	}
	return fb
}
func (f *fakeSettings) GetIntOr(_ context.Context, k string, fb int) int {
	if v, ok := f.ints[k]; ok {
		return v
	}
	return fb
}
func (f *fakeSettings) GetBoolOr(_ context.Context, k string, fb bool) bool {
	if v, ok := f.bols[k]; ok {
		return v
	}
	return fb
}

type fakeVK struct {
	m map[string]string
}

func newFakeVK() *fakeVK { return &fakeVK{m: map[string]string{}} }

func (f *fakeVK) Exists(_ context.Context, k string) (bool, error) {
	_, ok := f.m[k]
	return ok, nil
}
func (f *fakeVK) SetNX(_ context.Context, k, v string, _ int) (bool, error) {
	if _, ok := f.m[k]; ok {
		return false, nil
	}
	f.m[k] = v
	return true, nil
}
func (f *fakeVK) Get(_ context.Context, k string) (string, error) { return f.m[k], nil }
func (f *fakeVK) Set(_ context.Context, k, v string, _ int) error {
	f.m[k] = v
	return nil
}
func (f *fakeVK) Del(_ context.Context, k string) error {
	delete(f.m, k)
	return nil
}

type fakeStore struct {
	walletByUser map[string]*Wallet
	walletByAddr map[string]*Wallet
	walletByID   map[string]*Wallet
	activities   map[string]*Activity // tx_sig → row
	inserted     []Activity
	escrows      map[string]*Escrow
	bookings     map[string]*ServiceBooking
	treasury     map[string]*TreasuryWallet // chain:role → wallet
	sweeps       []TreasurySweep
	outSum       int64
	pending      []Activity
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		walletByUser: map[string]*Wallet{},
		walletByAddr: map[string]*Wallet{},
		walletByID:   map[string]*Wallet{},
		activities:   map[string]*Activity{},
		escrows:      map[string]*Escrow{},
		bookings:     map[string]*ServiceBooking{},
		treasury:     map[string]*TreasuryWallet{},
	}
}

func (f *fakeStore) GetWalletByUser(_ context.Context, u pgtype.UUID) (*Wallet, error) {
	if w, ok := f.walletByUser[u.String()]; ok {
		return w, nil
	}
	return nil, ErrWalletNotFound
}
func (f *fakeStore) GetWalletByAddress(_ context.Context, a string) (*Wallet, error) {
	if w, ok := f.walletByAddr[a]; ok {
		return w, nil
	}
	return nil, ErrWalletNotFound
}
func (f *fakeStore) GetWalletByID(_ context.Context, id pgtype.UUID) (*Wallet, error) {
	if w, ok := f.walletByID[id.String()]; ok {
		return w, nil
	}
	return nil, ErrWalletNotFound
}
func (f *fakeStore) InsertWallet(_ context.Context, w Wallet) (*Wallet, error) {
	if _, dup := f.walletByUser[w.UserID]; dup {
		return nil, fmt.Errorf("duplicate user")
	}
	if w.ID == "" {
		w.ID = newUUID().String()
	}
	f.walletByUser[w.UserID] = &w
	f.walletByAddr[w.Address] = &w
	f.walletByID[w.ID] = &w
	return &w, nil
}
func (f *fakeStore) UpdateWalletStatus(_ context.Context, id pgtype.UUID, st string) error {
	if w, ok := f.walletByID[id.String()]; ok {
		w.Status = st
	}
	return nil
}
func (f *fakeStore) UpdateDelegatedSigner(_ context.Context, id pgtype.UUID, ds *DelegatedSigner) error {
	if w, ok := f.walletByID[id.String()]; ok {
		w.DelegatedSigner = ds
	}
	return nil
}
func (f *fakeStore) ListWalletsByStatus(_ context.Context, st string, _, _ int32) ([]Wallet, error) {
	var out []Wallet
	for _, w := range f.walletByID {
		if w.Status == st {
			out = append(out, *w)
		}
	}
	return out, nil
}
func (f *fakeStore) ListWallets(_ context.Context, _, _ string, _, _ int32) ([]Wallet, error) {
	return nil, nil
}
func (f *fakeStore) InsertActivity(_ context.Context, a Activity) (*Activity, error) {
	if _, dup := f.activities[a.TxSignature]; dup {
		return nil, nil // ON CONFLICT DO NOTHING
	}
	f.activities[a.TxSignature] = &a
	f.inserted = append(f.inserted, a)
	return &a, nil
}
func (f *fakeStore) GetActivityBySig(_ context.Context, sig string) (*Activity, error) {
	if a, ok := f.activities[sig]; ok {
		return a, nil
	}
	return nil, ErrActivityNotFound
}
func (f *fakeStore) ListActivityByWallet(_ context.Context, wid pgtype.UUID, _, _ int32) ([]Activity, error) {
	var out []Activity
	for _, a := range f.activities {
		if a.WalletID == wid.String() {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (f *fakeStore) ListPendingActivity(_ context.Context, _ int32) ([]Activity, error) {
	return f.pending, nil
}
func (f *fakeStore) SetActivityStatus(_ context.Context, id pgtype.UUID, st string) error {
	for _, a := range f.activities {
		if a.ID == id.String() {
			a.Status = st
		}
	}
	return nil
}
func (f *fakeStore) SumOutByAssetSince(_ context.Context, _ pgtype.UUID, _ string, _ time.Time) (int64, error) {
	return f.outSum, nil
}
func (f *fakeStore) GetEscrow(_ context.Context, id pgtype.UUID) (*Escrow, error) {
	if e, ok := f.escrows[id.String()]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStore) GetEscrowByBooking(_ context.Context, b pgtype.UUID) (*Escrow, error) {
	for _, e := range f.escrows {
		if e.BookingID == b.String() {
			return e, nil
		}
	}
	return nil, nil
}
func (f *fakeStore) GetEscrowByVault(_ context.Context, vault string) (*Escrow, error) {
	for _, e := range f.escrows {
		if e.VaultAddress == vault {
			return e, nil
		}
	}
	return nil, ErrEscrowNotFound
}
func (f *fakeStore) GetLiveEscrowByWallet(_ context.Context, w pgtype.UUID) (*Escrow, error) {
	for _, e := range f.escrows {
		if (e.BuyerWalletID == w.String() || e.SellerWalletID == w.String()) &&
			(e.State == EscrowFunding || e.State == EscrowFunded || e.State == EscrowDisputed) {
			return e, nil
		}
	}
	return nil, ErrEscrowNotFound
}
func (f *fakeStore) InsertEscrow(_ context.Context, e Escrow) (*Escrow, error) {
	if e.ID == "" {
		e.ID = newUUID().String()
	}
	f.escrows[e.ID] = &e
	return &e, nil
}
func (f *fakeStore) SetEscrowState(_ context.Context, id pgtype.UUID, st, fundedSig, releaseSig string, d *ArbiterDecision, dl *time.Time) error {
	e, ok := f.escrows[id.String()]
	if !ok {
		return fmt.Errorf("not found")
	}
	e.State = st
	if fundedSig != "" {
		e.FundedSig = fundedSig
	}
	if releaseSig != "" {
		e.ReleaseSig = releaseSig
	}
	if d != nil {
		e.ArbiterDecision = d
	}
	if dl != nil {
		e.ConfirmationDeadline = *dl
	}
	return nil
}
func (f *fakeStore) SetEscrowVaultAddress(_ context.Context, id pgtype.UUID, addr string) error {
	if e, ok := f.escrows[id.String()]; ok {
		e.VaultAddress = addr
	}
	return nil
}
func (f *fakeStore) ListExpiredFundedEscrows(_ context.Context, _ int32) ([]Escrow, error) {
	return nil, nil
}
func (f *fakeStore) GetServiceBooking(_ context.Context, id pgtype.UUID) (*ServiceBooking, error) {
	if b, ok := f.bookings[id.String()]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStore) ListTreasuryWallets(_ context.Context) ([]TreasuryWallet, error) {
	var out []TreasuryWallet
	for _, t := range f.treasury {
		out = append(out, *t)
	}
	return out, nil
}
func (f *fakeStore) GetTreasuryWallet(_ context.Context, chain, role string) (*TreasuryWallet, error) {
	if t, ok := f.treasury[chain+":"+role]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStore) InsertTreasurySweep(_ context.Context, sw TreasurySweep) (*TreasurySweep, error) {
	if sw.State == "" {
		sw.State = "proposed"
	}
	f.sweeps = append(f.sweeps, sw)
	return &sw, nil
}
func (f *fakeStore) GetTreasurySweep(_ context.Context, id pgtype.UUID) (*TreasurySweep, error) {
	for i := range f.sweeps {
		if f.sweeps[i].ID == id.String() {
			return &f.sweeps[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeStore) DecideTreasurySweep(_ context.Context, id pgtype.UUID, st string, _ pgtype.UUID, _ string) error {
	for i := range f.sweeps {
		if f.sweeps[i].ID == id.String() {
			f.sweeps[i].State = st
		}
	}
	return nil
}
func (f *fakeStore) ListSweepsByState(_ context.Context, st string, _, _ int32) ([]TreasurySweep, error) {
	var out []TreasurySweep
	for _, sw := range f.sweeps {
		if sw.State == st {
			out = append(out, sw)
		}
	}
	return out, nil
}
func (f *fakeStore) GetTreasuryWalletByAddress(_ context.Context, chain, addr string) (*TreasuryWallet, error) {
	for _, t := range f.treasury {
		if t.Chain == chain && t.Address == addr {
			return t, nil
		}
	}
	return nil, ErrTreasuryNotFound
}
func (f *fakeStore) GetApprovedSweepForTransfer(_ context.Context, _, asset, hotID string, amt int64) (*TreasurySweep, error) {
	for i := range f.sweeps {
		if f.sweeps[i].State == "approved" && f.sweeps[i].Asset == asset &&
			f.sweeps[i].HotWalletID == hotID && f.sweeps[i].AmountMinor <= amt {
			return &f.sweeps[i], nil
		}
	}
	return nil, ErrSweepNotFound
}
func (f *fakeStore) MarkSweepExecuted(_ context.Context, id pgtype.UUID, sig string) error {
	for i := range f.sweeps {
		if f.sweeps[i].ID == id.String() {
			f.sweeps[i].State = "executed"
			f.sweeps[i].ExecutedSig = sig
		}
	}
	return nil
}

type fakeProvider struct {
	transfers     []TransferInput
	removedSigner []string
	verifyErr     error
	activity      []ProviderEvent
	balances      map[string]int64
}

func (f *fakeProvider) CreateWallet(_ context.Context, _ CreateWalletInput) (ProviderWallet, error) {
	return ProviderWallet{Locator: "loc-1", Address: "addr-1", Chain: "solana"}, nil
}
func (f *fakeProvider) GetWallet(_ context.Context, _ string) (ProviderWallet, error) {
	return ProviderWallet{Balances: f.balances}, nil
}
func (f *fakeProvider) RegisterDelegatedSigner(_ context.Context, _ string, _ DelegatedSignerInput) error {
	return nil
}
func (f *fakeProvider) RemoveDelegatedSigner(_ context.Context, _ string, ref string) error {
	f.removedSigner = append(f.removedSigner, ref)
	return nil
}
func (f *fakeProvider) Transfer(_ context.Context, _ string, in TransferInput) (TransferResult, error) {
	f.transfers = append(f.transfers, in)
	return TransferResult{TxSignature: "sig-" + in.Idempotency, Status: "confirmed"}, nil
}
func (f *fakeProvider) BuildWithdrawal(_ context.Context, _ string, _ WithdrawalInput) (SignableOp, error) {
	return SignableOp{Locator: "op-1"}, nil
}
func (f *fakeProvider) CreateOnrampSession(_ context.Context, _ OnrampInput) (OnrampSession, error) {
	return OnrampSession{URL: "https://checkout"}, nil
}
func (f *fakeProvider) Activity(_ context.Context, _ string, _ string) ([]ProviderEvent, string, error) {
	return f.activity, "", nil
}
func (f *fakeProvider) VerifyWebhook(_ []byte, _ string) error { return f.verifyErr }

type fakeEnq struct{ tasks []string }

func (f *fakeEnq) Enqueue(t string, _ []byte, _ ...interface{}) error {
	f.tasks = append(f.tasks, t)
	return nil
}

type fakeBonus struct{ rows []BonusEntry }

func (f *fakeBonus) ListCreditEntries(_ context.Context, _ pgtype.UUID, _, _ int32) ([]BonusEntry, error) {
	return f.rows, nil
}

type fakeRater struct{ n float64 }

func (f *fakeRater) CreditsPerUnit(_ context.Context, _ string) (float64, error) {
	return f.n, nil
}

type fakeTips struct {
	tips    map[string]*Tip
	settled []string
}

func (f *fakeTips) GetTip(_ context.Context, id pgtype.UUID) (*Tip, error) {
	if t, ok := f.tips[id.String()]; ok {
		return t, nil
	}
	return nil, fmt.Errorf("tip not found")
}
func (f *fakeTips) ClaimTipSettle(_ context.Context, id pgtype.UUID) (bool, error) {
	if t, ok := f.tips[id.String()]; ok {
		if t.Status == "completed" || t.Status == "refunded" {
			return false, nil
		}
		t.Status = "settling"
		return true, nil
	}
	return false, fmt.Errorf("tip not found")
}
func (f *fakeTips) MarkSettled(_ context.Context, id pgtype.UUID, sig string) error {
	f.settled = append(f.settled, id.String()+":"+sig)
	if t, ok := f.tips[id.String()]; ok {
		t.Status = "completed"
	}
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────

var testUUIDSeq atomic.Int64

func newUUID() pgtype.UUID {
	var u pgtype.UUID
	_ = u.Scan(fmt.Sprintf("019b0000-0000-7000-8000-%012x", testUUIDSeq.Add(1)))
	return u
}

func uuidOf(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatalf("bad uuid %s: %v", s, err)
	}
	return u
}

func newSvc(st *fakeStore, p *fakeProvider, vk *fakeVK, set *fakeSettings) *Service {
	return NewService(p, st, set, vk)
}

// ── assetList ────────────────────────────────────────────────────────────

func TestAssetListParsing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		raw  string
		want []string
	}{
		{`["usdc","usdt"]`, []string{"usdc", "usdt"}},
		{`["usdc"]`, []string{"usdc"}},
		{"usdc,usdt", []string{"usdc", "usdt"}},
		{"usdc, bogus ,usdt", []string{"usdc", "usdt"}},
		{"", []string{"usdc", "usdt"}},
		{`["eurc"]`, []string{"usdc", "usdt"}}, // unsupported only → default
	}
	for _, c := range cases {
		svc := newSvc(newFakeStore(), &fakeProvider{}, newFakeVK(), &fakeSettings{
			strs: map[string]string{"stablecoin.assets": c.raw},
		})
		got := svc.assetList(context.Background())
		if len(got) != len(c.want) {
			t.Fatalf("assetList(%q) = %v, want %v", c.raw, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("assetList(%q)[%d] = %s, want %s", c.raw, i, got[i], c.want[i])
			}
		}
	}
}

// ── Killswitch ───────────────────────────────────────────────────────────

func TestKillswitchSettleBlock(t *testing.T) {
	t.Parallel()
	vk := newFakeVK()
	st := newFakeStore()
	svc := newSvc(st, &fakeProvider{}, vk, &fakeSettings{})
	uid := newUUID()

	if err := svc.SetKillswitch(context.Background(), pgtype.UUID{}, true); err != nil {
		t.Fatal(err)
	}
	_, err := svc.Settle(context.Background(), SettleInput{FromUserID: uid, Asset: AssetUSDC, AmountMinor: 100})
	if !errors.Is(err, ErrKillswitch) {
		t.Fatalf("settle under killswitch = %v, want ErrKillswitch", err)
	}
	if err := svc.SetKillswitch(context.Background(), pgtype.UUID{}, false); err != nil {
		t.Fatal(err)
	}
	if halted, _ := svc.killswitch(context.Background()); halted {
		t.Fatal("killswitch still engaged after Del")
	}
}

func TestSettleScopeCapEnforced(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	uid := newUUID()
	w := &Wallet{
		ID: newUUID().String(), UserID: uid.String(), Status: WalletStatusActive,
		Locator: "loc", Chain: "solana",
		DelegatedSigner: &DelegatedSigner{SignerRef: "ref", Scopes: []SignerScope{
			{Asset: AssetUSDC, DailyCapMinor: 1000},
		}},
	}
	st.walletByUser[uid.String()] = w
	st.walletByAddr["dest"] = &Wallet{Address: "dest", Status: WalletStatusActive} // known recipient
	st.outSum = 950                                                                // today's settled volume per SQL aggregate
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	// Unknown recipients are rejected before any provider call.
	if _, err := svc.Settle(context.Background(), SettleInput{
		FromUserID: uid, ToAddress: "attacker-addr", Asset: AssetUSDC, AmountMinor: 100,
	}); !errors.Is(err, ErrUnknownRecipient) {
		t.Fatalf("unknown recipient = %v, want ErrUnknownRecipient", err)
	}
	// Suspended member wallets reject too — known address, wrong status.
	st.walletByAddr["susp-addr"] = &Wallet{Address: "susp-addr", Status: WalletStatusSuspended}
	if _, err := svc.Settle(context.Background(), SettleInput{
		FromUserID: uid, ToAddress: "susp-addr", Asset: AssetUSDC, AmountMinor: 100,
	}); !errors.Is(err, ErrUnknownRecipient) {
		t.Fatalf("suspended recipient = %v, want ErrUnknownRecipient", err)
	}

	_, err := svc.Settle(context.Background(), SettleInput{
		FromUserID: uid, ToAddress: "dest", Asset: AssetUSDC, AmountMinor: 100,
	})
	if !errors.Is(err, ErrScopeExceeded) {
		t.Fatalf("cap breach = %v, want ErrScopeExceeded", err)
	}

	// Under cap passes through to the provider.
	st.outSum = 100
	p := &fakeProvider{}
	svc = newSvc(st, p, newFakeVK(), &fakeSettings{})
	if _, err := svc.Settle(context.Background(), SettleInput{
		FromUserID: uid, ToAddress: "dest", Asset: AssetUSDC, AmountMinor: 100,
	}); err != nil {
		t.Fatalf("under-cap settle: %v", err)
	}
	if len(p.transfers) != 1 {
		t.Fatalf("provider transfers = %d, want 1", len(p.transfers))
	}
}

// ── Webhook ingest ───────────────────────────────────────────────────────

func TestIngestWebhookDropsNonTxEvents(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	enq := &fakeEnq{}
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	svc.SetEnqueuer(enq)

	// Verified event with no transaction signature — ack + drop.
	err := svc.IngestWebhook(context.Background(), []byte(`{"type":"wallet.created","wallet_address":"a1"}`), "sig")
	if err != nil {
		t.Fatalf("non-tx webhook = %v, want nil", err)
	}
	if len(enq.tasks) != 0 || len(st.inserted) != 0 {
		t.Fatal("non-tx webhook should not enqueue or mirror")
	}
}

func TestIngestWebhookDedupesReplay(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	enq := &fakeEnq{}
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	svc.SetEnqueuer(enq)

	payload := []byte(`{"type":"transfer_in","tx_signature":"sig-1","wallet_address":"a1"}`)
	if err := svc.IngestWebhook(context.Background(), payload, "s"); err != nil {
		t.Fatal(err)
	}
	if err := svc.IngestWebhook(context.Background(), payload, "s"); err != nil {
		t.Fatal(err)
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("replayed webhook enqueued %d times, want 1", len(enq.tasks))
	}
}

func TestIngestWebhookRejectsBadSignature(t *testing.T) {
	t.Parallel()
	svc := newSvc(newFakeStore(), &fakeProvider{verifyErr: errors.New("bad hmac")}, newFakeVK(), &fakeSettings{})
	if err := svc.IngestWebhook(context.Background(), []byte(`{}`), "x"); err == nil {
		t.Fatal("unverified webhook accepted")
	}
}

// ── MirrorActivity ───────────────────────────────────────────────────────

func TestMirrorActivityTimestampFallback(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	before := time.Now().UTC()
	err := svc.MirrorActivity(context.Background(), ProviderEvent{
		Type: "transfer_in", TxSignature: "sig-9", Asset: "solana:usdc",
		AmountMinor: 500, OccurredAt: 0,
	}, newUUID().String())
	if err != nil {
		t.Fatal(err)
	}
	got := st.inserted[0]
	if got.OccurredAt.Before(before) {
		t.Fatalf("occurred_at %v predates call — 1970 fallback bug", got.OccurredAt)
	}
	if got.Asset != AssetUSDC {
		t.Fatalf("asset = %s, solana: prefix not stripped", got.Asset)
	}
	if got.Status != "confirmed" || got.Direction != "in" {
		t.Fatalf("row = %+v", got)
	}
}

func TestMirrorWithdrawalCollectsFee(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	uid := newUUID()
	w := &Wallet{
		ID: newUUID().String(), UserID: uid.String(), Status: WalletStatusActive,
		Chain: "solana", Locator: "loc",
		DelegatedSigner: &DelegatedSigner{SignerRef: "ref"},
	}
	st.walletByID[w.ID] = w
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{Role: TreasuryHot, Chain: "solana", Address: "hot-addr"}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{ints: map[string]int{"stablecoin.withdrawal_fee_bps": 100}})

	err := svc.MirrorActivity(context.Background(), ProviderEvent{
		Type: "transfer_out", TxSignature: "wd-1", Asset: "usdc", AmountMinor: 10_000,
	}, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.transfers) != 1 {
		t.Fatalf("fee transfers = %d, want 1", len(p.transfers))
	}
	fee := p.transfers[0]
	if fee.AmountMinor != 100 || fee.RecipientAddr != "hot-addr" || fee.Idempotency != "wd-1:withdrawal_fee" {
		t.Fatalf("fee leg = %+v", fee)
	}
}

func TestMirrorActivityIdempotentOnSig(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	ev := ProviderEvent{Type: "transfer_in", TxSignature: "dup-1", Asset: "usdc", AmountMinor: 1}
	wid := newUUID().String()
	_ = svc.MirrorActivity(context.Background(), ev, wid)
	if err := svc.MirrorActivity(context.Background(), ev, wid); err != nil {
		t.Fatal(err)
	}
	if len(st.inserted) != 1 {
		t.Fatalf("duplicate mirror inserted %d rows", len(st.inserted))
	}
}

// ── Escrow state machine ─────────────────────────────────────────────────

func fundedEscrow(st *fakeStore, buyerUID, sellerUID string) *Escrow {
	bw := &Wallet{ID: newUUID().String(), UserID: buyerUID}
	sw := &Wallet{ID: newUUID().String(), UserID: sellerUID}
	st.walletByID[bw.ID] = bw
	st.walletByID[sw.ID] = sw
	e := &Escrow{
		ID: newUUID().String(), BookingID: newUUID().String(),
		BuyerWalletID: bw.ID, SellerWalletID: sw.ID,
		State: EscrowFunded, FundedSig: "fund-sig",
	}
	st.escrows[e.ID] = e
	return e
}

func TestConfirmReleaseRequiresFunded(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowDisputed
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	err := svc.ConfirmRelease(context.Background(), uuidOf(t, e.ID), buyer, "sig")
	if !errors.Is(err, ErrEscrowState) {
		t.Fatalf("confirm on disputed = %v, want ErrEscrowState", err)
	}
}

func TestConfirmReleaseBuyerOnly(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	err := svc.ConfirmRelease(context.Background(), uuidOf(t, e.ID), seller, "sig")
	if !errors.Is(err, ErrEscrowParty) {
		t.Fatalf("seller confirm-release = %v, want ErrEscrowParty", err)
	}

	if err := svc.ConfirmRelease(context.Background(), uuidOf(t, e.ID), buyer, "rel-sig"); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowReleased || e.ReleaseSig != "rel-sig" {
		t.Fatalf("escrow = %+v", e)
	}
}

func TestDisputePreservesFundedSig(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	if err := svc.DisputeEscrow(context.Background(), uuidOf(t, e.ID), seller, "no delivery"); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowDisputed || e.FundedSig != "fund-sig" {
		t.Fatalf("dispute wiped state: %+v", e)
	}
	if e.ArbiterDecision == nil || !strings.Contains(e.ArbiterDecision.Note, "no delivery") {
		t.Fatalf("dispute reason not recorded: %+v", e.ArbiterDecision)
	}
}

func TestArbitrateTwoPhase(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowDisputed
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	eid := uuidOf(t, e.ID)

	// Phase 2 before a ruling — rejected.
	if err := svc.ArbitrateEscrow(context.Background(), eid, nil, "exec-sig"); err == nil {
		t.Fatal("release sig accepted without a ruling")
	}

	// Phase 1 records the ruling; escrow stays disputed.
	d := &ArbiterDecision{ReleasedTo: "seller", DecidedBy: "admin-1", DecidedAt: time.Now()}
	if err := svc.ArbitrateEscrow(context.Background(), eid, d, ""); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowDisputed || e.ArbiterDecision == nil {
		t.Fatalf("phase 1 = %+v", e)
	}

	// Phase 2 completes on the executed sig.
	if err := svc.ArbitrateEscrow(context.Background(), eid, nil, "exec-sig"); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowReleased || e.ReleaseSig != "exec-sig" {
		t.Fatalf("phase 2 = %+v", e)
	}
}

// ── Unified feed ─────────────────────────────────────────────────────────

func TestUnifiedFeedMergesTiers(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	uid := newUUID()
	w := &Wallet{ID: newUUID().String(), UserID: uid.String()}
	st.walletByUser[uid.String()] = w
	now := time.Now().UTC()
	st.activities["s1"] = &Activity{
		ID: newUUID().String(), WalletID: w.ID, Kind: ActivityDeposit,
		Direction: "in", CreditsAmount: 500, OccurredAt: now.Add(-time.Hour),
	}
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	svc.SetBonusLedger(&fakeBonus{rows: []BonusEntry{
		{ID: "b1", Operation: "earn", Amount: 50, OccurredAt: now},
		{ID: "b2", Operation: "spend", Amount: -20, OccurredAt: now.Add(-2 * time.Hour)},
	}})

	feed, err := svc.ListActivity(context.Background(), uid, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(feed) != 3 {
		t.Fatalf("feed = %d rows, want 3", len(feed))
	}
	if feed[0].Tier != "bonus" || feed[0].Kind != "bonus_earn" {
		t.Fatalf("newest row = %+v, want bonus_earn", feed[0])
	}
	if feed[2].Kind != "bonus_spend" || feed[2].Direction != "out" {
		t.Fatalf("oldest row = %+v", feed[2])
	}
	if feed[1].Tier != "backed" {
		t.Fatalf("onchain row = %+v", feed[1])
	}
}

// ── Treasury sweeps ──────────────────────────────────────────────────────

func TestProposeSweepSkipsOpenProposal(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{ID: "hot-1", Role: TreasuryHot, Chain: "solana", Locator: "hot-loc"}
	st.treasury["solana:"+TreasuryCold] = &TreasuryWallet{Role: TreasuryCold, Chain: "solana", Address: "cold-addr"}
	p := &fakeProvider{balances: map[string]int64{"usdc": 5_000_000}}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{
		strs: map[string]string{"stablecoin.network": "solana", "stablecoin.assets": `["usdc"]`},
		ints: map[string]int{"stablecoin.hot_float_cap_minor": 2_500_000},
	})

	if err := svc.ProposeSweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.sweeps) != 1 || st.sweeps[0].AmountMinor != 2_500_000 {
		t.Fatalf("sweeps = %+v", st.sweeps)
	}
	// Second tick while the proposal is open — no duplicate.
	if err := svc.ProposeSweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(st.sweeps) != 1 {
		t.Fatalf("duplicate sweep proposed: %d rows", len(st.sweeps))
	}
}

// ── tips:settle_stablecoin ───────────────────────────────────────────────

func TestProcessTipSettleLifecycle(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	sender, recip := newUUID(), newUUID()
	for _, u := range []pgtype.UUID{sender, recip} {
		w := &Wallet{
			ID: newUUID().String(), UserID: u.String(), Status: WalletStatusActive,
			Chain: "solana", Locator: "loc", Address: "addr-" + u.String()[:8],
			DelegatedSigner: &DelegatedSigner{SignerRef: "ref", Scopes: []SignerScope{
				{Asset: AssetUSDC, DailyCapMinor: 50_000_000}, // $50 — must cover the converted tip
			}},
		}
		st.walletByUser[u.String()] = w
		st.walletByAddr[w.Address] = w // real wallets resolve by address too
	}
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{Role: TreasuryHot, Chain: "solana", Address: "hot"}
	tid := newUUID()
	tips := &fakeTips{tips: map[string]*Tip{
		tid.String(): {ID: tid, SenderUserID: sender, RecipientUserID: recip, NetCredits: 950, FeeAmount: 50, Status: "pending"},
	}}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{strs: map[string]string{"stablecoin.assets": `["usdc"]`}})
	svc.SetTipReader(tips)
	svc.SetCreditRater(&fakeRater{n: 100}) // §58 rate — settle math fails closed without it

	payload, _ := json.Marshal(TipSettlePayload{TipID: tid.String()})
	if err := svc.ProcessTipSettle(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(p.transfers) != 1 {
		t.Fatalf("settles = %d, want 1", len(p.transfers))
	}
	// Tip legs are credits — the onchain op is USD-token minor (§58 rate,
	// fallback 100 credits/$): 950 credits → 9.5 USDC → 9_500_000 minor.
	if p.transfers[0].AmountMinor != 9_500_000 || p.transfers[0].FeeMinor != 500_000 || p.transfers[0].Idempotency != "tip:"+tid.String() {
		t.Fatalf("settle legs = %+v", p.transfers[0])
	}
	if tips.tips[tid.String()].Status != "completed" {
		t.Fatalf("tip status = %s — lifecycle not closed", tips.tips[tid.String()].Status)
	}
	if len(tips.settled) != 1 {
		t.Fatalf("MarkSettled calls = %d", len(tips.settled))
	}
}

func TestSignerExpiringWithin(t *testing.T) {
	t.Parallel()
	cutoff := time.Now().Add(30 * 24 * time.Hour)
	cases := []struct {
		name string
		w    *Wallet
		want bool
	}{
		{"no signer", &Wallet{}, false},
		{"expires in 10d", &Wallet{DelegatedSigner: &DelegatedSigner{Scopes: []SignerScope{
			{Asset: "usdc", ExpiresAt: time.Now().Add(10 * 24 * time.Hour)}}}}, true},
		{"expires in 300d", &Wallet{DelegatedSigner: &DelegatedSigner{Scopes: []SignerScope{
			{Asset: "usdc", ExpiresAt: time.Now().Add(300 * 24 * time.Hour)}}}}, false},
		{"nearest scope wins", &Wallet{DelegatedSigner: &DelegatedSigner{Scopes: []SignerScope{
			{Asset: "usdt", ExpiresAt: time.Now().Add(300 * 24 * time.Hour)},
			{Asset: "usdc", ExpiresAt: time.Now().Add(5 * 24 * time.Hour)}}}}, true},
	}
	for _, c := range cases {
		if got := signerExpiringWithin(c.w, cutoff); got != c.want {
			t.Errorf("%s: signerExpiringWithin = %v, want %v", c.name, got, c.want)
		}
	}
}

// ── Escrow party gates (member-side mutations) ──────────────────────────

func TestEscrowPartyGates(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller, stranger := newUUID(), newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowFunding
	eid := uuidOf(t, e.ID)
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})

	if err := svc.RegisterEscrowVault(context.Background(), eid, stranger, "vault-1"); !errors.Is(err, ErrEscrowParty) {
		t.Fatalf("stranger vault registration = %v, want ErrEscrowParty", err)
	}
	if err := svc.RegisterEscrowVault(context.Background(), eid, buyer, "pending:"+e.BookingID); err == nil {
		t.Fatal("placeholder-shaped vault address must be rejected")
	}
	if err := svc.ConfirmEscrowFunded(context.Background(), eid, stranger, "sig"); !errors.Is(err, ErrEscrowParty) {
		t.Fatalf("stranger funded confirm = %v, want ErrEscrowParty", err)
	}
	// A party may proceed.
	vault := "EscrowVau1t1111111111111111111111111111111" // ≥32-char plausible base58 shape
	if err := svc.RegisterEscrowVault(context.Background(), eid, buyer, vault); err != nil {
		t.Fatalf("buyer vault registration: %v", err)
	}
}

func TestPrepareEscrowGates(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller, stranger := newUUID(), newUUID(), newUUID()
	for _, u := range []pgtype.UUID{buyer, seller} {
		w := &Wallet{
			ID: newUUID().String(), UserID: u.String(), Address: "addr-" + u.String()[:8],
		}
		st.walletByUser[u.String()] = w
		st.walletByID[w.ID] = w
	}
	bookingID := newUUID()
	mk := func(status string) {
		st.bookings[bookingID.String()] = &ServiceBooking{
			ID: bookingID.String(), BookerUserID: buyer.String(), ProviderUserID: seller.String(),
			TotalPrice: 1000, FeeCurrency: "usd", Status: status,
		}
	}
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{
		strs: map[string]string{"stablecoin.escrow_arbiter_ref": "arb-key", "stablecoin.assets": `["usdc"]`},
	})

	mk("pending")
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, buyer, "usdc"); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("pending booking = %v, want ErrEscrowState", err)
	}
	mk("cancelled")
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, buyer, "usdc"); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("cancelled booking = %v, want ErrEscrowState", err)
	}
	mk("confirmed")
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, stranger, "usdc"); !errors.Is(err, ErrEscrowParty) {
		t.Fatalf("stranger prepare = %v, want ErrEscrowParty", err)
	}
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, buyer, "eurc"); err == nil {
		t.Fatal("unsupported asset accepted")
	}
	setup, err := svc.PrepareEscrow(context.Background(), bookingID, buyer, "usdc")
	if err != nil {
		t.Fatal(err)
	}
	// $10.00 booking (1000 fiat minor) → 10_000_000 USD-token minor;
	// 15% platform share → 1_500_000 fee / 8_500_000 seller leg.
	if setup.ArbiterAddress != "arb-key" || setup.FeeMinor != 1_500_000 || setup.AmountMinor != 8_500_000 {
		t.Fatalf("setup = %+v", setup)
	}
	// Idempotent per booking.
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, seller, "usdc"); err != nil {
		t.Fatalf("idempotent re-prepare: %v", err)
	}
}

func TestPrepareEscrowFailsWithoutArbiter(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	for _, u := range []pgtype.UUID{buyer, seller} {
		w := &Wallet{ID: newUUID().String(), UserID: u.String()}
		st.walletByUser[u.String()] = w
		st.walletByID[w.ID] = w
	}
	bookingID := newUUID()
	st.bookings[bookingID.String()] = &ServiceBooking{
		ID: bookingID.String(), BookerUserID: buyer.String(), ProviderUserID: seller.String(),
		TotalPrice: 1000, FeeCurrency: "usd", Status: "confirmed",
	}
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{
		strs: map[string]string{"stablecoin.escrow_arbiter_ref": ""},
	})
	if _, err := svc.PrepareEscrow(context.Background(), bookingID, buyer, ""); err == nil {
		t.Fatal("escrow prepared with no arbiter key — 2-of-3 could never arbitrate")
	}
}

func TestPrepareEscrowHidesSetupFromNonParty(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller, stranger := newUUID(), newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	// An existing escrow's setup (both parties' addresses + amounts) must
	// not be enumerable by a non-party who knows the booking id.
	if _, err := svc.PrepareEscrow(context.Background(), uuidOf(t, e.BookingID), stranger, "usdc"); !errors.Is(err, ErrEscrowParty) {
		t.Fatalf("stranger re-prepare = %v, want ErrEscrowParty", err)
	}
}

func TestWithdrawalFeeSkippedForDelegatedLegs(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	p := &fakeProvider{}
	sender := newUUID()
	w := &Wallet{
		ID: newUUID().String(), UserID: sender.String(), Status: WalletStatusActive,
		Chain: "solana", Locator: "loc",
		DelegatedSigner: &DelegatedSigner{SignerRef: "ref"},
	}
	st.walletByID[w.ID] = w
	member := &Wallet{ID: newUUID().String(), Status: WalletStatusActive, Address: "member-addr-9999"}
	st.walletByAddr[member.Address] = member
	st.treasury["solana:"+TreasuryHot] = &TreasuryWallet{Role: TreasuryHot, Chain: "solana", Address: "hot-addr"}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{ints: map[string]int{"stablecoin.withdrawal_fee_bps": 100}})
	ctx := context.Background()

	// Tip-settle recipient leg pays a member wallet — already fee'd in the
	// batched op; mirroring it must not collect the withdrawal fee again.
	if err := svc.MirrorActivity(ctx, ProviderEvent{
		Type: "transfer_out", TxSignature: "tip-leg-1", Asset: "usdc",
		AmountMinor: 10_000, CounterAddr: member.Address,
	}, w.ID); err != nil {
		t.Fatal(err)
	}
	// The fee leg itself pays the treasury — taxing it would recurse.
	if err := svc.MirrorActivity(ctx, ProviderEvent{
		Type: "transfer_out", TxSignature: "fee-leg-1", Asset: "usdc",
		AmountMinor: 500, CounterAddr: "hot-addr",
	}, w.ID); err != nil {
		t.Fatal(err)
	}
	if len(p.transfers) != 0 {
		t.Fatalf("delegated legs were double-charged: %+v", p.transfers)
	}
	// A genuinely external send still collects.
	if err := svc.MirrorActivity(ctx, ProviderEvent{
		Type: "transfer_out", TxSignature: "ext-wd-1", Asset: "usdc",
		AmountMinor: 10_000, CounterAddr: "external-addr",
	}, w.ID); err != nil {
		t.Fatal(err)
	}
	if len(p.transfers) != 1 {
		t.Fatalf("external withdrawal fee legs = %d, want 1", len(p.transfers))
	}
}

func TestRegisterEscrowVaultRejectsSwap(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowFunding
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	eid := uuidOf(t, e.ID)
	vaultA := "VaultAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	vaultB := "VaultBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	ctx := context.Background()

	if err := svc.RegisterEscrowVault(ctx, eid, buyer, vaultA); err != nil {
		t.Fatalf("initial registration: %v", err)
	}
	if err := svc.RegisterEscrowVault(ctx, eid, buyer, vaultA); err != nil {
		t.Fatalf("same-address re-registration must be idempotent: %v", err)
	}
	if err := svc.RegisterEscrowVault(ctx, eid, buyer, vaultB); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("vault swap = %v, want ErrEscrowState — the PDA encodes the member set", err)
	}
}

func TestConfirmEscrowFundedVerification(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowFunding
	e.VaultAddress = "VaultCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC"
	e.AmountMinor, e.FeeMinor = 8_500_000, 1_500_000
	eid := uuidOf(t, e.ID)
	p := &fakeProvider{activity: []ProviderEvent{
		{Type: "transfer_out", TxSignature: "good-sig", AmountMinor: 10_000_000, CounterAddr: e.VaultAddress},
		{Type: "transfer_out", TxSignature: "other-payee", AmountMinor: 10_000_000, CounterAddr: "someone-else"},
		{Type: "transfer_out", TxSignature: "short-pay", AmountMinor: 5_000_000, CounterAddr: e.VaultAddress},
	}}
	svc := newSvc(st, p, newFakeVK(), &fakeSettings{})
	ctx := context.Background()

	if err := svc.ConfirmEscrowFunded(ctx, eid, buyer, ""); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("empty funding sig = %v, want ErrEscrowState", err)
	}
	if err := svc.ConfirmEscrowFunded(ctx, eid, buyer, "other-payee"); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("sig paying a different address = %v, want ErrEscrowState", err)
	}
	if err := svc.ConfirmEscrowFunded(ctx, eid, buyer, "short-pay"); !errors.Is(err, ErrEscrowState) {
		t.Fatalf("under-funded sig = %v, want ErrEscrowState", err)
	}
	if err := svc.ConfirmEscrowFunded(ctx, eid, buyer, "good-sig"); err != nil {
		t.Fatalf("valid funding confirm: %v", err)
	}
	if e.State != EscrowFunded {
		t.Fatalf("state = %s, want funded", e.State)
	}
}

func TestMirrorPartialEscrowFundingStaysFunding(t *testing.T) {
	t.Parallel()
	st := newFakeStore()
	buyer, seller := newUUID(), newUUID()
	e := fundedEscrow(st, buyer.String(), seller.String())
	e.State = EscrowFunding
	e.VaultAddress = "VaultDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"
	e.AmountMinor, e.FeeMinor = 8_500_000, 1_500_000
	svc := newSvc(st, &fakeProvider{}, newFakeVK(), &fakeSettings{})
	ctx := context.Background()

	// A partial transfer into the vault must not flip the escrow funded —
	// the release would pay out short.
	if err := svc.MirrorActivity(ctx, ProviderEvent{
		Type: "escrow_funding", TxSignature: "partial-1", Asset: "usdc",
		AmountMinor: 5_000_000, CounterAddr: e.VaultAddress,
	}, ""); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowFunding {
		t.Fatalf("partial funding flipped state to %s", e.State)
	}
	if err := svc.MirrorActivity(ctx, ProviderEvent{
		Type: "escrow_funding", TxSignature: "full-1", Asset: "usdc",
		AmountMinor: 10_000_000, CounterAddr: e.VaultAddress,
	}, ""); err != nil {
		t.Fatal(err)
	}
	if e.State != EscrowFunded {
		t.Fatalf("full funding state = %s, want funded", e.State)
	}
}
