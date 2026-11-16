package stablecoin

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
)

// Escrow orchestration for §111/§114 service bookings — per-order Squads
// 2-of-3 multisig vault (buyer, seller, platform-arbiter; threshold 2).
// Deferred capture and treasury-held escrow are rejected (ADR-140).
//
// State machine: funding → funded → released | refunded | disputed.
// Ghosted buyers: seller + arbiter release after
// stablecoin.escrow_confirm_hours (stablecoin:escrow_timeout).

// FundEscrow creates the vault for a booking and returns the funding op —
// member signs via passkey unless delegated-signer program-call scopes are
// available (open validation §128, item 1).
func (s *Service) FundEscrow(ctx context.Context, bookingID pgtype.UUID) (SignableOp, error) {
	// TODO(§128.5): create Squads vault (buyer/seller/arbiter, threshold 2),
	// persist service_escrows row, return funding SignableOp.
	return SignableOp{}, fmt.Errorf("stablecoin: escrow funding not implemented")
}

// ReleaseEscrow executes the release after delivery confirmation or
// arbiter decision: seller share → seller wallet + platform fee → hot,
// batched.
func (s *Service) ReleaseEscrow(ctx context.Context, escrowID pgtype.UUID, decision *ArbiterDecision) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return fmt.Errorf("stablecoin: escrow: %w", err)
	}
	if e == nil || e.State != EscrowFunded && e.State != EscrowDisputed {
		return fmt.Errorf("stablecoin: escrow not releasable")
	}
	// TODO(§128.5): buyer-confirm → routine release; dispute → arbiter +
	// winning party co-sign. Arbiter alone can never move funds.
	_ = decision
	return nil
}

// HandleEscrowTimeout is driven by stablecoin:escrow_timeout — releases
// funded escrows whose confirmation_deadline passed (ghosted buyers).
func (s *Service) HandleEscrowTimeout(ctx context.Context) error {
	// TODO(§128.5): list funded escrows past deadline → seller + arbiter
	// release path.
	return nil
}
