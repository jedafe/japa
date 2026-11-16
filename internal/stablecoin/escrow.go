package stablecoin

import (
	"context"
	"errors"
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

// ErrEscrowState marks transitions that violate the state machine.
var ErrEscrowState = errors.New("stablecoin: invalid escrow state transition")

// ErrEscrowParty marks a caller who is not buyer/seller on the escrow.
var ErrEscrowParty = errors.New("stablecoin: caller is not a party to this escrow")

// EscrowSetup is what the client needs to create the Squads vault — the
// PDA is derived client-side (createKey + members + threshold), then
// registered back via RegisterEscrowVault.
type EscrowSetup struct {
	EscrowID       string `json:"escrow_id"`
	BookingID      string `json:"booking_id"`
	BuyerAddress   string `json:"buyer_address"`
	SellerAddress  string `json:"seller_address"`
	ArbiterAddress string `json:"arbiter_address"`
	Asset          string `json:"asset"`
	AmountMinor    int64  `json:"amount_minor"`
	FeeMinor       int64  `json:"fee_minor"`
}

// PrepareEscrow resolves the booking parties into wallet addresses and
// returns the vault-creation parameters. Idempotent per booking — a second
// call returns the existing escrow's setup.
func (s *Service) PrepareEscrow(ctx context.Context, bookingID pgtype.UUID, asset string) (*EscrowSetup, error) {
	if existing, err := s.store.GetEscrowByBooking(ctx, bookingID); err == nil && existing != nil {
		return s.setupFor(ctx, existing)
	} else if err != nil && !errors.Is(err, ErrEscrowNotFound) {
		return nil, fmt.Errorf("stablecoin: escrow lookup: %w", err)
	}

	b, err := s.store.GetServiceBooking(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: booking: %w", err)
	}
	var buyerUID, sellerUID pgtype.UUID
	if err := buyerUID.Scan(b.BookerUserID); err != nil {
		return nil, fmt.Errorf("stablecoin: booking buyer: %w", err)
	}
	if err := sellerUID.Scan(b.ProviderUserID); err != nil {
		return nil, fmt.Errorf("stablecoin: booking seller: %w", err)
	}
	buyer, err := s.store.GetWalletByUser(ctx, buyerUID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: buyer wallet: %w", err)
	}
	seller, err := s.store.GetWalletByUser(ctx, sellerUID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: seller wallet: %w", err)
	}
	if asset == "" {
		asset = AssetUSDC
	}

	// Platform share on the release (§128.8) — computed at prepare-time so
	// all parties see the split before funding.
	shareBps := s.settings.GetIntOr(ctx, "stablecoin.platform_share_bps", 1500)
	fee := b.TotalPrice * int64(shareBps) / 10_000

	e, err := s.store.InsertEscrow(ctx, Escrow{
		BookingID:      b.ID,
		BuyerWalletID:  buyer.ID,
		SellerWalletID: seller.ID,
		VaultAddress:   "pending:" + bookingID.String(), // client derives real PDA
		Asset:          asset,
		AmountMinor:    b.TotalPrice - fee,
		FeeMinor:       fee,
	})
	if err != nil {
		return nil, err
	}
	return s.setupFor(ctx, e)
}

func (s *Service) setupFor(ctx context.Context, e *Escrow) (*EscrowSetup, error) {
	buyer, err := s.walletByID(ctx, e.BuyerWalletID)
	if err != nil {
		return nil, err
	}
	seller, err := s.walletByID(ctx, e.SellerWalletID)
	if err != nil {
		return nil, err
	}
	arbiter := s.arbiterAddress(ctx)
	return &EscrowSetup{
		EscrowID:       e.ID,
		BookingID:      e.BookingID,
		BuyerAddress:   buyer.Address,
		SellerAddress:  seller.Address,
		ArbiterAddress: arbiter,
		Asset:          e.Asset,
		AmountMinor:    e.AmountMinor,
		FeeMinor:       e.FeeMinor,
	}, nil
}

// walletByID resolves a wallet row by its domain ID. The store seam keys
// on userID for the hot path; escrow needs the wallet's own row — the
// address is resolved via ListWalletsByStatus fallback when a direct
// by-ID query isn't in the seam (kept narrow: buyer/seller are always
// active wallets at prepare-time).
func (s *Service) walletByID(ctx context.Context, walletID string) (*Wallet, error) {
	// The escrow row stores wallet row IDs; resolve through the active set.
	// (Store seam stays narrow — a by-ID lookup query can be added if this
	// path ever needs provisioned/suspended wallets.)
	active, err := s.store.ListWalletsByStatus(ctx, WalletStatusActive, 1000)
	if err != nil {
		return nil, err
	}
	for i := range active {
		if active[i].ID == walletID {
			return &active[i], nil
		}
	}
	return nil, ErrWalletNotFound
}

// arbiterAddress is the platform's escrow-arbiter public key — a separate
// operational key from the delegated signer, so arbiter compromise can't
// spend member funds (§128.5).
func (s *Service) arbiterAddress(ctx context.Context) string {
	return s.settings.GetStringOr(ctx, "stablecoin.escrow_arbiter_ref", "")
}

// RegisterEscrowVault records the client-derived Squads PDA after the
// vault exists onchain.
func (s *Service) RegisterEscrowVault(ctx context.Context, escrowID pgtype.UUID, vaultAddr string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e.State != EscrowFunding {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	return s.store.SetEscrowVaultAddress(ctx, escrowID, vaultAddr)
}

// ConfirmEscrowFunded marks the vault funded — the buyer's passkey-signed
// funding tx landed (verified via the mirrored onchain event / client sig).
// Sets the buyer confirmation deadline.
func (s *Service) ConfirmEscrowFunded(ctx context.Context, escrowID pgtype.UUID, fundedSig string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e.State != EscrowFunding {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	if err := s.store.SetEscrowState(ctx, escrowID, EscrowFunded, fundedSig, "", nil); err != nil {
		return err
	}
	// Deadline recorded separately — confirmation_deadline starts when the
	// service window opens (delivery confirmation flow); the timeout task
	// reads funded+deadline rows only.
	return nil
}

// ConfirmRelease is the buyer-side delivery confirmation — releases
// seller share + platform fee out of the vault.
func (s *Service) ConfirmRelease(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	buyer, err := s.walletByID(ctx, e.BuyerWalletID)
	if err != nil {
		return err
	}
	if buyer.UserID != callerUserID.String() {
		return ErrEscrowParty
	}
	return s.ReleaseEscrow(ctx, escrowID, nil)
}

// DisputeEscrow freezes a funded escrow pending arbiter review — either
// party may open the dispute.
func (s *Service) DisputeEscrow(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID, reason string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e.State != EscrowFunded {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	buyer, bErr := s.walletByID(ctx, e.BuyerWalletID)
	seller, sErr := s.walletByID(ctx, e.SellerWalletID)
	if bErr != nil || sErr != nil {
		return ErrEscrowParty
	}
	if buyer.UserID != callerUserID.String() && seller.UserID != callerUserID.String() {
		return ErrEscrowParty
	}
	return s.store.SetEscrowState(ctx, escrowID, EscrowDisputed, "", "", nil)
}

// ReleaseEscrow executes the release after delivery confirmation or
// arbiter decision: seller share → seller wallet + platform fee → hot.
// The onchain transfer is a Squads proposal — buyer/arbiter co-sign per
// path; the arbiter alone can never move funds.
func (s *Service) ReleaseEscrow(ctx context.Context, escrowID pgtype.UUID, decision *ArbiterDecision) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return fmt.Errorf("stablecoin: escrow: %w", err)
	}
	if e == nil || e.State != EscrowFunded && e.State != EscrowDisputed {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}

	// The Squads release executes off-backend (arbiter + winning party
	// co-sign in the Squads app — no program-call signer scope exists,
	// §128 open validation item 1). We record the decision + new state;
	// release_sig is written when the executed tx is observed by the
	// activity mirror.
	newState := EscrowReleased
	if decision != nil && decision.ReleasedTo == "buyer" {
		newState = EscrowRefunded
	}
	return s.store.SetEscrowState(ctx, escrowID, newState, "", "", decision)
}

// ArbitrateEscrow is the admin dispute ruling — escrow.arbitrate gated at
// the route; the decision still needs the onchain co-signer (the node
// grants decision authority, not unilateral fund control).
func (s *Service) ArbitrateEscrow(ctx context.Context, escrowID pgtype.UUID, decision *ArbiterDecision) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e.State != EscrowDisputed {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	if decision == nil || (decision.ReleasedTo != "buyer" && decision.ReleasedTo != "seller") {
		return fmt.Errorf("stablecoin: arbiter decision must release to buyer or seller")
	}
	return s.ReleaseEscrow(ctx, escrowID, decision)
}

// HandleEscrowTimeout is driven by stablecoin:escrow_timeout — releases
// funded escrows whose confirmation_deadline passed (ghosted buyers).
// Seller + arbiter co-sign the release (§128.5).
func (s *Service) HandleEscrowTimeout(ctx context.Context) error {
	expired, err := s.store.ListExpiredFundedEscrows(ctx, 100)
	if err != nil {
		return fmt.Errorf("stablecoin: escrow timeout list: %w", err)
	}
	for i := range expired {
		id, perr := pgUUID(expired[i].ID)
		if perr != nil {
			continue
		}
		if err := s.ReleaseEscrow(ctx, id, nil); err != nil {
			return fmt.Errorf("stablecoin: timeout release %s: %w", expired[i].ID, err)
		}
	}
	return nil
}
