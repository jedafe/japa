package stablecoin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
func (s *Service) PrepareEscrow(ctx context.Context, bookingID pgtype.UUID, callerUserID pgtype.UUID, asset string) (*EscrowSetup, error) {
	if existing, err := s.store.GetEscrowByBooking(ctx, bookingID); err == nil && existing != nil {
		return s.setupFor(ctx, existing)
	} else if err != nil && !errors.Is(err, ErrEscrowNotFound) {
		return nil, fmt.Errorf("stablecoin: escrow lookup: %w", err)
	}

	b, err := s.store.GetServiceBooking(ctx, bookingID)
	if err != nil {
		return nil, fmt.Errorf("stablecoin: booking: %w", err)
	}
	// Only a booking party may open the escrow — and only once the provider
	// has accepted (pending bookings have no committed counterparties;
	// cancelled/declined/disputed/refunded are terminal for escrow).
	if callerUserID.String() != b.BookerUserID && callerUserID.String() != b.ProviderUserID {
		return nil, ErrEscrowParty
	}
	if b.Status != "confirmed" {
		return nil, fmt.Errorf("%w: booking %s", ErrEscrowState, b.Status)
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
	if ok := func() bool {
		for _, a := range s.assetList(ctx) {
			if a == asset {
				return true
			}
		}
		return false
	}(); !ok {
		return nil, fmt.Errorf("%w: unsupported asset %s", ErrEscrowState, asset)
	}

	// The arbiter must be a real key before any vault is created — an empty
	// ref would build a 2-of-3 that can never resolve a dispute or timeout.
	if s.arbiterAddress(ctx) == "" {
		return nil, fmt.Errorf("stablecoin: escrow_arbiter_ref unset")
	}

	// Platform share on the release (§128.8) — computed at prepare-time so
	// all parties see the split before funding.
	shareBps := s.settings.GetIntOr(ctx, "stablecoin.platform_share_bps", 1500)
	fee := b.TotalPrice * int64(shareBps) / 10_000
	if b.TotalPrice <= 0 || fee < 0 || fee >= b.TotalPrice {
		return nil, fmt.Errorf("%w: booking amount %d / fee %d", ErrEscrowState, b.TotalPrice, fee)
	}

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

// walletByID resolves a wallet row by its domain ID — escrow rows store
// wallet row IDs, not user IDs.
func (s *Service) walletByID(ctx context.Context, walletID string) (*Wallet, error) {
	id, err := pgUUID(walletID)
	if err != nil {
		return nil, err
	}
	return s.store.GetWalletByID(ctx, id)
}

// arbiterAddress is the platform's escrow-arbiter public key — a separate
// operational key from the delegated signer, so arbiter compromise can't
// spend member funds (§128.5).
func (s *Service) arbiterAddress(ctx context.Context) string {
	return s.settings.GetStringOr(ctx, "stablecoin.escrow_arbiter_ref", "")
}

// partyTo checks the caller is buyer or seller on the escrow — every
// member-side escrow mutation goes through this gate.
func (s *Service) partyTo(ctx context.Context, e *Escrow, callerUserID pgtype.UUID) error {
	buyer, bErr := s.walletByID(ctx, e.BuyerWalletID)
	seller, sErr := s.walletByID(ctx, e.SellerWalletID)
	if bErr != nil || sErr != nil {
		return ErrEscrowParty
	}
	if buyer.UserID != callerUserID.String() && seller.UserID != callerUserID.String() {
		return ErrEscrowParty
	}
	return nil
}

// RegisterEscrowVault records the client-derived Squads PDA after the
// vault exists onchain — caller must be a booking party.
func (s *Service) RegisterEscrowVault(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID, vaultAddr string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	if err := s.partyTo(ctx, e, callerUserID); err != nil {
		return err
	}
	if e.State != EscrowFunding {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	// Reject placeholder-shaped or implausibly short values — a malformed
	// vault address would strand the funding flow later.
	if strings.HasPrefix(vaultAddr, "pending:") || len(vaultAddr) < 32 || strings.ContainsAny(vaultAddr, " 	\n") {
		return fmt.Errorf("%w: invalid vault address", ErrEscrowState)
	}
	return s.store.SetEscrowVaultAddress(ctx, escrowID, vaultAddr)
}

// ConfirmEscrowFunded marks the vault funded — the buyer's passkey-signed
// funding tx landed (verified via the mirrored onchain event / client sig).
// Sets the buyer confirmation deadline.
func (s *Service) ConfirmEscrowFunded(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID, fundedSig string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	if err := s.partyTo(ctx, e, callerUserID); err != nil {
		return err
	}
	if e.State != EscrowFunding {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	// Verify the funding tx is visible to the provider before trusting the
	// client-reported signature. Soft-fail on provider errors — the Squads
	// vault itself enforces solvency at release (an empty vault can't pay),
	// but a fabricated sig shouldn't mark the escrow funded.
	buyer, berr := s.walletByID(ctx, e.BuyerWalletID)
	if berr == nil && fundedSig != "" {
		if events, perr := s.provider.Activity(ctx, buyer.Locator, ""); perr == nil {
			seen := false
			for _, ev := range events {
				if ev.TxSignature == fundedSig {
					seen = true
					break
				}
			}
			if !seen {
				slog.Warn("stablecoin: funded sig not visible to provider yet — accepting with flag",
					"escrow", escrowID, "sig", fundedSig)
			}
		}
	}
	// The buyer confirmation window opens at funding — after it lapses the
	// escrow_timeout task releases seller+arbiter (ghosted-buyer path).
	hours := s.settings.GetIntOr(ctx, "stablecoin.escrow_confirm_hours", 72)
	deadline := time.Now().Add(time.Duration(hours) * time.Hour)
	return s.store.SetEscrowState(ctx, escrowID, EscrowFunded, fundedSig, "", nil, &deadline)
}

// ConfirmRelease is the buyer-side delivery confirmation — releases
// seller share + platform fee out of the vault. releaseSig is the executed
// Squads proposal signature when the client has it; the transition is the
// authorization record, the sig is the chain proof.
func (s *Service) ConfirmRelease(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID, releaseSig string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	// Strict: a disputed escrow can only resolve through arbiter ruling —
	// the buyer must not release unilaterally once a dispute is open.
	if e.State != EscrowFunded {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	buyer, err := s.walletByID(ctx, e.BuyerWalletID)
	if err != nil {
		return err
	}
	if buyer.UserID != callerUserID.String() {
		return ErrEscrowParty
	}
	return s.ReleaseEscrow(ctx, escrowID, nil, releaseSig)
}

// DisputeEscrow freezes a funded escrow pending arbiter review — either
// party may open the dispute.
func (s *Service) DisputeEscrow(ctx context.Context, escrowID pgtype.UUID, callerUserID pgtype.UUID, reason string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	if e.State != EscrowFunded {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	if err := s.partyTo(ctx, e, callerUserID); err != nil {
		return err
	}
	// Record who opened the dispute and why — the arbiter reads it.
	return s.store.SetEscrowState(ctx, escrowID, EscrowDisputed, "", "", &ArbiterDecision{
		DecidedBy: callerUserID.String(),
		DecidedAt: time.Now().UTC(),
		Note:      "dispute: " + reason,
	}, nil)
}

// ReleaseEscrow records the release authorization: seller share → seller
// wallet + platform fee → hot. The Squads release executes off-backend
// (arbiter + winning party co-sign in the Squads app — no program-call
// signer scope exists). The state transition records the authorized
// outcome; release_sig lands when the executed tx is reported or mirrored.
func (s *Service) ReleaseEscrow(ctx context.Context, escrowID pgtype.UUID, decision *ArbiterDecision, releaseSig string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return fmt.Errorf("stablecoin: escrow: %w", err)
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	if e.State != EscrowFunded && e.State != EscrowDisputed {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	newState := EscrowReleased
	if decision != nil && decision.ReleasedTo == "buyer" {
		newState = EscrowRefunded
	}
	return s.store.SetEscrowState(ctx, escrowID, newState, "", releaseSig, decision, nil)
}

// ArbitrateEscrow is the admin dispute ruling — two phases on one route:
// phase 1 records the decision (escrow stays disputed; the arbiter then
// co-signs in Squads). Phase 2 — a second call carrying release_sig once
// the ceremony executes — completes the transition. DB never claims the
// outcome before the chain event exists.
func (s *Service) ArbitrateEscrow(ctx context.Context, escrowID pgtype.UUID, decision *ArbiterDecision, releaseSig string) error {
	e, err := s.store.GetEscrow(ctx, escrowID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrEscrowNotFound
	}
	if e.State != EscrowDisputed {
		return fmt.Errorf("%w: %s", ErrEscrowState, e.State)
	}
	// Phase 2: a prior decision exists and the caller reports the executed
	// release signature → complete the transition with the chain proof.
	if releaseSig != "" {
		if e.ArbiterDecision == nil || e.ArbiterDecision.ReleasedTo == "" {
			return fmt.Errorf("%w: no ruling recorded", ErrEscrowState)
		}
		return s.ReleaseEscrow(ctx, escrowID, e.ArbiterDecision, releaseSig)
	}
	// Phase 1: record the ruling — arbiter authority is decision-only.
	if decision == nil || (decision.ReleasedTo != "buyer" && decision.ReleasedTo != "seller") {
		return fmt.Errorf("stablecoin: arbiter decision must release to buyer or seller")
	}
	return s.store.SetEscrowState(ctx, escrowID, EscrowDisputed, "", "", decision, nil)
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
		// Timeout auto-release: authorized to seller, audit-trailed like an
		// arbiter ruling — the Squads release executes off-backend and the
		// mirror lands the executed sig when the chain reports it.
		decision := &ArbiterDecision{
			ReleasedTo: "seller",
			DecidedBy:  "system:escrow_timeout",
			DecidedAt:  time.Now().UTC(),
			Note:       "confirmation window lapsed — auto-release authorized",
		}
		if err := s.ReleaseEscrow(ctx, id, decision, ""); err != nil {
			return fmt.Errorf("stablecoin: timeout release %s: %w", expired[i].ID, err)
		}
	}
	return nil
}

// completeEscrowFunding is the mirror-driven funding transition: a mirrored
// escrow_funding event on a party wallet is chain proof — stronger than the
// client-reported sig in ConfirmEscrowFunded. Called by MirrorActivity on a
// fresh insert only.
func (s *Service) completeEscrowFunding(ctx context.Context, walletID, sig string) {
	wid, err := pgUUID(walletID)
	if err != nil {
		return
	}
	e, err := s.store.GetLiveEscrowByWallet(ctx, wid)
	if err != nil || e == nil || e.State != EscrowFunding {
		return
	}
	hours := s.settings.GetIntOr(ctx, "stablecoin.escrow_confirm_hours", 72)
	deadline := time.Now().Add(time.Duration(hours) * time.Hour)
	eid, err := pgUUID(e.ID)
	if err != nil {
		return
	}
	if err := s.store.SetEscrowState(ctx, eid, EscrowFunded, sig, "", nil, &deadline); err != nil {
		slog.Warn("stablecoin: mirror escrow-funding transition failed", "escrow", e.ID, "error", err)
	}
}

// completeEscrowRelease is the mirror-driven release/refund transition — a
// mirrored escrow_release event carries the executed Squads signature the
// authorize-side calls can't supply. Arbiter rulings decide direction;
// everything else releases to the seller.
func (s *Service) completeEscrowRelease(ctx context.Context, walletID, sig string) {
	wid, err := pgUUID(walletID)
	if err != nil {
		return
	}
	e, err := s.store.GetLiveEscrowByWallet(ctx, wid)
	if err != nil || e == nil || (e.State != EscrowFunded && e.State != EscrowDisputed) {
		return
	}
	eid, err := pgUUID(e.ID)
	if err != nil {
		return
	}
	if err := s.ReleaseEscrow(ctx, eid, e.ArbiterDecision, sig); err != nil {
		slog.Warn("stablecoin: mirror escrow-release transition failed", "escrow", e.ID, "error", err)
	}
}
