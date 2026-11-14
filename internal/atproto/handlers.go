package atproto

import (
	"encoding/json"
	"errors"
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
	default:
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
// X-Forwarded-Host wins over Host — the apex SvelteKit proxy forwards
// the original subdomain that way.
func (h *Handlers) GetAtprotoDID(w http.ResponseWriter, r *http.Request) {
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
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

// GetDIDDoc serves GET /atproto/u/{username}/did.json — the did:web
// resolution document (application/did+ld+json).
func (h *Handlers) GetDIDDoc(w http.ResponseWriter, r *http.Request) {
	doc, err := h.service.DIDDocument(r.Context(), chi.URLParam(r, "username"))
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
	writeTypedJSON(w, http.StatusOK, "application/json", st)
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
	writeTypedJSON(w, http.StatusOK, "application/json", st)
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
	writeTypedJSON(w, http.StatusOK, "application/json", st)
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
	writeTypedJSON(w, http.StatusOK, "application/json", map[string]any{
		"accounts": accounts,
		"total":    total,
	})
}

// GetStats serves GET /api/v1/admin/atproto/stats.
func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.service.Stats(r.Context())
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", stats)
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
