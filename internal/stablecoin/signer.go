package stablecoin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ErrKillswitch is returned when the platform-wide delegated-signer halt is
// set (jc:stablecoin:signer_killswitch — §128.10 incident response).
var ErrKillswitch = errors.New("stablecoin: delegated-signer ops halted (killswitch)")

// ErrScopeExceeded marks a transfer that would exceed the onchain scope cap —
// mapped to a user-facing "daily limit reached" rather than a hard error.
var ErrScopeExceeded = errors.New("stablecoin: transfer exceeds delegated-signer scope")

// ErrWalletNotSettleable covers sender wallets missing an active delegated
// signer (provisioned-but-unregistered or suspended).
var ErrWalletNotSettleable = errors.New("stablecoin: sender wallet not settleable")

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
		return TransferResult{}, ErrWalletNotSettleable
	}

	// Scope pre-check: today's settled volume for this asset must stay
	// under the registered daily cap (the chain enforces regardless — this
	// is the clean-UX path, mapped to a limit message not a hard error).
	capMinor := scopeCapFor(w.DelegatedSigner, in.Asset)
	if capMinor > 0 {
		spent, err := s.dailySettledMinor(ctx, w, in.Asset)
		if err != nil {
			return TransferResult{}, fmt.Errorf("stablecoin: scope check: %w", err)
		}
		if spent+in.AmountMinor+in.FeeMinor > capMinor {
			return TransferResult{}, fmt.Errorf("%w: daily cap %d minor", ErrScopeExceeded, capMinor)
		}
	}

	// The fee leg lands on the hot treasury wallet (§128.5 — batched
	// atomically with the recipient leg).
	feeAddr := ""
	if in.FeeMinor > 0 {
		hot, err := s.store.GetTreasuryWallet(ctx, w.Chain, TreasuryHot)
		if err != nil {
			return TransferResult{}, fmt.Errorf("stablecoin: fee leg treasury: %w", err)
		}
		feeAddr = hot.Address
	}

	return s.provider.Transfer(ctx, w.Locator, TransferInput{
		Asset:         in.Asset,
		RecipientAddr: in.ToAddress,
		AmountMinor:   in.AmountMinor,
		FeeAddr:       feeAddr,
		FeeMinor:      in.FeeMinor,
		Idempotency:   in.Idempotency,
		Reference:     in.Reference,
	})
}

// scopeCapFor returns the registered daily cap for an asset (0 = no scope).
func scopeCapFor(ds *DelegatedSigner, asset string) int64 {
	for _, sc := range ds.Scopes {
		if sc.Asset == asset {
			return sc.DailyCapMinor
		}
	}
	return 0
}

// dailySettledMinor sums today's out-volume for the asset via SQL
// aggregate — tips/purchases/withdrawals all count toward the scope
// window. UTC-day truncation matches the provider's 24h spending-limit
// interval closely enough for the UX pre-check (the chain enforces its
// own window regardless).
func (s *Service) dailySettledMinor(ctx context.Context, w *Wallet, asset string) (int64, error) {
	wid, err := pgUUID(w.ID)
	if err != nil {
		return 0, err
	}
	return s.store.SumOutByAssetSince(ctx, wid, asset, time.Now().UTC().Truncate(24*time.Hour))
}

// BuildMemberWithdrawal returns the signable op for
// POST /me/stablecoin-wallet/withdrawals. Japa.to cannot execute member
// withdrawals — the member's passkey signs; we only build and relay.
// The platform fee is collected post-confirmation by collectWithdrawalFee —
// a member-signed send can't carry a second delegated leg (§128.6).
func (s *Service) BuildMemberWithdrawal(ctx context.Context, userID pgtype.UUID, in WithdrawalInput) (SignableOp, error) {
	w, err := s.store.GetWalletByUser(ctx, userID)
	if err != nil {
		return SignableOp{}, fmt.Errorf("stablecoin: wallet: %w", err)
	}
	if w == nil || w.Status != WalletStatusActive {
		return SignableOp{}, ErrWalletNotSettleable
	}

	// Withdrawal fee — gross-up is wrong here (member controls the send);
	// the fee leg is carved out of the signed amount (§128.6).
	bps := s.settings.GetIntOr(ctx, "stablecoin.withdrawal_fee_bps", 100)
	if in.FeeMinor == 0 && bps > 0 {
		in.FeeMinor = in.AmountMinor * int64(bps) / 10_000
	}
	return s.provider.BuildWithdrawal(ctx, w.Locator, in)
}

// collectWithdrawalFee settles the platform fee on a confirmed member
// withdrawal. The member-signed send can't carry a second leg (only the
// member signs), so the fee is collected via the delegated signer once the
// withdrawal lands — scope-capped, counted toward the same daily cap.
// Called by MirrorActivity on confirmed withdrawal rows.
func (s *Service) collectWithdrawalFee(ctx context.Context, w *Wallet, amountMinor int64, asset, txSig string) {
	bps := s.settings.GetIntOr(ctx, "stablecoin.withdrawal_fee_bps", 100)
	fee := amountMinor * int64(bps) / 10_000
	if fee <= 0 || w.DelegatedSigner == nil {
		return
	}
	// The killswitch gates every delegated-signer op — a direct provider
	// call here would bypass the incident-response halt.
	if halted, err := s.killswitch(ctx); err != nil || halted {
		slog.Warn("stablecoin: withdrawal fee deferred (killswitch/error)", "sig", txSig, "fee", fee)
		return
	}
	hot, err := s.store.GetTreasuryWallet(ctx, w.Chain, TreasuryHot)
	if err != nil {
		slog.Warn("stablecoin: withdrawal fee — no hot treasury", "sig", txSig, "error", err)
		return
	}
	if _, err := s.provider.Transfer(ctx, w.Locator, TransferInput{
		Asset:         asset,
		RecipientAddr: hot.Address,
		AmountMinor:   fee,
		Idempotency:   txSig + ":withdrawal_fee",
		Reference:     "withdrawal_fee",
	}); err != nil {
		// Non-fatal: member funds already moved; reconcile surfaces the
		// uncollected fee for recovery.
		slog.Warn("stablecoin: withdrawal fee collection failed", "sig", txSig, "fee", fee, "error", err)
	}
}

// RotateSignerScopes replaces an expiring delegated signer — scopes are
// immutable (§128.2), so renewal means registering a fresh scoped signer
// (member OTP approval ceremony) and dropping the old reference locally.
func (s *Service) RotateSignerScopes(ctx context.Context, w *Wallet) error {
	if w == nil {
		return fmt.Errorf("stablecoin: nil wallet")
	}
	// Drop the prior registration first — two live signers with the same
	// scopes would double the blast radius during overlap. Removal needs
	// member recovery approval like registration; if it fails the provider
	// rejects re-registration as a duplicate and the error propagates.
	if w.DelegatedSigner != nil && w.DelegatedSigner.SignerRef != "" {
		if err := s.provider.RemoveDelegatedSigner(ctx, w.Locator, w.DelegatedSigner.SignerRef); err != nil {
			return fmt.Errorf("stablecoin: remove prior signer: %w", err)
		}
	}
	return s.registerDelegatedSigner(ctx, w)
}
