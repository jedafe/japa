package atproto

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds dependencies for the atproto HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// handleServiceErr maps service sentinels to the standard error envelope.
func handleServiceErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	case errors.Is(err, ErrInvalidHandle):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_handle", "Invalid handle")
	case errors.Is(err, ErrInvalidID):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid account id")
	case errors.Is(err, ErrHandleTaken):
		errpkg.WriteError(w, http.StatusConflict, "handle_taken", "Handle already claimed")
	case errors.Is(err, ErrMisconfigured):
		slog.Error("atproto misconfigured", "err", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "misconfigured", "AT Protocol is not configured")
	default:
		slog.Error("atproto handler error", "err", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Internal error")
	}
}

// ---------------------------------------------------------------------------
// Public surface (§127 URL Layout — unversioned PDS endpoints)
// ---------------------------------------------------------------------------

// GetAtprotoDID serves GET /.well-known/atproto-did on a member-handle
// subdomain (alice.japa.to). The response is the DID as text/plain —
// the handle→DID proof Bluesky's appview verifies. Byte-exact: any
// deviation makes the account resolve as handle.invalid.
//
// X-Forwarded-Host is honored only from the apex proxy path (the
// x-jc-surface marker the SvelteKit route sets) — a direct API hit with
// a forged forwarded host can't steer handle derivation.
func (h *Handlers) GetAtprotoDID(w http.ResponseWriter, r *http.Request) {
	host := r.Host
	if r.Header.Get("X-Jc-Surface") != "" {
		if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
			host = fwd
		}
	}
	handle := h.service.HandleFromHost(host)
	if handle == "" {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
		return
	}
	did, err := h.service.ResolveHandleDID(r.Context(), handle)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(did))
}

// GetDIDDoc serves GET /atproto/u/{id}/did.json — the did:web
// resolution document (application/did+ld+json). The path id is the
// account's user UUID — permanent, rename- and re-claim-proof.
func (h *Handlers) GetDIDDoc(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.DIDDocument(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/did+ld+json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(doc); err != nil {
		_ = err
	}
}

// ---------------------------------------------------------------------------
// Member (/api/v1/me/atproto — auth + atproto.use in server.go)
// ---------------------------------------------------------------------------

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// GetMyAccount serves GET /api/v1/me/atproto.
func (h *Handlers) GetMyAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.GetMyAccount(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// EnableMyAccount serves POST /api/v1/me/atproto (idempotent opt-in).
func (h *Handlers) EnableMyAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.EnableAccount(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// DisableMyAccount serves DELETE /api/v1/me/atproto.
func (h *Handlers) DisableMyAccount(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	st, err := h.service.DisableAccount(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// ---------------------------------------------------------------------------
// Admin (/api/v1/admin/atproto — auth + atproto.accounts.view)
// ---------------------------------------------------------------------------

// ListAccounts serves GET /api/v1/admin/atproto/accounts.
func (h *Handlers) ListAccounts(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseLimitOffset(r, 20, 100)
	accounts, total, err := h.service.ListAccounts(r.Context(), r.URL.Query().Get("q"), int32(limit), int32(offset))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": accounts,
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset, "total_pages": totalPages(total, int32(limit))},
	})
}

// GetStats serves GET /api/v1/admin/atproto/stats.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, stats)
}

// writeTypedJSON is shared with the federation package's convention:
// public PDS responses don't use the API envelope for 2xx.
func writeTypedJSON(w http.ResponseWriter, status int, contentType string, v any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		_ = err
	}
}

func totalPages(total int64, limit int32) int32 {
	if limit <= 0 || total <= 0 {
		return 1
	}
	return int32((total + int64(limit) - 1) / int64(limit))
}

func parseLimitOffset(r *http.Request, defLimit, maxLimit int) (int, int) {
	limit := defLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = n
		}
	}
	return limit, offset
}

// RunConformanceCheck serves POST /api/v1/admin/atproto/conformance —
// the §127 release-gate readback from the admin UI. {handle?, did?}
// picks the target; an empty body auto-picks an active account. The
// run persists to atproto_conformance_runs either way — history exists
// so admins can see when appview indexing last broke, and a failing
// run must not vanish just because it failed.
func (h *Handlers) RunConformanceCheck(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	var input struct {
		Handle string `json:"handle"`
		DID    string `json:"did"`
	}
	// Empty body is legal (auto-pick) — a malformed body is not.
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
			return
		}
	}
	run, res, err := h.service.RunConformanceCheck(r.Context(), input.Handle, input.DID, user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"run": run, "result": res})
}

// ListConformanceRuns serves GET /api/v1/admin/atproto/conformance.
func (h *Handlers) ListConformanceRuns(w http.ResponseWriter, r *http.Request) {
	limit, offset := parseLimitOffset(r, 20, 100)
	runs, total, err := h.service.ListConformanceRuns(r.Context(), int32(limit), int32(offset))
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": runs,
		"meta": map[string]any{"total": total, "page_size": limit, "page_offset": offset, "total_pages": totalPages(total, int32(limit))},
	})
}
