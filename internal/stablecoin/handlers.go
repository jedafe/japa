package stablecoin

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
)

// Handlers exposes the §128 HTTP surface (registered in
// api-routes-catalog.md; wire into server.go at implementation time).
type Handlers struct {
	svc *Service
}

// NewHandlers wires HTTP handlers.
func NewHandlers(svc *Service) *Handlers {
	return &Handlers{svc: svc}
}

// GetWallet handles GET /me/stablecoin-wallet.
func (h *Handlers) GetWallet(w http.ResponseWriter, r *http.Request) {
	userID := pgtype.UUID{} // TODO: from auth context (same pattern as wallet.Handlers)
	sum, err := h.svc.GetWalletSummary(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":{"code":"wallet_unavailable","message":"wallet unavailable","status":502}}`, http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, sum)
}

// ListActivity handles GET /me/wallet/activity — unified feed: onchain
// mirror rows + bonus-ledger entries, credits-denominated.
func (h *Handlers) ListActivity(w http.ResponseWriter, r *http.Request) {
	// TODO(§128.9): merge stablecoin_activity + wallet ledger rows.
	writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "meta": map[string]any{}})
}

// CreateOnrampSession handles POST /me/funding/onramp-session
// (Idempotency-Key required — Rule 9).
func (h *Handlers) CreateOnrampSession(w http.ResponseWriter, r *http.Request) {
	// TODO(§128.4): require Idempotency-Key, resolve member wallet,
	// provider.CreateOnrampSession with WalletAddress = member address.
	http.Error(w, `{"error":{"code":"not_implemented","message":"onramp not implemented","status":501}}`, http.StatusNotImplemented)
}

// CreateWithdrawal handles POST /me/stablecoin-wallet/withdrawals —
// returns the signable op; the member's passkey signs (step_up +
// Idempotency-Key required).
func (h *Handlers) CreateWithdrawal(w http.ResponseWriter, r *http.Request) {
	// TODO(§128.6): require step_up + Idempotency-Key, svc.BuildMemberWithdrawal.
	http.Error(w, `{"error":{"code":"not_implemented","message":"withdrawal not implemented","status":501}}`, http.StatusNotImplemented)
}

// ProviderWebhook handles POST /webhooks/stablecoin/crossmint.
func (h *Handlers) ProviderWebhook(w http.ResponseWriter, r *http.Request) {
	// TODO: read body + signature header → svc.IngestWebhook → 200 fast
	// (fan-out, Rule 8).
	w.WriteHeader(http.StatusOK)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
