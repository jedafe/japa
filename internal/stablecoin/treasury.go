package stablecoin

import (
	"context"
	"fmt"
)

// Treasury is platform money only — hot/cold split per §128.7/§128.11.
// The cold vault is a plain Squads multisig; the backend watches balances
// for reconciliation and never holds its keys.

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

// ProposeSweep creates a hot→cold sweep proposal when float exceeds
// stablecoin.hot_float_cap_minor (stablecoin:treasury_sweep). The ceremony
// executes in Squads (propose → co-sign → execute) — this backend only
// proposes and watches.
func (s *Service) ProposeSweep(ctx context.Context) error {
	floatCap := s.settings.GetIntOr(ctx, "stablecoin.hot_float_cap_minor", 2500000)
	_ = floatCap
	// TODO(§128.7): read hot balance → if > cap, create sweep proposal row
	// (pending treasury.sweep_approve) + notify signers.
	return nil
}

// ApproveSweep records the treasury.sweep_approve co-sign for a proposal.
// Guards hot-wallet ops only — the cold vault needs no API approval because
// its keys are unreachable from here (§128.10 loss table).
func (s *Service) ApproveSweep(ctx context.Context, proposalID string, approverID string) error {
	// TODO(§128.7): validate approver node (treasury.sweep_approve,
	// super_admin default) → mark approved → Squads ceremony proceeds.
	return fmt.Errorf("stablecoin: sweep approval not implemented")
}
