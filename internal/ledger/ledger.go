// Package ledger routes wallet ledger reads and writes to the correct
// physical table: wallet_ledger holds the points stream, credits_ledger
// holds the monetary stream (Section 28, ADR-136,
// conventions/wallet-ledger-split-plan.md).
//
// Ledger embeds *gen.Queries and shadows only the ledger-touching
// methods, so call sites swap the constructor (gen.New(tx) ->
// ledger.New(gen.New(tx))) and every other query promotes through
// unchanged. All application ledger writes MUST go through this type —
// it is the single seam the split plan's write-path census relies on.
package ledger

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Ledger stream names — kept in sync with wallet.LedgerType* (which remain
// the canonical constants callers use in params and payloads).
const (
	TypePoints  = "points"
	TypeCredits = "credits"
)

// Ledger dispatches wallet-ledger queries by ledger_type.
type Ledger struct {
	*gen.Queries
}

// New wraps a gen.Queries (pooled or transaction-bound) in the dispatcher.
func New(q *gen.Queries) *Ledger {
	return &Ledger{Queries: q}
}

// walletRow normalizes a credits_ledger row into the shared WalletLedger
// shape callers already consume (ledger_type filled with 'credits').
func walletRow(c gen.CreditsLedger) gen.WalletLedger {
	return gen.WalletLedger{
		ID:               c.ID,
		UserID:           c.UserID,
		LedgerType:       TypeCredits,
		Operation:        c.Operation,
		Amount:           c.Amount,
		ResultingBalance: c.ResultingBalance,
		Source:           c.Source,
		ActorUserID:      c.ActorUserID,
		Reference:        c.Reference,
		IdempotencyKey:   c.IdempotencyKey,
		Reason:           c.Reason,
		CreatedAt:        c.CreatedAt,
	}
}

func walletRowFromMerged(m gen.GetMergedLedgerEntryByIDRow) gen.WalletLedger {
	return gen.WalletLedger{
		ID:               m.ID,
		UserID:           m.UserID,
		LedgerType:       m.LedgerType,
		Operation:        m.Operation,
		Amount:           m.Amount,
		ResultingBalance: m.ResultingBalance,
		Source:           m.Source,
		ActorUserID:      m.ActorUserID,
		Reference:        m.Reference,
		IdempotencyKey:   m.IdempotencyKey,
		Reason:           m.Reason,
		CreatedAt:        m.CreatedAt,
	}
}

func walletRowFromList(m gen.ListMergedLedgerEntriesByUserRow) gen.WalletLedger {
	return gen.WalletLedger{
		ID:               m.ID,
		UserID:           m.UserID,
		LedgerType:       m.LedgerType,
		Operation:        m.Operation,
		Amount:           m.Amount,
		ResultingBalance: m.ResultingBalance,
		Source:           m.Source,
		ActorUserID:      m.ActorUserID,
		Reference:        m.Reference,
		IdempotencyKey:   m.IdempotencyKey,
		Reason:           m.Reason,
		CreatedAt:        m.CreatedAt,
	}
}

// InsertLedgerEntry routes on LedgerType: credits rows go to
// credits_ledger, everything else to wallet_ledger. The return value is
// always normalized to the shared WalletLedger shape.
func (l *Ledger) InsertLedgerEntry(ctx context.Context, p gen.InsertLedgerEntryParams) (gen.WalletLedger, error) {
	if p.LedgerType == TypeCredits {
		row, err := l.Queries.InsertCreditLedgerEntry(ctx, gen.InsertCreditLedgerEntryParams{
			UserID:           p.UserID,
			Operation:        p.Operation,
			Amount:           p.Amount,
			ResultingBalance: p.ResultingBalance,
			Source:           p.Source,
			ActorUserID:      p.ActorUserID,
			Reference:        p.Reference,
			IdempotencyKey:   p.IdempotencyKey,
			Reason:           p.Reason,
		})
		if err != nil {
			return gen.WalletLedger{}, err
		}
		return walletRow(row), nil
	}

	return l.Queries.InsertLedgerEntry(ctx, p)
}

// InsertCreditPurchaseEntry is the credits purchase path — the only writer
// that can supply the payment_id linkage refunds depend on. Kept separate
// from InsertLedgerEntry so the shared params shape stays unchanged.
func (l *Ledger) InsertCreditPurchaseEntry(ctx context.Context, p gen.InsertLedgerEntryParams, paymentID pgtype.UUID) (gen.WalletLedger, error) {
	if p.LedgerType != TypeCredits {
		return l.InsertLedgerEntry(ctx, p)
	}
	row, err := l.Queries.InsertCreditLedgerEntry(ctx, gen.InsertCreditLedgerEntryParams{
		UserID:           p.UserID,
		Operation:        p.Operation,
		Amount:           p.Amount,
		ResultingBalance: p.ResultingBalance,
		Source:           p.Source,
		ActorUserID:      p.ActorUserID,
		Reference:        p.Reference,
		IdempotencyKey:   p.IdempotencyKey,
		Reason:           p.Reason,
		PaymentID:        paymentID,
	})
	if err != nil {
		return gen.WalletLedger{}, err
	}
	return walletRow(row), nil
}

// GetLatestBalance routes on LedgerType — credits balances come from
// credits_ledger, everything else from wallet_ledger.
func (l *Ledger) GetLatestBalance(ctx context.Context, p gen.GetLatestBalanceParams) (int64, error) {
	if p.LedgerType == TypeCredits {
		return l.Queries.GetLatestCreditBalance(ctx, p.UserID)
	}
	return l.Queries.GetLatestBalance(ctx, p)
}

// CheckIdempotencyKey preserves the pre-split global uniqueness semantics
// by checking both tables.
func (l *Ledger) CheckIdempotencyKey(ctx context.Context, idempotencyKey *string) (bool, error) {
	return l.Queries.CheckAnyLedgerIdempotencyKey(ctx, idempotencyKey)
}

// ListLedgerEntriesByUser serves the merged-history contract — a nil/empty
// ledger_type returns both streams interleaved, exactly like the old
// shared-table query.
func (l *Ledger) ListLedgerEntriesByUser(ctx context.Context, p gen.ListMergedLedgerEntriesByUserParams) ([]gen.WalletLedger, error) {
	rows, err := l.Queries.ListMergedLedgerEntriesByUser(ctx, gen.ListMergedLedgerEntriesByUserParams{
		UserID:     p.UserID,
		LedgerType: p.LedgerType,
		PageLimit:  p.PageLimit,
		PageOffset: p.PageOffset,
	})
	if err != nil {
		return nil, err
	}

	out := make([]gen.WalletLedger, 0, len(rows))
	for _, r := range rows {
		out = append(out, walletRowFromList(r))
	}
	return out, nil
}

// GetLedgerEntryByID resolves an entry id across both streams so admin
// tooling keeps the "found but wrong ledger" error semantics.
func (l *Ledger) GetLedgerEntryByID(ctx context.Context, id pgtype.UUID) (gen.WalletLedger, error) {
	row, err := l.Queries.GetMergedLedgerEntryByID(ctx, id)
	if err != nil {
		return gen.WalletLedger{}, err
	}
	return walletRowFromMerged(row), nil
}

// CountUserLedgerBySource routes on the stream being counted. Callers that
// only ever count points (one-time rules) are unaffected.
func (l *Ledger) CountUserLedgerBySource(ctx context.Context, p gen.CountUserLedgerBySourceParams, ledgerType string) (int64, error) {
	if ledgerType == TypeCredits {
		return l.Queries.CountUserCreditLedgerBySource(ctx, gen.CountUserCreditLedgerBySourceParams{
			UserID: p.UserID,
			Source: p.Source,
		})
	}
	return l.Queries.CountUserLedgerBySource(ctx, p)
}

// GetLedgerEntryByReference routes on the stream being searched — used by
// the ADR-136 compensation path (points) and by refund linkage (credits).
func (l *Ledger) GetLedgerEntryByReference(ctx context.Context, p gen.GetLedgerEntryByReferenceParams) (gen.WalletLedger, error) {
	if p.LedgerType == TypeCredits {
		row, err := l.Queries.GetCreditLedgerEntryByReference(ctx, gen.GetCreditLedgerEntryByReferenceParams{
			UserID:    p.UserID,
			Reference: p.Reference,
		})
		if err != nil {
			return gen.WalletLedger{}, err
		}
		return walletRow(row), nil
	}
	return l.Queries.GetLedgerEntryByReference(ctx, p)
}
