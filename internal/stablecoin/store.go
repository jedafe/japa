package stablecoin

// store.go — the §128 persistence seam over sqlc-generated queries
// (migration 001177). Maps gen rows to domain types; chain is truth for
// balances, these rows are render cache + orchestration state.
//
// Ref: japaconnect-engineering/sections/128-stablecoin-wallet-custody.md

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gen "github.com/jedafe/japa/internal/db/gen"
)

// ErrWalletNotFound is returned when a member has no provisioned wallet.
var ErrWalletNotFound = errors.New("stablecoin: wallet not found")

// ErrActivityNotFound is returned for unknown tx signatures.
var ErrActivityNotFound = errors.New("stablecoin: activity not found")

// ErrEscrowNotFound is returned for unknown escrows.
var ErrEscrowNotFound = errors.New("stablecoin: escrow not found")

// ErrBookingNotFound is returned for unknown service bookings.
var ErrBookingNotFound = errors.New("stablecoin: booking not found")

// ErrTreasuryNotFound is returned for unconfigured treasury roles.
var ErrTreasuryNotFound = errors.New("stablecoin: treasury wallet not found")

// ErrSweepNotFound is returned for unknown sweep proposals.
var ErrSweepNotFound = errors.New("stablecoin: sweep proposal not found")

// Store adapts *gen.Queries to the service's store seam.
type Store struct {
	q *gen.Queries
}

// NewStore wires the sqlc-backed store.
func NewStore(q *gen.Queries) *Store {
	return &Store{q: q}
}

// Compile-time check: Store satisfies the service's store seam.
var _ store = (*Store)(nil)

func pgUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if s == "" {
		return u, errors.New("stablecoin: empty uuid")
	}
	return u, u.Scan(s)
}

func pgUUIDPtr(s *string) pgtype.UUID {
	var u pgtype.UUID
	if s != nil {
		_ = u.Scan(*s)
	}
	return u
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b, _ := u.Value()
	s, _ := b.(string)
	return s
}

func domainWallet(w gen.StablecoinWallet) *Wallet {
	out := &Wallet{
		ID:        uuidStr(w.ID),
		UserID:    uuidStr(w.UserID),
		Chain:     w.Chain,
		Address:   w.Address,
		Locator:   w.Locator,
		Status:    w.Status,
		CreatedAt: w.CreatedAt.Time,
		UpdatedAt: w.UpdatedAt.Time,
	}
	if len(w.RecoveryMethods) > 0 {
		_ = json.Unmarshal(w.RecoveryMethods, &out.RecoveryMethods)
	}
	if len(w.DelegatedSigner) > 0 {
		var ds DelegatedSigner
		if json.Unmarshal(w.DelegatedSigner, &ds) == nil {
			out.DelegatedSigner = &ds
		}
	}
	return out
}

// GetWalletByUser returns the member's wallet or ErrWalletNotFound.
func (st *Store) GetWalletByUser(ctx context.Context, userID pgtype.UUID) (*Wallet, error) {
	w, err := st.q.GetStablecoinWalletByUser(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get wallet: %w", err)
	}
	return domainWallet(w), nil
}

// InsertWallet persists a freshly provisioned member wallet.
func (st *Store) InsertWallet(ctx context.Context, w Wallet) (*Wallet, error) {
	uid, err := pgUUID(w.UserID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert wallet: %w", err)
	}
	rm, _ := json.Marshal(w.RecoveryMethods)
	row, err := st.q.InsertStablecoinWallet(ctx, gen.InsertStablecoinWalletParams{
		UserID:          uid,
		Chain:           w.Chain,
		Address:         w.Address,
		Locator:         w.Locator,
		RecoveryMethods: rm,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert wallet: %w", err)
	}
	return domainWallet(row), nil
}

// UpdateWalletStatus moves the wallet through its state machine.
func (st *Store) UpdateWalletStatus(ctx context.Context, id pgtype.UUID, status string) error {
	_, err := st.q.SetStablecoinWalletStatus(ctx, gen.SetStablecoinWalletStatusParams{ID: id, Status: status})
	return err
}

// InsertActivity mirrors one onchain event. (wallet_id, tx_signature)
// UNIQUE + ON CONFLICT DO NOTHING makes provider webhook replays
// idempotent — a replay returns the zero row, callers treat it as a
// dedupe hit. The unique is per-wallet because one member→member tx
// legitimately mirrors on both wallets under the same signature.
func (st *Store) InsertActivity(ctx context.Context, a Activity) (*Activity, error) {
	wid, err := pgUUID(a.WalletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert activity: %w", err)
	}
	var sig *string
	if a.TxSignature != "" {
		sig = &a.TxSignature
	}
	var slot *int64
	if a.Slot != 0 {
		slot = &a.Slot
	}
	row, err := st.q.InsertStablecoinActivity(ctx, gen.InsertStablecoinActivityParams{
		WalletID:           wid,
		Direction:          a.Direction,
		Kind:               a.Kind,
		Asset:              a.Asset,
		AmountMinor:        a.AmountMinor,
		CreditsAmount:      a.CreditsAmount,
		CounterpartyUserID: pgUUIDPtr(a.CounterpartyUserID),
		TxSignature:        sig,
		Slot:               slot,
		Status:             a.Status,
		OccurredAt:         pgtype.Timestamptz{Time: a.OccurredAt, Valid: !a.OccurredAt.IsZero()},
		Raw:                a.Raw,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING returns no row on a tx_signature replay —
		// idempotent dedupe hit, not a failure (webhook replays, task retries).
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert activity: %w", err)
	}
	return domainActivity(row), nil
}

// GetActivityBySig finds a wallet's mirrored event by tx signature —
// per-wallet because one member→member tx shares its signature across
// both wallets' rows.
func (st *Store) GetActivityBySig(ctx context.Context, walletID, sig string) (*Activity, error) {
	wid, err := pgUUID(walletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get activity: %w", err)
	}
	row, err := st.q.GetStablecoinActivityByTxSignature(ctx, gen.GetStablecoinActivityByTxSignatureParams{
		WalletID:    wid,
		TxSignature: &sig,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrActivityNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get activity: %w", err)
	}
	return domainActivity(row), nil
}

func domainActivity(a gen.StablecoinActivity) *Activity {
	out := &Activity{
		ID:            uuidStr(a.ID),
		WalletID:      uuidStr(a.WalletID),
		Direction:     a.Direction,
		Kind:          a.Kind,
		Asset:         a.Asset,
		AmountMinor:   a.AmountMinor,
		CreditsAmount: a.CreditsAmount,
		Status:        a.Status,
		OccurredAt:    a.OccurredAt.Time,
		Raw:           a.Raw,
	}
	if a.CounterpartyUserID.Valid {
		s := uuidStr(a.CounterpartyUserID)
		out.CounterpartyUserID = &s
	}
	if a.TxSignature != nil {
		out.TxSignature = *a.TxSignature
	}
	if a.Slot != nil {
		out.Slot = *a.Slot
	}
	return out
}

// GetEscrow returns one service-escrow vault by id.
func (st *Store) GetEscrow(ctx context.Context, id pgtype.UUID) (*Escrow, error) {
	row, err := st.q.GetServiceEscrow(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEscrowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get escrow: %w", err)
	}
	return domainEscrow(row), nil
}

// ListTreasuryWallets returns the platform treasury registry (hot/cold/revenue).
func (st *Store) ListTreasuryWallets(ctx context.Context) ([]TreasuryWallet, error) {
	rows, err := st.q.ListTreasuryWallets(ctx)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: list treasury: %w", err)
	}
	out := make([]TreasuryWallet, 0, len(rows))
	for _, r := range rows {
		out = append(out, domainTreasuryWallet(r))
	}
	return out, nil
}

func domainTreasuryWallet(r gen.TreasuryWallet) TreasuryWallet {
	t := TreasuryWallet{
		ID:      uuidStr(r.ID),
		Role:    r.Role,
		Chain:   r.Chain,
		Address: r.Address,
	}
	if r.Locator != nil {
		t.Locator = *r.Locator
	}
	if r.FloatCapMinor != nil {
		t.FloatCapMinor = *r.FloatCapMinor
	}
	return t
}

// GetWalletByAddress resolves a member wallet by its onchain address —
// webhook counterparty resolution + external senders.
func (st *Store) GetWalletByAddress(ctx context.Context, address string) (*Wallet, error) {
	w, err := st.q.GetStablecoinWalletByAddress(ctx, address)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWalletNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get wallet by address: %w", err)
	}
	return domainWallet(w), nil
}

// UpdateDelegatedSigner persists the delegated-signer registration state.
func (st *Store) UpdateDelegatedSigner(ctx context.Context, id pgtype.UUID, ds *DelegatedSigner) error {
	b, err := json.Marshal(ds)
	if err != nil {
		return fmt.Errorf("stablecoin: marshal signer: %w", err)
	}
	return st.q.SetStablecoinWalletDelegatedSigner(ctx, gen.SetStablecoinWalletDelegatedSignerParams{ID: id, DelegatedSigner: b})
}

// GetWalletByID resolves a wallet row by primary key — activity rows
// carry wallet_id, so the offramp sweep resolves wallets this way.
func (st *Store) GetWalletByID(ctx context.Context, id pgtype.UUID) (*Wallet, error) {
	row, err := st.q.GetStablecoinWalletByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}
	return domainWallet(row), nil
}

// ListPendingActivity feeds the offramp/webhook-gap sweep — pending rows
// oldest-first so a lagging confirmation catches up deterministically.
func (st *Store) ListPendingActivity(ctx context.Context, limit int32) ([]Activity, error) {
	rows, err := st.q.ListPendingStablecoinActivity(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainActivity(r))
	}
	return out, nil
}

// SumOutByAssetSince is the delegated-signer cap source — SQL aggregate
// over confirmed+pending out-rows (pending counts: parallel ops race the
// cap otherwise).
func (st *Store) SumOutByAssetSince(ctx context.Context, walletID pgtype.UUID, asset string, since time.Time) (int64, error) {
	return st.q.SumOutActivityByAssetSince(ctx, gen.SumOutActivityByAssetSinceParams{
		WalletID: walletID,
		Asset:    asset,
		Since:    pgtype.Timestamptz{Time: since, Valid: true},
	})
}

// SumBackedCredits nets confirmed in/out mirror rows — the cache-miss
// balance derivation (never page-capped like row iteration).
func (st *Store) SumBackedCredits(ctx context.Context, walletID pgtype.UUID) (int64, error) {
	return st.q.SumBackedCreditsByWallet(ctx, walletID)
}

// SumPendingInCredits totals unconfirmed inbound credits for the pending
// balance display.
func (st *Store) SumPendingInCredits(ctx context.Context, walletID pgtype.UUID) (int64, error) {
	return st.q.SumPendingInCreditsByWallet(ctx, walletID)
}

// ConfirmActivity writes a terminal status plus the confirming event's
// canonical fields onto a pending row — WHERE status='pending' guards the
// transition so concurrent terminal deliveries can't double-fire hooks.
// Returns true when the row actually transitioned.
func (st *Store) ConfirmActivity(ctx context.Context, id pgtype.UUID, a Activity) (bool, error) {
	var sig *string
	if a.TxSignature != "" {
		sig = &a.TxSignature
	}
	var slot *int64
	if a.Slot != 0 {
		slot = &a.Slot
	}
	n, err := st.q.ConfirmStablecoinActivity(ctx, gen.ConfirmStablecoinActivityParams{
		ID:                 id,
		Status:             a.Status,
		Kind:               a.Kind,
		Direction:          a.Direction,
		Asset:              a.Asset,
		AmountMinor:        a.AmountMinor,
		CreditsAmount:      a.CreditsAmount,
		CounterpartyUserID: pgUUIDPtr(a.CounterpartyUserID),
		TxSignature:        sig,
		Slot:               slot,
		OccurredAt:         pgtype.Timestamptz{Time: a.OccurredAt, Valid: !a.OccurredAt.IsZero()},
		Raw:                a.Raw,
	})
	if err != nil {
		return false, fmt.Errorf("stablecoin: confirm activity: %w", err)
	}
	return n > 0, nil
}

// ListWallets pages the admin registry — status "" = all, q = address
// substring search.
func (st *Store) ListWallets(ctx context.Context, status, q string, limit, offset int32) ([]Wallet, error) {
	var qp interface{}
	if q != "" {
		qp = q
	}
	rows, err := st.q.ListStablecoinWallets(ctx, gen.ListStablecoinWalletsParams{
		Status:      status,
		Q:           qp,
		OffsetCount: offset,
		LimitCount:  limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Wallet, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainWallet(r))
	}
	return out, nil
}

// ListWalletsByStatus feeds the reconcile sweep — active wallets only.
func (st *Store) ListWalletsByStatus(ctx context.Context, status string, limit, offset int32) ([]Wallet, error) {
	rows, err := st.q.ListStablecoinWalletsByStatus(ctx, gen.ListStablecoinWalletsByStatusParams{
		Status:      status,
		LimitCount:  limit,
		OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: list wallets: %w", err)
	}
	out := make([]Wallet, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainWallet(r))
	}
	return out, nil
}

// ListActivityByWallet returns mirrored events newest-first.
func (st *Store) ListActivityByWallet(ctx context.Context, walletID pgtype.UUID, limit, offset int32) ([]Activity, error) {
	rows, err := st.q.ListStablecoinActivityByWallet(ctx, gen.ListStablecoinActivityByWalletParams{
		WalletID:    walletID,
		LimitCount:  limit,
		OffsetCount: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: list activity: %w", err)
	}
	out := make([]Activity, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainActivity(r))
	}
	return out, nil
}

// GetEscrowByBooking returns the latest escrow for a booking.
func (st *Store) GetEscrowByBooking(ctx context.Context, bookingID pgtype.UUID) (*Escrow, error) {
	row, err := st.q.GetServiceEscrowByBooking(ctx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEscrowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: escrow by booking: %w", err)
	}
	return domainEscrow(row), nil
}

// GetUnambiguousLiveEscrowByWallet returns the wallet's live escrow ONLY
// when exactly one exists — the mirror's fallback join must never pick
// between multiple live escrows or the wrong vault could transition.
func (st *Store) GetUnambiguousLiveEscrowByWallet(ctx context.Context, walletID pgtype.UUID) (*Escrow, error) {
	rows, err := st.q.ListLiveEscrowsByWallet(ctx, walletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: live escrows by wallet: %w", err)
	}
	if len(rows) != 1 {
		return nil, ErrEscrowNotFound
	}
	return domainEscrow(rows[0]), nil
}

// GetEscrowByVault returns the escrow whose Squads vault matches —
// the precise join for mirrored escrow_funding/escrow_release events
// (the event counterparty IS the vault).
func (st *Store) GetEscrowByVault(ctx context.Context, vaultAddr string) (*Escrow, error) {
	row, err := st.q.GetEscrowByVault(ctx, vaultAddr)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrEscrowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: escrow by vault: %w", err)
	}
	return domainEscrow(row), nil
}

// InsertEscrow persists a new escrow vault record (state=funding).
func (st *Store) InsertEscrow(ctx context.Context, e Escrow) (*Escrow, error) {
	booking, err := pgUUID(e.BookingID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert escrow booking: %w", err)
	}
	buyer, err := pgUUID(e.BuyerWalletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert escrow buyer: %w", err)
	}
	seller, err := pgUUID(e.SellerWalletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert escrow seller: %w", err)
	}
	var deadline pgtype.Timestamptz
	if !e.ConfirmationDeadline.IsZero() {
		deadline = pgtype.Timestamptz{Time: e.ConfirmationDeadline, Valid: true}
	}
	row, err := st.q.InsertServiceEscrow(ctx, gen.InsertServiceEscrowParams{
		BookingID:            booking,
		BuyerWalletID:        buyer,
		SellerWalletID:       seller,
		VaultAddress:         e.VaultAddress,
		Asset:                e.Asset,
		AmountMinor:          e.AmountMinor,
		FeeMinor:             e.FeeMinor,
		ConfirmationDeadline: deadline,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert escrow: %w", err)
	}
	return domainEscrow(row), nil
}

// SetEscrowState moves the escrow through its state machine. Nil params
// leave prior values intact (SQL COALESCE) — funded_sig and decisions
// persist across later transitions.
func (st *Store) SetEscrowState(ctx context.Context, id pgtype.UUID, state, fundedSig, releaseSig string, decision *ArbiterDecision, deadline *time.Time) error {
	var fs, rs *string
	if fundedSig != "" {
		fs = &fundedSig
	}
	if releaseSig != "" {
		rs = &releaseSig
	}
	var dec []byte
	if decision != nil {
		var err error
		if dec, err = json.Marshal(decision); err != nil {
			return fmt.Errorf("stablecoin: marshal arbiter decision: %w", err)
		}
	}
	var dl pgtype.Timestamptz
	if deadline != nil {
		dl = pgtype.Timestamptz{Time: *deadline, Valid: true}
	}
	_, err := st.q.SetServiceEscrowState(ctx, gen.SetServiceEscrowStateParams{
		ID:                   id,
		State:                state,
		FundedSig:            fs,
		ReleaseSig:           rs,
		ArbiterDecision:      dec,
		ConfirmationDeadline: dl,
	})
	return err
}

// SetEscrowVaultAddress registers the client-derived Squads PDA.
func (st *Store) SetEscrowVaultAddress(ctx context.Context, id pgtype.UUID, addr string) error {
	return st.q.SetServiceEscrowVaultAddress(ctx, gen.SetServiceEscrowVaultAddressParams{ID: id, VaultAddress: addr})
}

// ListExpiredFundedEscrows feeds the buyer-ghost timeout sweep.
func (st *Store) ListExpiredFundedEscrows(ctx context.Context, limit int32) ([]Escrow, error) {
	rows, err := st.q.ListExpiredFundedEscrows(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: expired escrows: %w", err)
	}
	out := make([]Escrow, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainEscrow(r))
	}
	return out, nil
}

// GetServiceBooking returns the escrow-relevant slice of a §111 booking.
func (st *Store) GetServiceBooking(ctx context.Context, id pgtype.UUID) (*ServiceBooking, error) {
	b, err := st.q.GetServiceBooking(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: booking: %w", err)
	}
	out := &ServiceBooking{
		ID:             uuidStr(b.ID),
		BookerUserID:   uuidStr(b.BookerUserID),
		ProviderUserID: uuidStr(b.ProviderUserID),
		Status:         b.Status,
	}
	if b.TotalPrice != nil {
		out.TotalPrice = *b.TotalPrice
	}
	if b.FeeAmount != nil {
		out.FeeAmount = *b.FeeAmount
	}
	if b.FeeCurrency != nil {
		out.FeeCurrency = *b.FeeCurrency
	}
	return out, nil
}

// GetTreasuryWalletByAddress resolves a treasury wallet by chain+address —
// the mirror's lookup when a provider event matches no member wallet.
func (st *Store) GetTreasuryWalletByAddress(ctx context.Context, chain, address string) (*TreasuryWallet, error) {
	r, err := st.q.GetTreasuryWalletByAddress(ctx, gen.GetTreasuryWalletByAddressParams{Chain: chain, Address: address})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTreasuryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: treasury by address: %w", err)
	}
	t := domainTreasuryWallet(r)
	return &t, nil
}

// GetApprovedSweepForTransfer matches a mirrored hot→cold out-transfer to
// the open approved sweep it executes.
func (st *Store) GetApprovedSweepForTransfer(ctx context.Context, chain, asset, hotWalletID string, amountMinor int64) (*TreasurySweep, error) {
	hot, err := pgUUID(hotWalletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: sweep hot wallet: %w", err)
	}
	r, err := st.q.GetApprovedSweepForTransfer(ctx, gen.GetApprovedSweepForTransferParams{
		Chain:       chain,
		Asset:       asset,
		HotWalletID: hot,
		AmountMinor: amountMinor,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSweepNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: approved sweep: %w", err)
	}
	return domainSweep(r), nil
}

// MarkSweepExecuted records the executed Squads signature on an approved
// sweep proposal — the mirror's completion write for treasury flow.
func (st *Store) MarkSweepExecuted(ctx context.Context, id pgtype.UUID, sig string) error {
	return st.q.MarkTreasurySweepExecuted(ctx, gen.MarkTreasurySweepExecutedParams{ID: id, ExecutedSig: &sig})
}

// GetTreasuryWallet returns one treasury wallet by chain+role.
func (st *Store) GetTreasuryWallet(ctx context.Context, chain, role string) (*TreasuryWallet, error) {
	r, err := st.q.GetTreasuryWallet(ctx, gen.GetTreasuryWalletParams{Chain: chain, Role: role})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrTreasuryNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: treasury wallet: %w", err)
	}
	t := domainTreasuryWallet(r)
	return &t, nil
}

// InsertTreasurySweep persists a hot→cold sweep proposal.
func (st *Store) InsertTreasurySweep(ctx context.Context, sw TreasurySweep) (*TreasurySweep, error) {
	hot, err := pgUUID(sw.HotWalletID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: sweep hot wallet: %w", err)
	}
	row, err := st.q.InsertTreasurySweep(ctx, gen.InsertTreasurySweepParams{
		Chain:       sw.Chain,
		Asset:       sw.Asset,
		AmountMinor: sw.AmountMinor,
		HotWalletID: hot,
		ColdAddress: sw.ColdAddress,
		ProposedBy:  sw.ProposedBy,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert sweep: %w", err)
	}
	return domainSweep(row), nil
}

// GetTreasurySweep returns one proposal.
func (st *Store) GetTreasurySweep(ctx context.Context, id pgtype.UUID) (*TreasurySweep, error) {
	row, err := st.q.GetTreasurySweep(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSweepNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get sweep: %w", err)
	}
	return domainSweep(row), nil
}

// ListSweepsByState pages proposals by state for the admin surface.
func (st *Store) ListSweepsByState(ctx context.Context, state string, limit, offset int32) ([]TreasurySweep, error) {
	rows, err := st.q.ListTreasurySweepsByState(ctx, gen.ListTreasurySweepsByStateParams{
		State:       state,
		OffsetCount: offset,
		LimitCount:  limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]TreasurySweep, 0, len(rows))
	for _, r := range rows {
		out = append(out, *domainSweep(r))
	}
	return out, nil
}

// ExpireStaleSweeps flips open proposals past the expiry window to
// 'expired' — a stalled Squads ceremony must not block future sweeps for
// that asset. Returns the expired count for logging.
func (st *Store) ExpireStaleSweeps(ctx context.Context, cutoff time.Time) (int64, error) {
	return st.q.ExpireStaleTreasurySweeps(ctx, pgtype.Timestamptz{Time: cutoff, Valid: true})
}

// DecideTreasurySweep records the approve/reject decision on a proposal.
func (st *Store) DecideTreasurySweep(ctx context.Context, id pgtype.UUID, state string, approvedBy pgtype.UUID, squadsProposalID string) error {
	var sp *string
	if squadsProposalID != "" {
		sp = &squadsProposalID
	}
	_, err := st.q.DecideTreasurySweep(ctx, gen.DecideTreasurySweepParams{
		ID:               id,
		State:            state,
		ApprovedBy:       approvedBy,
		SquadsProposalID: sp,
	})
	return err
}

func domainSweep(r gen.TreasurySweep) *TreasurySweep {
	out := &TreasurySweep{
		ID:          uuidStr(r.ID),
		Chain:       r.Chain,
		Asset:       r.Asset,
		AmountMinor: r.AmountMinor,
		HotWalletID: uuidStr(r.HotWalletID),
		ColdAddress: r.ColdAddress,
		State:       r.State,
		ProposedBy:  r.ProposedBy,
	}
	if r.ApprovedBy.Valid {
		out.ApprovedBy = uuidStr(r.ApprovedBy)
	}
	if r.SquadsProposalID != nil {
		out.SquadsProposalID = *r.SquadsProposalID
	}
	if r.ExecutedSig != nil {
		out.ExecutedSig = *r.ExecutedSig
	}
	return out
}

func domainEscrow(row gen.ServiceEscrow) *Escrow {
	out := &Escrow{
		ID:             uuidStr(row.ID),
		BookingID:      uuidStr(row.BookingID),
		BuyerWalletID:  uuidStr(row.BuyerWalletID),
		SellerWalletID: uuidStr(row.SellerWalletID),
		VaultAddress:   row.VaultAddress,
		Asset:          row.Asset,
		AmountMinor:    row.AmountMinor,
		FeeMinor:       row.FeeMinor,
		State:          row.State,
	}
	if row.FundedSig != nil {
		out.FundedSig = *row.FundedSig
	}
	if row.ReleaseSig != nil {
		out.ReleaseSig = *row.ReleaseSig
	}
	if row.ConfirmationDeadline.Valid {
		out.ConfirmationDeadline = row.ConfirmationDeadline.Time
	}
	if len(row.ArbiterDecision) > 0 {
		var d ArbiterDecision
		if json.Unmarshal(row.ArbiterDecision, &d) == nil {
			out.ArbiterDecision = &d
		}
	}
	return out
}
