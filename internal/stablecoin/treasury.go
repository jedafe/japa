package stablecoin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Treasury is platform money only — hot/cold split per §128.7/§128.11.
// The cold vault is a Squads multisig; the backend watches balances for
// reconciliation and never holds its keys.

// TreasuryView is the admin treasury surface (GET /admin/treasury/wallets).
type TreasuryView struct {
	Wallets []TreasuryWallet `json:"wallets"`
}

// ListTreasury returns the treasury registry for the admin surface.
func (s *Service) ListTreasury(ctx context.Context) (*TreasuryView, error) {
	wallets, err := s.store.ListTreasuryWallets(ctx)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: treasury: %w", err)
	}
	return &TreasuryView{Wallets: wallets}, nil
}

// ProposeSweep creates a hot→cold sweep proposal when the hot float
// exceeds stablecoin.hot_float_cap_minor (stablecoin:treasury_sweep).
// The ceremony executes in Squads (propose → co-sign → execute) — this
// backend only proposes and watches.
func (s *Service) ProposeSweep(ctx context.Context) error {
	chain := s.settings.GetStringOr(ctx, "stablecoin.network", "solana")
	hot, err := s.store.GetTreasuryWallet(ctx, chain, TreasuryHot)
	if err != nil {
		return fmt.Errorf("stablecoin: hot wallet: %w", err)
	}
	cold, err := s.store.GetTreasuryWallet(ctx, chain, TreasuryCold)
	if err != nil {
		return fmt.Errorf("stablecoin: cold vault: %w", err)
	}

	capMinor := hot.FloatCapMinor
	if capMinor <= 0 {
		capMinor = int64(s.settings.GetIntOr(ctx, "stablecoin.hot_float_cap_minor", 2500000))
	}

	// Live hot balance from the provider — sweep the excess over cap.
	pw, err := s.provider.GetWallet(ctx, hot.Locator)
	if err != nil {
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
