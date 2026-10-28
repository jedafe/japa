package tips

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// settleWalletParams describes a completed points/credits tip for the atomic
// ledger-pair write.
type settleWalletParams struct {
	senderID        pgtype.UUID
	walletOwnerID   pgtype.UUID // user the wallet credits land on (recipient user, or org owner)
	recipientUserID pgtype.UUID // set when recipient_type='user'
	ledgerType      string      // 'points' | 'credits'
	gross           int64       // debited from sender
	net             int64       // credited to recipient (gross − fee)
	fee             int64
	method          string
	recipientType   string
	recipientOrgID  pgtype.UUID
	contentID       pgtype.UUID
	message         string
	isAnonymous     bool
	idempotencyKey  string
}

// lockWalletUsers acquires the per-user advisory locks in uuid order so two
// reciprocal tips (A→B and B→A) cannot interlock. Mirrors the AUDIT-BE-005
// wallet lock key.
func lockWalletUsers(ctx context.Context, tx pgx.Tx, ids ...pgtype.UUID) error {
	keys := make([]string, 0, len(ids))
	for _, id := range ids {
		if id.Valid {
			keys = append(keys, "wallet::"+id.String())
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", k); err != nil {
			return fmt.Errorf("acquiring wallet lock: %w", err)
		}
	}
	return nil
}

// latestBalance reads the current ledger balance inside a transaction.
func latestBalance(ctx context.Context, qtx *gen.Queries, userID pgtype.UUID, ledgerType string) (int64, error) {
	bal, err := qtx.GetLatestBalance(ctx, gen.GetLatestBalanceParams{UserID: userID, LedgerType: ledgerType})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return bal, nil
}

// settleWalletTip performs the §125 points/credits settlement: sender spend +
// recipient earn in one advisory-locked transaction, then the completed tip row.
func (s *Store) settleWalletTip(ctx context.Context, p settleWalletParams) (gen.Tip, error) {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return gen.Tip{}, fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.write.WithTx(tx)

	if err := lockWalletUsers(ctx, tx, p.senderID, p.walletOwnerID); err != nil {
		return gen.Tip{}, err
	}

	senderBal, err := latestBalance(ctx, qtx, p.senderID, p.ledgerType)
	if err != nil {
		return gen.Tip{}, fmt.Errorf("reading sender balance: %w", err)
	}
	if senderBal < p.gross {
		return gen.Tip{}, ErrInsufficientBalance
	}

	// Insert the tip first — its generated id becomes the ledger reference.
	tip, err := qtx.InsertTip(ctx, gen.InsertTipParams{
		SenderUserID:            p.senderID,
		RecipientType:           p.recipientType,
		RecipientUserID:         p.recipientUserID,
		RecipientOrganizationID: p.recipientOrgID,
		ContentID:               p.contentID,
		Method:                  p.method,
		PointsAmount:            pointsFor(p, "points"),
		CreditsAmount:           creditsFor(p),
		FeeAmount:               p.fee,
		NetCredits:              netCreditsFor(p),
		Message:                 strPtrOrNil(p.message),
		IsAnonymous:             p.isAnonymous,
		Status:                  StatusCompleted,
		IdempotencyKey:          strPtrOrNil(p.idempotencyKey),
	})
	if err != nil {
		return gen.Tip{}, fmt.Errorf("inserting tip: %w", err)
	}

	tipRef := tip.ID.String()
	source := LedgerSourceTip
	senderKey := "tip:s:" + tipRef

	if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
		UserID:           p.senderID,
		LedgerType:       p.ledgerType,
		Operation:        "spend",
		Amount:           -p.gross,
		ResultingBalance: senderBal - p.gross,
		Source:           &source,
		Reference:        &tipRef,
		IdempotencyKey:   &senderKey,
	}); err != nil {
		return gen.Tip{}, fmt.Errorf("inserting sender ledger entry: %w", err)
	}

	if p.net > 0 {
		recipientBal, err := latestBalance(ctx, qtx, p.walletOwnerID, p.ledgerType)
		if err != nil {
			return gen.Tip{}, fmt.Errorf("reading recipient balance: %w", err)
		}
		recipientKey := "tip:r:" + tipRef
		if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			UserID:           p.walletOwnerID,
			LedgerType:       p.ledgerType,
			Operation:        "earn",
			Amount:           p.net,
			ResultingBalance: recipientBal + p.net,
			Source:           &source,
			Reference:        &tipRef,
			IdempotencyKey:   &recipientKey,
		}); err != nil {
			return gen.Tip{}, fmt.Errorf("inserting recipient ledger entry: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return gen.Tip{}, fmt.Errorf("committing settlement: %w", err)
	}
	return tip, nil
}

func pointsFor(p settleWalletParams, ledger string) int64 {
	if p.ledgerType == ledger {
		return p.gross
	}
	return 0
}

func creditsFor(p settleWalletParams) int64 {
	if p.ledgerType == "credits" {
		return p.gross
	}
	return 0
}

func netCreditsFor(p settleWalletParams) int64 {
	if p.ledgerType == "credits" {
		return p.net
	}
	return 0
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// settleFiatTip credits the recipient's credits wallet with netCredits and
// marks the tip completed. Idempotent — a completed tip returns immediately.
func (s *Store) settleFiatTip(ctx context.Context, tipID pgtype.UUID, netCredits int64, walletOwnerID pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.write.WithTx(tx)

	tip, err := qtx.GetTip(ctx, tipID)
	if err != nil {
		return err
	}
	if tip.Status == StatusCompleted {
		return nil // already settled — idempotent
	}
	if tip.Status != StatusPending {
		return ErrInvalidStatus
	}

	if netCredits > 0 && walletOwnerID.Valid {
		if err := lockWalletUsers(ctx, tx, walletOwnerID); err != nil {
			return err
		}
		bal, err := latestBalance(ctx, qtx, walletOwnerID, "credits")
		if err != nil {
			return fmt.Errorf("reading recipient balance: %w", err)
		}
		source := LedgerSourceTip
		ref := tipID.String()
		key := "tip:fr:" + tipID.String()
		if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			UserID:           walletOwnerID,
			LedgerType:       "credits",
			Operation:        "earn",
			Amount:           netCredits,
			ResultingBalance: bal + netCredits,
			Source:           &source,
			Reference:        &ref,
			IdempotencyKey:   &key,
		}); err != nil {
			return fmt.Errorf("crediting recipient: %w", err)
		}
	}

	if err := qtx.SettleTip(ctx, gen.SettleTipParams{ID: tipID, NetCredits: netCredits}); err != nil {
		return fmt.Errorf("marking tip completed: %w", err)
	}

	return tx.Commit(ctx)
}

// clawbackTip debits the recipient's credits wallet and marks the tip
// refunded — the §125 refund path (payment refunded upstream).
func (s *Store) clawbackTip(ctx context.Context, tipID pgtype.UUID, walletOwnerID pgtype.UUID) error {
	tx, err := s.db.Writer.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.write.WithTx(tx)

	tip, err := qtx.GetTip(ctx, tipID)
	if err != nil {
		return err
	}
	if tip.Status == StatusRefunded {
		return nil // already clawed back
	}
	if tip.Status != StatusCompleted {
		return ErrInvalidStatus
	}

	if tip.NetCredits > 0 && walletOwnerID.Valid {
		if err := lockWalletUsers(ctx, tx, walletOwnerID); err != nil {
			return err
		}
		bal, err := latestBalance(ctx, qtx, walletOwnerID, "credits")
		if err != nil {
			return fmt.Errorf("reading recipient balance: %w", err)
		}
		source := LedgerSourceTip
		ref := tipID.String()
		key := "tip:cl:" + tipID.String()
		if _, err = qtx.InsertLedgerEntry(ctx, gen.InsertLedgerEntryParams{
			UserID:           walletOwnerID,
			LedgerType:       "credits",
			Operation:        "clawback",
			Amount:           -tip.NetCredits,
			ResultingBalance: bal - tip.NetCredits,
			Source:           &source,
			Reference:        &ref,
			IdempotencyKey:   &key,
		}); err != nil {
			return fmt.Errorf("clawing back recipient credits: %w", err)
		}
	}

	if err := qtx.UpdateTipStatus(ctx, gen.UpdateTipStatusParams{ID: tipID, Status: StatusRefunded}); err != nil {
		return fmt.Errorf("marking tip refunded: %w", err)
	}

	return tx.Commit(ctx)
}
