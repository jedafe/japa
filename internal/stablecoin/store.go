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

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	gen "github.com/jedafe/japa/internal/db/gen"
)

// ErrWalletNotFound is returned when a member has no provisioned wallet.
var ErrWalletNotFound = errors.New("stablecoin: wallet not found")

// ErrActivityNotFound is returned for unknown tx signatures.
var ErrActivityNotFound = errors.New("stablecoin: activity not found")

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

// InsertActivity mirrors one onchain event. tx_signature UNIQUE +
// ON CONFLICT DO NOTHING makes provider webhook replays idempotent —
// a replay returns the zero row, callers treat it as a dedupe hit.
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
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: insert activity: %w", err)
	}
	return domainActivity(row), nil
}

// GetActivityBySig finds a mirrored event by tx signature.
func (st *Store) GetActivityBySig(ctx context.Context, sig string) (*Activity, error) {
	row, err := st.q.GetStablecoinActivityByTxSignature(ctx, &sig)
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
	if err != nil {
		return nil, fmt.Errorf("stablecoin: get escrow: %w", err)
	}
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
	return out, nil
}

// ListTreasuryWallets returns the platform treasury registry (hot/cold/revenue).
func (st *Store) ListTreasuryWallets(ctx context.Context) ([]TreasuryWallet, error) {
	rows, err := st.q.ListTreasuryWallets(ctx)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: list treasury: %w", err)
	}
	out := make([]TreasuryWallet, 0, len(rows))
	for _, r := range rows {
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
		out = append(out, t)
	}
	return out, nil
}
