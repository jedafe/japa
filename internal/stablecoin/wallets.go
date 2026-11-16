package stablecoin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// WalletSummary is the member-facing wallet view (GET /me/stablecoin-wallet).
// Credits-denominated — members never see USDC/USDT/Solana terminology.
type WalletSummary struct {
	Address         string         `json:"address"`
	Status          string         `json:"status"`
	RecoveryMethods []string       `json:"recovery_methods"`
	Balance         BalanceSummary `json:"balance"`
}

// GetWalletSummary composes the member wallet view: backed tier from the
// onchain mirror, bonus tier from the §28 credits ledger, pending
// confirmations merged in.
func (s *Service) GetWalletSummary(ctx context.Context, userID pgtype.UUID) (*WalletSummary, error) {
	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: wallet: %w", err)
	}
	if w == nil {
		return nil, nil // not provisioned yet — client triggers lazy provisioning
	}
	// TODO(§128.9): mirror + bonus composition once ledger seam lands.
	return &WalletSummary{
		Address:         w.Address,
		Status:          w.Status,
		RecoveryMethods: w.RecoveryMethods,
	}, nil
}

// ProvisionWallet lazily creates a member's smart wallet — called on signup
// completion or first wallet-surface visit (stablecoin:provision_wallet).
// The address is counterfactual: assigned now, deployed on first tx, so
// dormant wallets cost nothing.
func (s *Service) ProvisionWallet(ctx context.Context, userID pgtype.UUID, memberAuth string) (*Wallet, error) {
	if existing, err := s.store.GetWalletByUser(ctx, userID); err != nil {
		return nil, fmt.Errorf("stablecoin: wallet lookup: %w", err)
	} else if existing != nil {
		return existing, nil // idempotent
	}

	pw, err := s.provider.CreateWallet(ctx, CreateWalletInput{
		UserID:          userID.String(),
		Chain:           s.settings.GetStringOr(ctx, "stablecoin.network", "solana"),
		RecoveryMethods: []string{"email", "phone"},
		MemberAuth:      memberAuth,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: create wallet: %w", err)
	}

	w, err := s.store.InsertWallet(ctx, Wallet{
		UserID:  userID.String(),
		Chain:   pw.Chain,
		Address: pw.Address,
		Locator: pw.Locator,
		Status:  WalletStatusProvisioned,
	})
	if err != nil {
		return nil, fmt.Errorf("stablecoin: persist wallet: %w", err)
	}

	// Register the scoped delegated signer — powers in-app spending without
	// per-action signatures. Scope values are the blast-radius ceiling.
	// Registration itself needs member recovery approval (OTP ceremony);
	// scopes are immutable — expiry forces rotation, not renewal (§128.2).
	dailyCap := s.settings.GetIntOr(ctx, "stablecoin.signer_daily_cap_usd_minor", 5000)
	scopeDays := s.settings.GetIntOr(ctx, "stablecoin.signer_scope_expiry_days", 365)
	// TODO(§128.2): provider.RegisterDelegatedSigner + persist signer state —
	// registration requires the member's recovery approval ceremony.
	_ = dailyCap
	_ = scopeDays
	return w, nil
}
