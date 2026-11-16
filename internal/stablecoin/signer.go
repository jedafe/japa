package stablecoin

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrKillswitch is returned when the platform-wide delegated-signer halt is
// set (jc:stablecoin:signer_killswitch — §128.10 incident response).
var ErrKillswitch = errors.New("stablecoin: delegated-signer ops halted (killswitch)")

// ErrScopeExceeded marks a transfer that would exceed the onchain scope cap —
// mapped to a user-facing "daily limit reached" rather than a hard error.
var ErrScopeExceeded = errors.New("stablecoin: transfer exceeds delegated-signer scope")

// SettleInput is one in-app stablecoin spend (tip, purchase, creator share).
// The fee leg batches atomically into the same operation (§128.5).
type SettleInput struct {
	FromUserID  pgtype.UUID
	ToAddress   string // recipient member wallet, treasury hot, or escrow vault
	Asset       string
	AmountMinor int64
	FeeMinor    int64
	Idempotency string
	Reference   string
}

// Settle executes a delegated-signer transfer on behalf of a member —
// gated by the killswitch and the scope pre-check (the chain enforces the
// scope regardless; the pre-check exists for a clean UX error).
func (s *Service) Settle(ctx context.Context, in SettleInput) (TransferResult, error) {
	halted, err := s.killswitch(ctx)
	if err != nil {
		return TransferResult{}, fmt.Errorf("stablecoin: killswitch check: %w", err)
	}
	if halted {
		return TransferResult{}, ErrKillswitch
	}

	w, err := s.store.GetWalletByUser(ctx, in.FromUserID)
	if err != nil {
		return TransferResult{}, fmt.Errorf("stablecoin: sender wallet: %w", err)
	}
	if w == nil || w.Status != WalletStatusActive || w.DelegatedSigner == nil {
		return TransferResult{}, fmt.Errorf("stablecoin: sender wallet not settleable")
	}

	// TODO(§128.5): scope pre-check — sum today's settled volume for this
	// wallet against DelegatedSigner.Scopes[].DailyCapMinor.
	// TODO(§128.5): treasury hot address resolution from treasury_wallets.

	return s.provider.Transfer(ctx, w.Locator, TransferInput{
		Asset:         in.Asset,
		RecipientAddr: in.ToAddress,
		AmountMinor:   in.AmountMinor,
		FeeAddr:       "", // resolved from treasury_wallets (hot)
		FeeMinor:      in.FeeMinor,
		Idempotency:   in.Idempotency,
		Reference:     in.Reference,
	})
}

// BuildMemberWithdrawal returns the signable op for GET → POST
// /me/stablecoin-wallet/withdrawals. Japa.to cannot execute member
// withdrawals — the member's passkey signs; we only build and relay.
func (s *Service) BuildMemberWithdrawal(ctx context.Context, userID pgtype.UUID, in WithdrawalInput) (SignableOp, error) {
	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return SignableOp{}, fmt.Errorf("stablecoin: wallet: %w", err)
	}
	if w == nil || w.Status != WalletStatusActive {
		return SignableOp{}, fmt.Errorf("stablecoin: wallet not active")
	}
	// TODO(§128.6): compute withdrawal fee (stablecoin.withdrawal_fee_bps),
	// require step_up + Idempotency-Key (Rule 9, SEC-W-015 precedent).
	return s.provider.BuildWithdrawal(ctx, w.Locator, in)
}
