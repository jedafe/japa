// com.atproto.server.* XRPC handlers (§127 A6-3/A6-4).
//
// Same XRPC error contract as the sync surface — {"error","message"}
// at the lexicon's HTTP code — never the /api/v1 envelope.
// Authentication: Bearer access JWT for reads/ops, Bearer refresh JWT
// for refreshSession/deleteSession.
package atproto

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	comatproto "github.com/bluesky-social/indigo/api/atproto"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/clientip"
	gen "github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// xrpcAuth maps the auth sentinel to the lexicon names external
// clients branch on. AuthenticationRequired doubles as the
// bad-credentials answer (no account-existence oracle).
func xrpcAuthErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAuthRequired):
		xrpcError(w, http.StatusUnauthorized, "AuthenticationRequired", "authentication required")
	case errors.Is(err, ErrNotFound):
		xrpcError(w, http.StatusBadRequest, "AccountNotFound", "account not found")
	case errors.Is(err, ErrAdminDisabled):
		xrpcError(w, http.StatusBadRequest, "AccountDeactivated", "account was deactivated by an administrator")
	case errors.Is(err, ErrHandleTaken):
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "name already in use")
	case errors.Is(err, ErrInvalidID), errors.Is(err, ErrInvalidHandle):
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid parameter")
	default:
		xrpcError(w, http.StatusInternalServerError, "InternalError", "internal error")
	}
}

// requireSession resolves the Bearer access JWT → SessionAccount or
// writes the lexicon error. Shared by every authed endpoint.
func (h *Handlers) requireSession(w http.ResponseWriter, r *http.Request) *SessionAccount {
	acct, err := h.service.AuthenticateAccess(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		xrpcAuthErr(w, err)
		return nil
	}
	return acct
}

// sessionFields fills the lexicon {did, handle, email?, active?,
// status?} every session-shaped response carries.
func sessionFields(acct *SessionAccount) (active *bool, status *string, email *string) {
	if acct.Email != "" {
		email = &acct.Email
	}
	a, st := acct.activeStatus()
	if !a {
		active = &a
		status = &st
	}
	return active, status, email
}

// CreateSession serves POST /xrpc/com.atproto.server.createSession —
// app-password login. Rate-limited 'tight' at the router (the bcrypt
// verify is deliberately expensive already).
func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	var input comatproto.ServerCreateSession_Input
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid JSON body")
		return
	}
	acct, access, refresh, err := h.service.CreateSession(r.Context(),
		input.Identifier, input.Password, r.UserAgent(), clientip.FromRequest(r))
	if err != nil {
		xrpcAuthErr(w, err)
		return
	}
	active, status, email := sessionFields(acct)
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.ServerCreateSession_Output{
		AccessJwt:  access,
		RefreshJwt: refresh,
		Did:        acct.DID,
		Handle:     acct.Handle,
		Email:      email,
		Active:     active,
		Status:     status,
	})
}

// RefreshSession serves POST /xrpc/com.atproto.server.refreshSession —
// Bearer REFRESH token; rotates the pair (single-use).
func (h *Handlers) RefreshSession(w http.ResponseWriter, r *http.Request) {
	acct, access, refresh, err := h.service.RefreshSession(r.Context(), r.Header.Get("Authorization"))
	if err != nil {
		xrpcAuthErr(w, err)
		return
	}
	active, status, email := sessionFields(acct)
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.ServerRefreshSession_Output{
		AccessJwt:  access,
		RefreshJwt: refresh,
		Did:        acct.DID,
		Handle:     acct.Handle,
		Email:      email,
		Active:     active,
		Status:     status,
	})
}

// DeleteSession serves POST /xrpc/com.atproto.server.deleteSession —
// Bearer REFRESH token; revokes the session row (idempotent).
func (h *Handlers) DeleteSession(w http.ResponseWriter, r *http.Request) {
	if err := h.service.DeleteSession(r.Context(), r.Header.Get("Authorization")); err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", map[string]any{})
}

// GetSession serves GET /xrpc/com.atproto.server.getSession — Bearer
// access token; the client's "who am I" probe after login/refresh.
func (h *Handlers) GetSession(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	active, status, email := sessionFields(acct)
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.ServerGetSession_Output{
		Did:    acct.DID,
		Handle: acct.Handle,
		Email:  email,
		Active: active,
		Status: status,
	})
}

// CreateAppPasswordXRPC serves POST /xrpc/com.atproto.server.
// createAppPassword — REFUSED on this PDS. Every external session is
// an app-password session — non-privileged in the lexicon model —
// and minting a fresh credential from one would let a stolen password
// spawn credentials that outlive its own revocation. The member API
// (/api/v1/me/atproto/app-passwords, JapaConnect auth) is the only
// mint path; list/revoke below still work for the session holder.
func (h *Handlers) CreateAppPasswordXRPC(w http.ResponseWriter, r *http.Request) {
	if h.requireSession(w, r) == nil {
		return
	}
	xrpcError(w, http.StatusBadRequest, "InvalidRequest",
		"app passwords are managed in JapaConnect account settings")
}

// ListAppPasswords serves GET /xrpc/com.atproto.server.listAppPasswords.
func (h *Handlers) ListAppPasswords(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	views, err := h.service.ListAppPasswords(r.Context(), acct.AccountID)
	if err != nil {
		xrpcAuthErr(w, err)
		return
	}
	out := &comatproto.ServerListAppPasswords_Output{
		Passwords: make([]*comatproto.ServerListAppPasswords_AppPassword, 0, len(views)),
	}
	for i := range views {
		out.Passwords = append(out.Passwords, &comatproto.ServerListAppPasswords_AppPassword{
			Name:      views[i].Name,
			CreatedAt: views[i].CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		})
	}
	writeTypedJSON(w, http.StatusOK, "application/json", out)
}

// RevokeAppPassword serves POST /xrpc/com.atproto.server.revokeAppPassword.
func (h *Handlers) RevokeAppPassword(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	var input comatproto.ServerRevokeAppPassword_Input
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid JSON body")
		return
	}
	if err := h.service.RevokeAppPasswordByName(r.Context(), acct.AccountID, input.Name); err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", map[string]any{})
}

// ---------------------------------------------------------------------------
// A6-3 portability endpoints (access-JWT authed)
// ---------------------------------------------------------------------------

// GetServiceAuth serves GET /xrpc/com.atproto.server.getServiceAuth —
// the ES256 credential migration tooling presents to the destination
// PDS's importRepo.
func (h *Handlers) GetServiceAuth(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	q := r.URL.Query()
	var exp int64
	if raw := q.Get("exp"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			xrpcError(w, http.StatusBadRequest, "InvalidRequest", "invalid exp")
			return
		}
		exp = n
	}
	token, err := h.service.ServiceAuth(r.Context(), acct, q.Get("aud"), q.Get("lxm"), exp)
	if err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.ServerGetServiceAuth_Output{Token: token})
}

// CheckAccountStatus serves GET /xrpc/com.atproto.server.
// checkAccountStatus — migration tooling's source-side diff.
func (h *Handlers) CheckAccountStatus(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	stats, validDid, err := h.service.AccountStatusStats(r.Context(), acct)
	if err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", &comatproto.ServerCheckAccountStatus_Output{
		Activated:          stats.AccountStatus == AccountStatusActive,
		ValidDid:           validDid,
		RepoCommit:         stats.RepoCommit,
		RepoRev:            stats.RepoRev,
		RepoBlocks:         stats.RepoBlocks,
		IndexedRecords:     stats.IndexedRecords,
		ExpectedBlobs:      stats.ImportedBlobs,
		ImportedBlobs:      stats.ImportedBlobs,
		PrivateStateValues: 0,
	})
}

// ActivateAccount serves POST /xrpc/com.atproto.server.activateAccount.
func (h *Handlers) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	if err := h.service.ActivateAccount(r.Context(), acct); err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", map[string]any{})
}

// DeactivateAccount serves POST /xrpc/com.atproto.server.
// deactivateAccount — reversible member-semantics deactivation;
// deleteAfter (if sent) is advisory and ignored.
func (h *Handlers) DeactivateAccount(w http.ResponseWriter, r *http.Request) {
	acct := h.requireSession(w, r)
	if acct == nil {
		return
	}
	if err := h.service.DeactivateAccount(r.Context(), acct); err != nil {
		xrpcAuthErr(w, err)
		return
	}
	writeTypedJSON(w, http.StatusOK, "application/json", map[string]any{})
}

// ---------------------------------------------------------------------------
// Member API (/api/v1/me/atproto — JapaConnect session auth)
// ---------------------------------------------------------------------------

// ExportMyRepo serves GET /api/v1/me/atproto/export — the full repo
// CAR as an attachment (the migration/backup artifact; identical
// bytes to public getRepo).
func (h *Handlers) ExportMyRepo(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	car, did, err := h.service.ExportRepo(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	w.Header().Set("Content-Type", carMediaType)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+did+".car\"")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(car)
}

// ListMyAppPasswords serves GET /api/v1/me/atproto/app-passwords.
func (h *Handlers) ListMyAppPasswords(w http.ResponseWriter, r *http.Request) {
	acct, ok := h.myAccount(w, r)
	if !ok {
		return
	}
	views, err := h.service.ListAppPasswords(r.Context(), acct.ID)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": views})
}

// CreateMyAppPassword serves POST /api/v1/me/atproto/app-passwords —
// {name} → the generated secret once.
func (h *Handlers) CreateMyAppPassword(w http.ResponseWriter, r *http.Request) {
	acct, ok := h.myAccount(w, r)
	if !ok {
		return
	}
	var input struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		handleServiceErr(w, ErrInvalidID)
		return
	}
	view, plain, err := h.service.CreateAppPassword(r.Context(), acct.ID, input.Name)
	if err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": map[string]any{
		"id":       view.ID,
		"name":     view.Name,
		"password": plain, // shown once — only the bcrypt persists
	}})
}

// DeleteMyAppPassword serves DELETE /api/v1/me/atproto/app-passwords/{id}.
func (h *Handlers) DeleteMyAppPassword(w http.ResponseWriter, r *http.Request) {
	acct, ok := h.myAccount(w, r)
	if !ok {
		return
	}
	var id pgtype.UUID
	raw := chi.URLParam(r, "id")
	if !idRe.MatchString(raw) || id.Scan(raw) != nil {
		handleServiceErr(w, ErrInvalidID)
		return
	}
	if err := h.service.RevokeAppPassword(r.Context(), acct.ID, id); err != nil {
		handleServiceErr(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"revoked": true}})
}

// myAccount resolves the caller's atproto account for the /me routes.
func (h *Handlers) myAccount(w http.ResponseWriter, r *http.Request) (*gen.AtprotoAccount, bool) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return nil, false
	}
	acct, err := h.service.store.accountByUserID(r.Context(), user.UserID)
	if err != nil {
		handleServiceErr(w, err)
		return nil, false
	}
	// Tombstoned accounts mint nothing — the identity is dead.
	if acct.Status == AccountStatusTombstone {
		handleServiceErr(w, ErrNotFound)
		return nil, false
	}
	return &acct, true
}
