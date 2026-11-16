package stablecoin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
)

// Asynq task payloads — names registered in Section 11 (Rule 7). Handlers
// enqueue only; work happens in the worker (fire-and-forget, Rule 8).

// ProvisionWalletPayload carries stablecoin:provision_wallet.
type ProvisionWalletPayload struct {
	UserID     string `json:"user_id"`
	MemberAuth string `json:"member_auth"`
}

// SyncActivityPayload carries stablecoin:sync_activity.
type SyncActivityPayload struct {
	Event []byte `json:"event"` // raw provider event
}

// SignerScopeRenewPayload carries stablecoin:signer_scope_renew.
type SignerScopeRenewPayload struct {
	UserID string `json:"user_id"` // empty = sweep all active wallets
}

// TipSettlePayload carries tips:settle_stablecoin.
type TipSettlePayload struct {
	TipID string `json:"tip_id"`
}

// ProcessProvisionWallet handles stablecoin:provision_wallet.
func (s *Service) ProcessProvisionWallet(ctx context.Context, raw []byte) error {
	var p ProvisionWalletPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskProvisionWallet, err)
	}
	var uid pgtype.UUID
	if err := uid.Scan(p.UserID); err != nil {
		return fmt.Errorf("%s: user_id: %w", TaskProvisionWallet, err)
	}
	_, err := s.ProvisionWallet(ctx, uid, p.MemberAuth)
	return err
}

// ProcessSyncActivity handles stablecoin:sync_activity.
func (s *Service) ProcessSyncActivity(ctx context.Context, raw []byte) error {
	var p SyncActivityPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskSyncActivity, err)
	}
	return s.SyncActivity(ctx, p.Event)
}

// ProcessReconcile handles stablecoin:reconcile_wallets (cron).
func (s *Service) ProcessReconcile(ctx context.Context, _ []byte) error {
	return s.Reconcile(ctx)
}

// ProcessSignerScopeRenew handles stablecoin:signer_scope_renew (cron) —
// rotates delegated-signer scopes before expiresAt: scopes are immutable,
// so this registers a fresh scoped signer (member OTP approval) and
// removes the old one.
func (s *Service) ProcessSignerScopeRenew(ctx context.Context, raw []byte) error {
	var p SignerScopeRenewPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskSignerScopeRenew, err)
	}
	if p.UserID != "" {
		var uid pgtype.UUID
		if err := uid.Scan(p.UserID); err != nil {
			return fmt.Errorf("%s: user_id: %w", TaskSignerScopeRenew, err)
		}
		w, err := s.store.GetWalletByUser(ctx, uid)
		if err != nil {
			return err
		}
		return s.RotateSignerScopes(ctx, w)
	}
	// Sweep: rotate every active wallet whose nearest scope expires inside
	// the renew window — registration needs member OTP, so expiry is a
	// blast-radius control (§128.2).
	active, err := s.store.ListWalletsByStatus(ctx, WalletStatusActive, 500)
	if err != nil {
		return err
	}
	for i := range active {
		if err := s.RotateSignerScopes(ctx, &active[i]); err != nil {
			return fmt.Errorf("%s: rotate %s: %w", TaskSignerScopeRenew, active[i].ID, err)
		}
	}
	return nil
}

// ProcessEscrowTimeout handles stablecoin:escrow_timeout (cron).
func (s *Service) ProcessEscrowTimeout(ctx context.Context, _ []byte) error {
	return s.HandleEscrowTimeout(ctx)
}

// ProcessTreasurySweep handles stablecoin:treasury_sweep (cron).
func (s *Service) ProcessTreasurySweep(ctx context.Context, _ []byte) error {
	return s.ProposeSweep(ctx)
}

// ProcessOfframp handles stablecoin:process_offramp — settlement pass for
// pending mirror rows (offramp sends and any op whose webhook was missed):
// polls provider wallet activity, confirms rows whose tx_signature landed,
// and leaves unmatched rows pending for the reconcile sweep.
func (s *Service) ProcessOfframp(ctx context.Context, _ []byte) error {
	pending, err := s.store.ListPendingActivity(ctx, 200)
	if err != nil {
		return err
	}
	// Group by wallet — one provider poll per wallet.
	byWallet := map[string][]Activity{}
	for _, a := range pending {
		byWallet[a.WalletID] = append(byWallet[a.WalletID], a)
	}
	for walletIDStr, rows := range byWallet {
		var walletID pgtype.UUID
		if err := walletID.Scan(walletIDStr); err != nil {
			continue
		}
		w, err := s.store.GetWalletByID(ctx, walletID)
		if err != nil {
			continue
		}
		events, err := s.provider.Activity(ctx, w.Locator, "")
		if err != nil {
			slog.Warn("stablecoin: offramp poll failed", "wallet", w.Locator, "error", err)
			continue
		}
		landed := map[string]bool{}
		for _, ev := range events {
			landed[ev.TxSignature] = true
		}
		for _, a := range rows {
			var id pgtype.UUID
			if err := id.Scan(a.ID); err != nil {
				continue
			}
			if landed[a.TxSignature] {
				if err := s.store.SetActivityStatus(ctx, id, "confirmed"); err != nil {
					slog.Warn("stablecoin: confirm activity failed", "id", a.ID, "error", err)
				}
			}
		}
	}
	return nil
}

// ProcessTipSettle handles tips:settle_stablecoin — onchain tip settlement
// via delegated signer (tip−fee → recipient, fee → treasury hot).
func (s *Service) ProcessTipSettle(ctx context.Context, raw []byte) error {
	var p TipSettlePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskTipSettleStablecoin, err)
	}
	if s.tipReader == nil {
		return fmt.Errorf("%s: tip reader not wired", TaskTipSettleStablecoin)
	}
	var tipID pgtype.UUID
	if err := tipID.Scan(p.TipID); err != nil {
		return fmt.Errorf("%s: tip_id: %w", TaskTipSettleStablecoin, err)
	}
	tip, err := s.tipReader.GetTip(ctx, tipID)
	if err != nil {
		return err
	}
	if tip.Status != "pending" && tip.Status != "settling" {
		return nil // already settled/cancelled — idempotent replay
	}
	// Resolve the recipient's wallet — tips settle member→member onchain.
	rw, err := s.store.GetWalletByUser(ctx, tip.RecipientUserID)
	if err != nil {
		return err
	}
	// Settle from the sender's wallet: net credits → recipient, fee leg →
	// treasury hot. Settle enforces killswitch + per-wallet daily cap.
	asset := "usdc"
	if assets := s.assetList(ctx); len(assets) > 0 {
		asset = assets[0]
	}
	_, err = s.Settle(ctx, SettleInput{
		FromUserID:  tip.SenderUserID,
		ToAddress:   rw.Address,
		Asset:       asset,
		AmountMinor: tip.NetCredits,
		FeeMinor:    tip.FeeAmount,
		Idempotency: "tip:" + tip.ID.String(),
		Reference:   "tip",
	})
	return err
}
