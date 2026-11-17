package stablecoin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Treasury is platform money only — hot/cold split per §128.7/§128.11.
// The cold vault is a Squads multisig; the backend watches balances for
// reconciliation and never holds its keys.

// TreasuryView is the admin treasury surface (GET /admin/treasury/wallets).
// KillswitchEngaged rides along so the admin panel can render the halt
// state without a second route — same surface, same permission gate.
type TreasuryView struct {
	Wallets           []TreasuryWallet `json:"wallets"`
	KillswitchEngaged bool             `json:"killswitch_engaged"`
}

// ListTreasury returns the treasury registry for the admin surface.
func (s *Service) ListTreasury(ctx context.Context) (*TreasuryView, error) {
	wallets, err := s.store.ListTreasuryWallets(ctx)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: treasury: %w", err)
	}
	engaged, kerr := s.killswitch(ctx)
	if kerr != nil {
		slog.Warn("stablecoin: killswitch read failed", "error", kerr)
	}
	return &TreasuryView{Wallets: wallets, KillswitchEngaged: engaged}, nil
}

// SettleTipBacked settles a fiat-denominated tip to the recipient's backed
// balance — §128: real-money value lands onchain, never in the spend-only
// bonus ledger. The donor's fiat sits in the payment provider's settlement;
// this sends the equivalent USDC amount from the treasury hot wallet.
//
// Returns (false, nil) only when there is no member wallet to credit
// (platform recipient / zero-value row) — the caller then runs the
// bookkeeping settle. Errors are retryable (killswitch engaged, suspended
// recipient, provider transient) — the caller must not fall back on
// error or the tip could settle twice.
func (s *Service) SettleTipBacked(ctx context.Context, tipID, recipientUserID pgtype.UUID, netCredits int64) (bool, error) {
	if netCredits <= 0 || !recipientUserID.Valid {
		return false, nil
	}
	if engaged, kerr := s.killswitch(ctx); kerr != nil {
		return false, fmt.Errorf("stablecoin: killswitch check: %w", kerr)
	} else if engaged {
		// Retryable: the tip stays pending until the halt lifts rather
		// than silently degrading a real-money gift into bonus credits.
		return false, ErrKillswitch
	}
	w, err := s.store.GetWalletByUser(ctx, recipientUserID)
	if err != nil {
		if !errors.Is(err, ErrWalletNotFound) {
			return false, fmt.Errorf("stablecoin: tip recipient wallet: %w", err)
		}
		// Unprovisioned → auto-provision: the wallet is counterfactual
		// (deploys on first tx, costs nothing dormant) and receiving
		// needs no signer — the member activates their passkey later
		// via the §128.2 ceremony. No member-auth needed for create.
		if w, err = s.ProvisionWallet(ctx, recipientUserID, ""); err != nil {
			return false, fmt.Errorf("stablecoin: tip recipient provision: %w", err)
		}
	}
	if w.Status != WalletStatusActive && w.Status != WalletStatusProvisioned {
		// Suspended — retryable: the tip stays pending until the wallet
		// reactivates or the settle TTL fails it (donor payment refunds).
		return false, ErrWalletSuspended
	}

	hot, err := s.store.GetTreasuryWallet(ctx, w.Chain, TreasuryHot)
	if err != nil {
		return false, fmt.Errorf("stablecoin: hot treasury: %w", err)
	}
	if hot.Locator == "" {
		return false, fmt.Errorf("stablecoin: hot treasury has no provider locator")
	}

	// Settlement math must fail closed on a missing rate — the
	// creditsPerUSD fallback is display-only per its own contract.
	rate := s.creditsPerUnit(ctx, "usd")
	if rate <= 0 {
		return false, fmt.Errorf("stablecoin: USD credits rate missing — cannot price backed settle")
	}
	minor := int64(math.Round(float64(netCredits) * 1_000_000 / rate))
	res, err := s.provider.Transfer(ctx, hot.Locator, TransferInput{
		Asset:         AssetUSDC,
		RecipientAddr: w.Address,
		AmountMinor:   minor,
		Idempotency:   "tip:fiat:" + tipID.String(),
		Reference:     "tip:" + tipID.String(),
		// Empty Signer — the treasury hot wallet is platform-custodial;
		// the API key signs directly (no delegated scope needed).
		Signer: "",
	})
	if err != nil {
		return false, fmt.Errorf("stablecoin: treasury tip send: %w", err)
	}

	// Mirror the inbound leg now so the member's feed shows it immediately;
	// the webhook/reconcile path confirms it onchain-side (dedupe by sig).
	// A signature-less submission mirrors nothing — a NULL-sig row can
	// never transition by sig and would duplicate the webhook's insert.
	if res.TxSignature == "" {
		return true, nil
	}
	if _, err := s.store.InsertActivity(ctx, Activity{
		WalletID:      w.ID,
		Direction:     "in",
		Kind:          ActivityTipIn,
		Asset:         AssetUSDC,
		AmountMinor:   minor,
		CreditsAmount: netCredits,
		TxSignature:   res.TxSignature,
		Status:        "pending",
		OccurredAt:    time.Now().UTC(),
	}); err != nil {
		slog.Warn("stablecoin: tip mirror insert failed", "tip_id", tipID, "sig", res.TxSignature, "error", err)
	}
	return true, nil
}

// ProposeSweep creates a hot→cold sweep proposal when the hot float
// exceeds stablecoin.hot_float_cap_minor (stablecoin:treasury_sweep).
// The ceremony executes in Squads (propose → co-sign → execute) — this
// backend only proposes and watches.
func (s *Service) ProposeSweep(ctx context.Context) error {
	chain := s.settings.GetStringOr(ctx, "stablecoin.network", "solana")
	hot, err := s.store.GetTreasuryWallet(ctx, chain, TreasuryHot)
	if err != nil {
		if errors.Is(err, ErrTreasuryNotFound) {
			// Treasury not seeded yet (fresh deploy) — the cron must not
			// deadletter every tick; the ops runbook seeds the registry.
			slog.Info("stablecoin: treasury hot wallet unseeded — sweep skipped")
			return nil
		}
		return fmt.Errorf("stablecoin: hot wallet: %w", err)
	}
	cold, err := s.store.GetTreasuryWallet(ctx, chain, TreasuryCold)
	if err != nil {
		if errors.Is(err, ErrTreasuryNotFound) {
			slog.Info("stablecoin: treasury cold vault unseeded — sweep skipped")
			return nil
		}
		return fmt.Errorf("stablecoin: cold vault: %w", err)
	}
	if hot.Locator == "" {
		slog.Info("stablecoin: hot wallet has no provider locator — sweep skipped")
		return nil
	}

	capMinor := hot.FloatCapMinor
	if capMinor <= 0 {
		capMinor = int64(s.settings.GetIntOr(ctx, "stablecoin.hot_float_cap_minor", 5_000_000_000))
	}

	// Live hot balance from the provider — sweep the excess over cap.
	pw, err := s.provider.GetWallet(ctx, hot.Locator)
	if err != nil {
		if errors.Is(err, ErrNotImplemented) {
			slog.Info("stablecoin: provider balance read unimplemented — sweep skipped")
			return nil
		}
		return fmt.Errorf("stablecoin: hot balance: %w", err)
	}
	// Skip assets with an open proposal — a cron tick must not queue a
	// second sweep while one awaits ceremony approval.
	open := map[string]bool{}
	for _, st := range []string{"proposed", "approved"} {
		if rows, lerr := s.store.ListSweepsByState(ctx, st, 100, 0); lerr == nil {
			for _, sw := range rows {
				open[sw.Asset] = true
			}
		}
	}
	for _, asset := range s.assetList(ctx) {
		bal := pw.Balances[asset]
		if bal <= capMinor || open[asset] {
			continue
		}
		if _, err := s.store.InsertTreasurySweep(ctx, TreasurySweep{
			Chain:       chain,
			Asset:       asset,
			AmountMinor: bal - capMinor,
			HotWalletID: hot.ID,
			ColdAddress: cold.Address,
			ProposedBy:  "task:treasury_sweep",
		}); err != nil {
			return fmt.Errorf("stablecoin: propose sweep %s: %w", asset, err)
		}
	}
	return nil
}

// ApproveSweep records the treasury.sweep_approve decision for a proposal
// (route-gated, super_admin default). The Squads ceremony proceeds
// off-backend; executed_sig is written when the mirror observes the tx.
func (s *Service) ApproveSweep(ctx context.Context, proposalID pgtype.UUID, approverID pgtype.UUID, squadsProposalID string) error {
	sw, err := s.store.GetTreasurySweep(ctx, proposalID)
	if err != nil {
		return err
	}
	if sw.State != "proposed" {
		return fmt.Errorf("stablecoin: sweep not approvable in state %s", sw.State)
	}
	return s.store.DecideTreasurySweep(ctx, proposalID, "approved", approverID, squadsProposalID)
}

// RejectSweep records a rejection on a proposed sweep.
func (s *Service) RejectSweep(ctx context.Context, proposalID pgtype.UUID, approverID pgtype.UUID) error {
	sw, err := s.store.GetTreasurySweep(ctx, proposalID)
	if err != nil {
		return err
	}
	if sw.State != "proposed" {
		return fmt.Errorf("stablecoin: sweep not rejectable in state %s", sw.State)
	}
	return s.store.DecideTreasurySweep(ctx, proposalID, "rejected", approverID, "")
}

// ListSweeps pages sweep proposals for GET /admin/treasury/sweeps —
// ?state filters to one state; empty returns all states per state-grouped
// union (implemented as state="all" passthrough below).
func (s *Service) ListSweeps(ctx context.Context, state string, limit, offset int32) ([]TreasurySweep, error) {
	if state == "" {
		// Registry states — union of all, newest first per state group.
		var out []TreasurySweep
		for _, st := range []string{"proposed", "approved", "executed", "rejected", "expired"} {
			rows, err := s.store.ListSweepsByState(ctx, st, limit, offset)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
		}
		return out, nil
	}
	return s.store.ListSweepsByState(ctx, state, limit, offset)
}

// ListWallets pages the member wallet registry for
// GET /admin/stablecoin/wallets — ?status + ?q filters per catalog.
func (s *Service) ListWallets(ctx context.Context, status, q string, limit, offset int32) ([]Wallet, error) {
	return s.store.ListWallets(ctx, status, q, limit, offset)
}

// mirrorTreasuryEvent handles provider activity addressed to a treasury
// wallet — the authoritative completion write for approved sweeps: a
// hot→cold out-transfer matching an open proposal marks it executed with
// the real chain signature (§128.7 — the Squads ceremony executes
// off-backend; the mirror is how the registry learns it landed).
// Returns nil when the event was consumed (treasury-owned or recognized),
// ErrTreasuryNotFound otherwise so the caller can fall through to the
// unknown-wallet drop.
func (s *Service) mirrorTreasuryEvent(ctx context.Context, chain, walletAddr string, ev ProviderEvent) error {
	tw, err := s.store.GetTreasuryWalletByAddress(ctx, chain, walletAddr)
	if err != nil {
		return err
	}
	// Only the hot wallet's outbound transfers can execute a sweep.
	if tw.Role != TreasuryHot || ev.Type == "failed" {
		return nil
	}
	if st := eventStatus(ev); st != "confirmed" {
		return nil
	}
	if ev.TxSignature == "" {
		return nil
	}
	// Direction check: the event must be an out-transfer headed to the
	// cold vault — in-events and cold-bound noise can't mark execution.
	kind, direction := classifyEvent(ev)
	if direction != "out" || kind == ActivityEscrowFunding {
		return nil
	}
	sw, err := s.store.GetApprovedSweepForTransfer(ctx, chain, normalizeAsset(ev.Asset), tw.ID, ev.AmountMinor)
	if err != nil {
		return nil // no matching open proposal — ordinary treasury traffic
	}
	var id pgtype.UUID
	if err := id.Scan(sw.ID); err != nil {
		return nil
	}
	if err := s.store.MarkSweepExecuted(ctx, id, ev.TxSignature); err != nil {
		return fmt.Errorf("stablecoin: mark sweep executed: %w", err)
	}
	slog.Info("stablecoin: treasury sweep executed", "sweep_id", sw.ID, "asset", sw.Asset, "sig", ev.TxSignature)
	return nil
}
