package stablecoin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the §128 HTTP surface (routes per api-routes-catalog).
type Handlers struct {
	svc *Service
}

// NewHandlers wires HTTP handlers.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// handleError maps domain sentinels to the Rule 5 error format.
func handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotImplemented):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "feature_unavailable", "Stablecoin wallet is not enabled yet")
	case errors.Is(err, ErrWalletNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "wallet_not_found", "No stablecoin wallet provisioned")
	case errors.Is(err, ErrKillswitch):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "transfers_halted", "Transfers are temporarily paused")
	case errors.Is(err, ErrScopeExceeded):
		errpkg.WriteError(w, http.StatusTooManyRequests, "daily_limit_reached", "Daily transfer limit reached")
	case errors.Is(err, ErrWalletNotSettleable):
		errpkg.WriteError(w, http.StatusConflict, "wallet_not_active", "Wallet is not fully set up yet")
	case errors.Is(err, ErrEscrowNotFound), errors.Is(err, ErrBookingNotFound), errors.Is(err, ErrSweepNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Resource not found")
	case errors.Is(err, ErrEscrowState):
		errpkg.WriteError(w, http.StatusConflict, "invalid_state", "Operation not allowed in the current state")
	case errors.Is(err, ErrEscrowParty):
		errpkg.WriteError(w, http.StatusForbidden, "not_a_party", "Caller is not a party to this escrow")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Operation failed")
	}
}

func pathUUID(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	return id, id.Scan(chi.URLParam(r, param))
}

func parseLimit(r *http.Request) (int32, int32) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("page_size"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	return int32(limit), int32(offset)
}

// GetWallet handles GET /me/stablecoin-wallet.
func (h *Handlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	sum, err := h.svc.GetWalletSummary(r.Context(), user.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	if sum == nil {
		// Not provisioned — empty 200 so the client can offer activation.
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": nil})
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sum})
}

// ListActivity handles GET /me/stablecoin-wallet/activity — unified feed:
// onchain mirror rows + bonus-ledger entries, credits-denominated (§128.9).
func (h *Handlers) ListActivity(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	limit, offset := parseLimit(r)
	feed, err := h.svc.ListActivity(r.Context(), user.UserID, limit, offset)
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": feed})
}

// CreateOnrampSession handles POST /me/funding/onramp-session
// (Idempotency-Key required — Rule 9, enforced by middleware).
func (h *Handlers) CreateOnrampSession(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Asset        string `json:"asset"`
		FiatCurrency string `json:"fiat_currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	wallet, err := h.svc.store.GetWalletByUser(r.Context(), user.UserID)
	if err != nil {
		handleError(w, err)
		return
	}
	if wallet == nil {
		errpkg.WriteError(w, http.StatusNotFound, "wallet_not_found", "Stablecoin wallet not provisioned")
		return
	}
	if req.Asset != "" && !slices.Contains(h.svc.assetList(r.Context()), req.Asset) {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_asset", "Asset not supported")
		return
	}
	sess, err := h.svc.provider.CreateOnrampSession(r.Context(), OnrampInput{
		WalletAddress: wallet.Address,
		Asset:         req.Asset,
		FiatCurrency:  req.FiatCurrency,
		Ref:           "onramp:" + user.UserID.String(),
	})
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sess})
}

// CreateWithdrawal handles POST /me/stablecoin-wallet/withdrawals —
// returns the signable op; the member's passkey signs (step_up +
// Idempotency-Key required).
func (h *Handlers) CreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		Asset       string `json:"asset"`
		Destination string `json:"destination"`
		AmountMinor int64  `json:"amount_minor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if req.AmountMinor <= 0 || req.Destination == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_amount", "Amount and destination are required")
		return
	}
	op, err := h.svc.BuildMemberWithdrawal(r.Context(), user.UserID, WithdrawalInput{
		Asset:       req.Asset,
		Destination: req.Destination,
		AmountMinor: req.AmountMinor,
	})
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": op})
}

// ProviderWebhook handles POST /webhooks/stablecoin/crossmint — verify,
// dedupe, fan-out; always 200 fast on accepted events (Rule 8).
func (h *Handlers) ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Could not read body")
		return
	}
	sig := r.Header.Get("X-Webhook-Signature")
	if sig == "" {
		sig = r.Header.Get("Crossmint-Signature")
	}
	if err := h.svc.IngestWebhook(r.Context(), body, sig); err != nil {
		if errors.Is(err, ErrWebhookAuth) {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_signature", "Webhook signature verification failed")
			return
		}
		// Internal failure — 500 so the provider retries rather than
		// treating the event as permanently rejected.
		errpkg.WriteError(w, http.StatusInternalServerError, "ingest_failed", "Webhook ingestion failed")
		return
	}
	w.WriteHeader(http.StatusOK)
}

// ── Escrow (§128.5) ─────────────────────────────────────────────────────

// PrepareEscrow handles POST /me/escrows — resolves booking parties and
// returns the Squads vault-creation parameters (client derives the PDA).
func (h *Handlers) PrepareEscrow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var req struct {
		BookingID string `json:"booking_id"`
		Asset     string `json:"asset"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	var bookingID pgtype.UUID
	if err := bookingID.Scan(req.BookingID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_booking", "Invalid booking id")
		return
	}
	setup, err := h.svc.PrepareEscrow(r.Context(), bookingID, user.UserID, req.Asset)
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": setup})
}

// RegisterVault handles POST /me/escrows/{id}/vault — records the
// client-derived Squads PDA after onchain vault creation.
func (h *Handlers) RegisterVault(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid escrow id")
		return
	}
	var req struct {
		VaultAddress string `json:"vault_address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.VaultAddress == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "vault_address is required")
		return
	}
	if err := h.svc.RegisterEscrowVault(r.Context(), id, user.UserID, req.VaultAddress); err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"state": EscrowFunding}})
}

// ConfirmFunded handles POST /me/escrows/{id}/funded — marks the vault
// funded after the buyer's passkey-signed funding tx lands.
func (h *Handlers) ConfirmFunded(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid escrow id")
		return
	}
	var req struct {
		TxSignature string `json:"tx_signature"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.svc.ConfirmEscrowFunded(r.Context(), id, user.UserID, req.TxSignature); err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"state": EscrowFunded}})
}

// ConfirmRelease handles POST /me/escrows/{id}/confirm-release — buyer
// delivery confirmation; releases seller share + platform fee.
func (h *Handlers) ConfirmRelease(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid escrow id")
		return
	}
	var req struct {
		ReleaseSig string `json:"release_sig"` // executed Squads proposal sig, when known
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.svc.ConfirmRelease(r.Context(), id, user.UserID, req.ReleaseSig); err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"state": EscrowReleased}})
}

// DisputeEscrow handles POST /me/escrows/{id}/dispute — freezes a funded
// escrow for arbiter review; either party may open it.
func (h *Handlers) DisputeEscrow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid escrow id")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := h.svc.DisputeEscrow(r.Context(), id, user.UserID, req.Reason); err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"state": EscrowDisputed}})
}

// ── Admin (§128 routes — seeded nodes) ─────────────────────────────────

// AdminListTreasury handles GET /admin/treasury/wallets — treasury.view.
func (h *Handlers) AdminListTreasury(w http.ResponseWriter, r *http.Request) {
	view, err := h.svc.ListTreasury(r.Context())
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": view})
}

// AdminListWallets handles GET /admin/stablecoin/wallets —
// stablecoin.wallet.view; ?status= + ?q= filters per catalog.
func (h *Handlers) AdminListWallets(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseLimit(r)
	rows, err := h.svc.ListWallets(r.Context(), r.URL.Query().Get("status"), r.URL.Query().Get("q"), limit, offset)
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// AdminListSweeps handles GET /admin/treasury/sweeps — treasury.view;
// ?state= filter.
func (h *Handlers) AdminListSweeps(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseLimit(r)
	rows, err := h.svc.ListSweeps(r.Context(), r.URL.Query().Get("state"), limit, offset)
	if err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": rows})
}

// AdminKillswitch handles POST /admin/stablecoin/signer-killswitch —
// super_admin-gated per catalog; body {engaged: bool}.
func (h *Handlers) AdminKillswitch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Engaged bool `json:"engaged"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	if err := h.svc.SetKillswitch(r.Context(), req.Engaged); err != nil {
		handleError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]bool{"engaged": req.Engaged}})
}

// AdminDecideSweep handles POST /admin/treasury/sweeps/{id}/approve —
// treasury.sweep_approve gated; body carries approve|reject + optional
// squads_proposal_id.
func (h *Handlers) AdminDecideSweep(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sweep id")
		return
	}
	var req struct {
		Decision         string `json:"decision"` // approve | reject
		SquadsProposalID string `json:"squads_proposal_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	switch req.Decision {
	case "approve":
		err = h.svc.ApproveSweep(r.Context(), id, user.UserID, req.SquadsProposalID)
	case "reject":
		err = h.svc.RejectSweep(r.Context(), id, user.UserID)
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_decision", "decision must be approve or reject")
		return
	}
	if err != nil {
		handleError(w, err)
		return
	}
	state := "approved"
	if req.Decision == "reject" {
		state = "rejected"
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"state": state}})
}

// AdminArbitrateEscrow handles POST /admin/escrows/{id}/arbitrate —
// escrow.arbitrate gated. Two phases on one route: the ruling call records
// the decision (escrow stays disputed), then a call carrying release_sig
// completes the transition once the Squads ceremony executes.
func (h *Handlers) AdminArbitrateEscrow(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := pathUUID(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid escrow id")
		return
	}
	var req struct {
		ReleasedTo string `json:"released_to"` // buyer | seller
		Note       string `json:"note"`
		ReleaseSig string `json:"release_sig"` // phase 2: executed Squads tx sig
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	var decision *ArbiterDecision
	if req.ReleasedTo != "" {
		decision = &ArbiterDecision{
			ReleasedTo: req.ReleasedTo,
			DecidedBy:  user.UserID.String(),
			DecidedAt:  time.Now().UTC(),
			Note:       req.Note,
		}
	}
	if err := h.svc.ArbitrateEscrow(r.Context(), id, decision, req.ReleaseSig); err != nil {
		handleError(w, err)
		return
	}
	state := EscrowDisputed
	if req.ReleaseSig != "" {
		state = EscrowReleased // or refunded — phase-2 completes the transition
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"released_to": req.ReleasedTo, "state": state}})
}
