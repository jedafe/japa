package stablecoin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

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
// flags wallets whose delegated-signer scope expires inside the renew
// window (jc:stablecoin:signer_expiring:{user_id}). Rotation itself is a
// member ceremony (ActivateSigner) — a cron can never drive it because it
// cannot produce the member's recovery-approval proof. The wallet summary
// derives the same cue from scope expiry; the flag marks the sweep's
// sighting for ops/notification surfaces.
func (s *Service) ProcessSignerScopeRenew(ctx context.Context, raw []byte) error {
	var p SignerScopeRenewPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return fmt.Errorf("%s: unmarshal: %w", TaskSignerScopeRenew, err)
	}
	window := time.Duration(s.settings.GetIntOr(ctx, "stablecoin.signer_renew_window_days", 30)) * 24 * time.Hour
	cutoff := time.Now().Add(window)
	ttl := int(window.Seconds())
	if ttl < 3600 {
		ttl = 3600
	}
	mark := func(w *Wallet) error {
		if w == nil || !signerExpiringWithin(w, cutoff) {
			return nil
		}
		if err := s.vk.Set(ctx, "jc:stablecoin:signer_expiring:"+w.UserID, "1", ttl); err != nil {
			return err
		}
		slog.Info("stablecoin: delegated signer scope expiring — member rotation required",
			"wallet_id", w.ID, "user_id", w.UserID)
		return nil
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
		return mark(w)
	}
	active, err := s.listActiveWallets(ctx)
	if err != nil {
		return err
	}
	for i := range active {
		if err := mark(&active[i]); err != nil {
			slog.Warn("stablecoin: signer-expiry mark failed", "wallet_id", active[i].ID, "error", err)
		}
	}
	return nil
}

// signerExpiringWithin reports whether the wallet's nearest delegated-signer
// scope expires before the cutoff — the sweep's rotation eligibility check.
func signerExpiringWithin(w *Wallet, cutoff time.Time) bool {
	if w.DelegatedSigner == nil || len(w.DelegatedSigner.Scopes) == 0 {
		return false // nothing registered — provisioning retries registration
	}
	nearest := time.Time{}
	for _, sc := range w.DelegatedSigner.Scopes {
		if nearest.IsZero() || sc.ExpiresAt.Before(nearest) {
			nearest = sc.ExpiresAt
		}
	}
	return !nearest.IsZero() && nearest.Before(cutoff)
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
		// Page provider activity (bounded) — a busy wallet can push a
		// pending signature off page 1.
		var events []ProviderEvent
		cursor := ""
		failed := false
		for page := 0; page < 5; page++ {
			evs, next, err := s.provider.Activity(ctx, w.Locator, cursor)
			if err != nil {
				slog.Warn("stablecoin: offramp poll failed", "wallet", w.Locator, "error", err)
				failed = true
				break
			}
			events = append(events, evs...)
			if next == "" || len(evs) == 0 {
				break
			}
			cursor = next
		}
		if failed {
			continue
		}
		// Terminal event per signature — a failed onchain event must fail
		// the row, not confirm it. Terminal states are sticky in both
		// directions: a provider page shouldn't let ordering noise regress
		// a decided row.
		landed := map[string]ProviderEvent{}
		terminal := map[string]string{}
		for _, ev := range events {
			switch ev.Type {
			case "failed":
				if terminal[ev.TxSignature] != "confirmed" {
					terminal[ev.TxSignature] = "failed"
					landed[ev.TxSignature] = ev
				}
			case "pending", "submitted":
				// still in-flight — leave the row pending
			default:
				if terminal[ev.TxSignature] != "failed" {
					terminal[ev.TxSignature] = "confirmed"
					landed[ev.TxSignature] = ev
				}
			}
		}
		for _, a := range rows {
			var id pgtype.UUID
			if err := id.Scan(a.ID); err != nil {
				continue
			}
			ev, ok := landed[a.TxSignature]
			if !ok {
				continue
			}
			// The confirm path writes the event's canonical kind/direction/
			// counterparty (the pending row is a placeholder) and fires the
			// same postConfirmHooks as the live webhook — withdrawal-fee
			// collection and escrow transitions included.
			s.confirmActivity(ctx, id, ev, walletIDStr)
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
	// Method gate: only onchain-method tips settle through this task — a
	// credits-method tip settles in the §28 ledger synchronously at create
	// time and must never double-settle here.
	if tip.Method != "" && tip.Method != TipMethodStablecoin {
		return nil
	}
	if tip.Status != "pending" && tip.Status != "settling" && tip.Status != "failed" {
		return nil // already settled/cancelled — idempotent replay
	}
	// Resolve the net-leg destination BEFORE claiming. Member recipients
	// settle member→member onchain — unprovisioned → auto-provision: the
	// wallet is counterfactual (deploys on first tx, costs nothing
	// dormant) and receiving needs no signer — same rule as
	// SettleTipBacked's fiat path. A permanently-unprovisionable recipient
	// must leave the tip 'pending' so the TTL sweep fails it and the
	// sender isn't charged for nothing. Platform donations have no member
	// wallet — the net leg lands on the treasury hot wallet (resolved on
	// the sender's chain below, matching the fee leg's own lookup).
	var toAddr string
	if tip.RecipientType != "platform" {
		rw, err := s.store.GetWalletByUser(ctx, tip.RecipientUserID)
		if err != nil {
			if !errors.Is(err, ErrWalletNotFound) {
				return err
			}
			if rw, err = s.ProvisionWallet(ctx, tip.RecipientUserID, ""); err != nil {
				return fmt.Errorf("%s: recipient provision: %w", TaskTipSettleStablecoin, err)
			}
		}
		if rw == nil {
			return fmt.Errorf("%s: recipient wallet not provisioned", TaskTipSettleStablecoin)
		}
		if rw.Status == WalletStatusSuspended {
			return fmt.Errorf("%s: %w", TaskTipSettleStablecoin, ErrWalletSuspended)
		}
		toAddr = rw.Address
	}
	// Sender wallet pre-check BEFORE claiming: an unsettleable sender
	// (unprovisioned, suspended, signer never activated) can never complete
	// the delegated transfer. Failing here leaves the tip 'pending' — the
	// TTL sweep closes it — instead of deadlettering the task mid-claim
	// with the tip stuck in 'settling', which the pending-only sweep never
	// reclaims. The member can also fix the wallet between retries.
	sw, err := s.store.GetWalletByUser(ctx, tip.SenderUserID)
	if err != nil {
		if !errors.Is(err, ErrWalletNotFound) {
			return err
		}
		return fmt.Errorf("%s: sender wallet not provisioned", TaskTipSettleStablecoin)
	}
	if sw == nil || sw.Status != WalletStatusActive || sw.DelegatedSigner == nil {
		if sw != nil && sw.Status == WalletStatusSuspended {
			return fmt.Errorf("%s: %w", TaskTipSettleStablecoin, ErrWalletSuspended)
		}
		return fmt.Errorf("%s: %w", TaskTipSettleStablecoin, ErrWalletNotSettleable)
	}
	if tip.RecipientType == "platform" {
		hot, err := s.store.GetTreasuryWallet(ctx, sw.Chain, TreasuryHot)
		if err != nil {
			return fmt.Errorf("%s: treasury hot wallet: %w", TaskTipSettleStablecoin, err)
		}
		if hot == nil || hot.Address == "" {
			return fmt.Errorf("%s: treasury hot wallet has no address", TaskTipSettleStablecoin)
		}
		toAddr = hot.Address
	}
	// Claim last, immediately before the send: pending/failed → 'settling'
	// atomically excludes racing refunds/sweeps (they only touch 'pending')
	// while the onchain legs run outside any DB tx.
	claimed, err := s.tipReader.ClaimTipSettle(ctx, tipID)
	if err != nil {
		return fmt.Errorf("%s: claim tip: %w", TaskTipSettleStablecoin, err)
	}
	if !claimed {
		return nil // terminal state reached between read and claim
	}
	// Settle from the sender's wallet: net credits → recipient, fee leg →
	// treasury hot. Settle enforces killswitch + per-wallet daily cap.
	asset := "usdc"
	if assets := s.assetList(ctx); len(assets) > 0 {
		asset = assets[0]
	}
	// Tip legs are credits-denominated; the onchain transfer is USD-token
	// minor. Settlement math fails closed on a missing rate — the
	// creditsPerUSD fallback is display-only per its own contract.
	rate := s.creditsPerUnit(ctx, "usd")
	if rate <= 0 {
		return fmt.Errorf("%s: USD credits rate missing — cannot price settle", TaskTipSettleStablecoin)
	}
	toMinor := func(credits int64) int64 {
		return int64(math.Round(float64(credits) * 1_000_000 / rate))
	}
	res, err := s.Settle(ctx, SettleInput{
		FromUserID:  tip.SenderUserID,
		ToAddress:   toAddr,
		Asset:       asset,
		AmountMinor: toMinor(tip.NetCredits),
		FeeMinor:    toMinor(tip.FeeAmount),
		Idempotency: "tip:" + tip.ID.String(),
		Reference:   "tip",
	})
	if err != nil {
		// Task retries — the idempotency key dedupes the resend. A tip that
		// deadletters stays 'settling' for ops: the stale sweep is
		// pending-only by design, since a claimed tip may have a send in
		// flight and must never be auto-refunded.
		return err
	}
	// Lifecycle: the onchain op is submitted — close the tip out so it isn't
	// swept as stale-pending or double-settled on replay.
	if err := s.tipReader.MarkSettled(ctx, tipID, res.TxSignature); err != nil {
		return fmt.Errorf("%s: tip settled onchain but status update failed (sig %s): %w",
			TaskTipSettleStablecoin, res.TxSignature, err)
	}
	return nil
}
