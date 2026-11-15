// A6-5 full ATProto OAuth — HTTP handlers (§127 unified AS).
//
// These handlers share the root /oauth/* surface with the §120 MCP
// OAuth server; server.go dispatches by request shape:
//   - /oauth/authorize  — request_uri present → this flow
//   - /oauth/token      — client_id is https://…|http://localhost → this flow
//   - /oauth/revoke     — atp_at_/atp_rt_ token prefix → this flow
//   - /oauth/par        — atproto-only
//   - /.well-known/oauth-protected-resource — PDS host → this doc
//   - /.well-known/oauth-authorization-server — merged doc (both profiles)
package atproto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	gen "github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	valkeycompat "github.com/valkey-io/valkey-go/valkeycompat"
)

const maxOAuthFormBytes = 8 << 10

// IsAtprotoClientID is the /oauth/token discriminator — an atproto
// client_id is a metadata-document URL; §120 client_ids are "jcmc_" ids.
func IsAtprotoClientID(clientID string) bool {
	return strings.HasPrefix(clientID, "https://") || strings.HasPrefix(clientID, "http://localhost")
}

// IsAtprotoToken is the /oauth/revoke discriminator — our opaque grant
// tokens carry the atp_ prefix; §120 tokens are jcpat_/jcprt_.
func IsAtprotoToken(tok string) bool {
	return strings.HasPrefix(tok, oauthAccessTokenPrefix) ||
		strings.HasPrefix(tok, oauthRefreshTokenPrefix)
}

// SetConsentAuth wires the member-session resolver the consent flow
// shares with §120 (jc_token via KeyRotation + jti revocation check).
// siteURL/siteName feed the login redirect and page chrome.
func (h *Handlers) SetConsentAuth(kr *auth.KeyRotation, vk valkeycompat.Cmdable, siteURL, siteName string) {
	h.kr = kr
	h.vk = vk
	h.siteURL = siteURL
	h.siteName = siteName
}

// ---------------------------------------------------------------------------
// Small response helpers (RFC 6749 wire shape — not the platform envelope)
// ---------------------------------------------------------------------------

func writeOAuthErr(w http.ResponseWriter, status int, code, desc string) {
	writeOAuthJSONDoc(w, status, map[string]any{
		"error":             code,
		"error_description": desc,
	})
}

func writeOAuthJSONDoc(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("atproto oauth: encode response", "error", err)
	}
}

// oauthErrStatus maps the sentinel to the wire: nonce failures are 400
// at AS endpoints (RFC 9449 §8 — 401 is the RS-endpoint shape, applied
// by AuthenticateDPoP via WWW-Authenticate instead).
func oauthErrStatus(err error) int {
	switch {
	case errors.Is(err, ErrOAuthNonce), errors.Is(err, ErrOAuthDPoP),
		errors.Is(err, ErrOAuthInvalidClient), errors.Is(err, ErrOAuthInvalidGrant),
		errors.Is(err, ErrOAuthInvalidScope), errors.Is(err, ErrOAuthInvalidRequest):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func (h *Handlers) writeOAuthServiceErr(w http.ResponseWriter, err error) {
	var oe *oauthError
	if errors.As(err, &oe) {
		writeOAuthErr(w, oauthErrStatus(err), oe.code, oe.desc)
		return
	}
	slog.Error("atproto oauth: internal error", "error", err)
	writeOAuthErr(w, http.StatusInternalServerError, "server_error", "internal error")
}

// issuerOrigin returns scheme://host for the request (no path).
func issuerOrigin(r *http.Request) string {
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p == "http" || p == "https" {
		scheme = p
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if hh := r.Header.Get("X-Forwarded-Host"); hh != "" {
		host = hh
	}
	return scheme + "://" + host
}

// ---------------------------------------------------------------------------
// PAR — POST /oauth/par
// ---------------------------------------------------------------------------

// HandlePAR is the pushed-authorization-request endpoint: full up-front
// validation (client metadata, redirect, scope, PKCE, DPoP binding)
// returns a single-use request_uri the authorize step consumes.
func (h *Handlers) HandlePAR(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !h.service.featureOn(r.Context()) {
		writeOAuthErr(w, http.StatusNotFound, "invalid_request", "not found")
		return
	}

	proof, err := h.service.verifyDPoP(r.Context(), r)
	h.service.setNonceHeader(w)
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	if err := h.service.checkNonce(proof); err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "request must be application/x-www-form-urlencoded")
		return
	}
	f := r.PostForm

	clientID := f.Get("client_id")
	doc, err := h.service.fetchClientDoc(r.Context(), clientID)
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	if !doc.DPoPBoundAccessTokens {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_client_metadata", "client metadata must set dpop_bound_access_tokens")
		return
	}

	// Confidential clients authenticate on every endpoint call.
	clientJKT := ""
	if f.Get("client_assertion") != "" || f.Get("client_assertion_type") != "" || doc.confidential() {
		clientJKT, err = h.service.verifyClientAssertion(r.Context(), doc, issuerOrigin(r),
			f.Get("client_assertion_type"), f.Get("client_assertion"))
		if err != nil {
			h.writeOAuthServiceErr(w, err)
			return
		}
	}

	if f.Get("response_type") != "code" {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", `response_type must be "code"`)
		return
	}
	redirectURI := f.Get("redirect_uri")
	if err := validateRedirectURI(doc, clientID, redirectURI); err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	scope := f.Get("scope")
	if !hasScopeWord(scope, "atproto") {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_scope", "the atproto scope is required")
		return
	}
	if !scopeAllowed(doc, scope) {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_scope", "scope exceeds the client's declared scope")
		return
	}
	state := f.Get("state")
	if state == "" || len(state) > 512 {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "state is required")
		return
	}
	challenge := f.Get("code_challenge")
	if f.Get("code_challenge_method") != "S256" || !isS256Challenge(challenge) {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "code_challenge must be S256")
		return
	}
	loginHint := f.Get("login_hint")
	if len(loginHint) > 253 {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "login_hint is too long")
		return
	}

	requestURI := oauthRequestURIPrefix + oauthRandom("req_")
	row, err := h.service.CreateOAuthRequest(r.Context(), &gen.CreateOAuthRequestParams{
		RequestUri:    requestURI,
		ClientID:      clientID,
		ClientJkt:     strPtr(clientJKT),
		RedirectUri:   redirectURI,
		Scope:         scope,
		State:         state,
		CodeChallenge: challenge,
		DpopJkt:       proof.jkt,
		LoginHint:     strPtr(loginHint),
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(oauthPARTTL), Valid: true},
	})
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	_ = row
	writeOAuthJSONDoc(w, http.StatusCreated, map[string]any{
		"request_uri": requestURI,
		"expires_in":  int(oauthPARTTL / time.Second),
	})
}

// isS256Challenge — a base64url sha256 is exactly 43 chars with no
// padding; anything else can't be a real challenge.
func isS256Challenge(c string) bool {
	if len(c) != 43 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(c)
	return err == nil
}

// ---------------------------------------------------------------------------
// Authorize + consent — GET/POST /oauth/authorize (request_uri present)
// ---------------------------------------------------------------------------

// HandleAuthorize serves the atproto authorization endpoint when the
// request carries request_uri. GET renders consent (or bounces to
// login); POST takes the signed decision and redirects to the client.
func (h *Handlers) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if !h.service.featureOn(r.Context()) {
		xrpcError(w, http.StatusNotFound, "InvalidRequest", "not found")
		return
	}
	var q url.Values
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
		if err := r.ParseForm(); err != nil {
			h.renderOAuthError(w, "invalid request body")
			return
		}
		q = r.PostForm
	} else if r.Method == http.MethodGet {
		q = r.URL.Query()
	} else {
		w.Header().Set("Allow", "GET, POST")
		h.renderOAuthError(w, "method not allowed")
		return
	}

	requestURI := q.Get("request_uri")
	clientID := q.Get("client_id")
	row, err := h.service.store.q.GetOAuthRequest(r.Context(), requestURI)
	if err != nil || row.ClientID != clientID ||
		row.ConsumedAt.Valid || time.Now().After(row.ExpiresAt.Time) {
		h.renderOAuthError(w, "This authorization request is expired or invalid. Please restart sign-in from the app.")
		return
	}

	user := h.authorizeUser(r)
	if user == nil {
		if r.Method == http.MethodGet {
			h.redirectToLogin(w, r)
			return
		}
		h.renderOAuthError(w, "Your session has expired. Please restart sign-in from the app.")
		return
	}

	account, err := h.service.store.accountByUserID(r.Context(), user.UserID)
	if err != nil {
		h.renderOAuthError(w, "Your account has no ATProto identity — enable it in your JapaConnect settings first.")
		return
	}

	// login_hint binding: when the hint resolves to a real account it
	// must be the signed-in member's — a hint naming someone else's
	// account is a refusal, not a switch.
	if row.LoginHint != nil && *row.LoginHint != "" {
		hint := strings.TrimPrefix(strings.TrimSpace(*row.LoginHint), "@")
		if hintAcct, herr := h.service.store.q.GetAtprotoSessionAccount(r.Context(), hint); herr == nil && hintAcct.ID != account.ID {
			h.renderOAuthError(w, "This sign-in was started for a different account. Sign in as "+hint+" or restart from the app.")
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		h.renderOAuthConsent(w, r, &row, &account, user)
	case http.MethodPost:
		h.handleOAuthConsentDecision(w, r, &row, &account, user, q)
	}
}

// authorizeUser resolves the member from Bearer JWT or the jc_token
// cookie — identical to the §120 resolver.
func (h *Handlers) authorizeUser(r *http.Request) *auth.AuthenticatedUser {
	token := ""
	if ah := r.Header.Get("Authorization"); ah != "" {
		parts := strings.SplitN(ah, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
		}
	}
	if token == "" {
		if c, err := r.Cookie("jc_token"); err == nil {
			token = c.Value
		}
	}
	if token == "" || h.kr == nil {
		return nil
	}
	claims, err := h.kr.ValidateAccessToken(token)
	if err != nil {
		return nil
	}
	if auth.IsTokenRevoked(r.Context(), claims.JTI, h.vk) {
		return nil
	}
	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		return nil
	}
	return &auth.AuthenticatedUser{UserID: userID, SessionID: claims.JTI}
}

// redirectToLogin mirrors §120: site login carries the absolute
// authorize URL back when the request arrived on the API host; a
// site-host request stays relative so the cookie domain matches.
func (h *Handlers) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := issuerOrigin(r) + r.URL.RequestURI()
	if u, err := url.Parse(h.siteURL); err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
		target = r.URL.RequestURI()
	}
	http.Redirect(w, r, h.siteURL+"/login?redirect="+url.QueryEscape(target), http.StatusFound)
}

// handleOAuthConsentDecision processes the consent POST: verify the
// consent token, mint a code on approve, redirect back to the client.
func (h *Handlers) handleOAuthConsentDecision(w http.ResponseWriter, r *http.Request, row *gen.AtprotoOauthRequest, account *gen.AtprotoAccount, user *auth.AuthenticatedUser, q url.Values) {
	token := q.Get("consent_token")
	if !h.service.verifyConsentToken(token, row.RequestUri, user.UserID) {
		h.renderOAuthError(w, "The consent form has expired or is invalid. Please restart sign-in from the app.")
		return
	}
	if q.Get("action") != "approve" {
		_, _ = h.service.store.q.DenyOAuthRequest(r.Context(), row.RequestUri)
		h.redirectOAuthError(w, r, row, "access_denied", "The user denied the request")
		return
	}

	code := oauthRandom("atp_cd_")
	ch := oauthHash(code)
	issued, err := h.service.store.q.IssueOAuthCode(r.Context(), gen.IssueOAuthCodeParams{
		RequestUri: row.RequestUri,
		AccountID:  account.ID,
		CodeHash:   &ch,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.renderOAuthError(w, "This authorization request was already used. Please restart sign-in from the app.")
			return
		}
		slog.Error("atproto oauth: issue code", "error", err)
		h.renderOAuthError(w, "Failed to issue an authorization code. Please try again.")
		return
	}
	_ = issued
	target, perr := url.Parse(row.RedirectUri)
	if perr != nil {
		h.renderOAuthError(w, "The client redirect URI is malformed.")
		return
	}
	qq := target.Query()
	qq.Set("code", code)
	qq.Set("state", row.State)
	qq.Set("iss", issuerOrigin(r))
	target.RawQuery = qq.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound) // #nosec G710 -- redirect_uri was proven against the client metadata at PAR
}

// redirectOAuthError sends the RFC 6749 error back through the client —
// the redirect_uri+state on the PAR row were validated at PAR time.
func (h *Handlers) redirectOAuthError(w http.ResponseWriter, r *http.Request, row *gen.AtprotoOauthRequest, code, desc string) {
	target, err := url.Parse(row.RedirectUri)
	if err != nil {
		h.renderOAuthError(w, desc)
		return
	}
	qq := target.Query()
	qq.Set("error", code)
	qq.Set("error_description", desc)
	qq.Set("state", row.State)
	qq.Set("iss", issuerOrigin(r))
	target.RawQuery = qq.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound) // #nosec G710 -- redirect_uri was proven against the client metadata at PAR
}

// ---------------------------------------------------------------------------
// Consent page
// ---------------------------------------------------------------------------

var oauthScopeLabels = map[string][2]string{
	"atproto":              {"Use the AT Protocol", "Sign in to your account on this server"},
	"transition:generic":   {"Full account access", "Read and act on your behalf across the network (legacy transition scope)"},
	"transition:chat.bsky": {"Chat access", "Read and send direct messages via Bluesky chat"},
	"transition:email":     {"See your email", "Learn the email address on your account"},
}

func oauthScopeDescriptions(scope string) [][2]string {
	var out [][2]string
	for _, s := range strings.Fields(scope) {
		if l, ok := oauthScopeLabels[s]; ok {
			out = append(out, l)
		} else {
			out = append(out, [2]string{s, ""})
		}
	}
	return out
}

func (h *Handlers) renderOAuthConsent(w http.ResponseWriter, r *http.Request, row *gen.AtprotoOauthRequest, account *gen.AtprotoAccount, user *auth.AuthenticatedUser) {
	siteName := h.siteName
	if siteName == "" {
		siteName = "JapaConnect"
	}
	clientHost := row.ClientID
	if u, err := url.Parse(row.ClientID); err == nil && u.Host != "" {
		clientHost = u.Host
	}
	consentToken := h.service.consentTokenValue(row.RequestUri, user.UserID, time.Now().Add(oauthConsentTokenTTL))
	scopes := oauthScopeDescriptions(row.Scope)

	hidden := func(k, v string) string {
		return `<input type="hidden" name="` + html.EscapeString(k) + `" value="` + html.EscapeString(v) + `">`
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>Authorize — ` + html.EscapeString(siteName) + `</title>`)
	b.WriteString(`<style>
body{font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;justify-content:center;padding:2rem;margin:0}
.card{background:#1e293b;border:1px solid #334155;border-radius:12px;max-width:28rem;width:100%;padding:2rem}
h1{font-size:1.25rem;margin:0 0 .5rem;word-break:break-all}
.muted{color:#94a3b8;font-size:.9rem}
.acct{background:#0f172a;border:1px solid #334155;border-radius:8px;padding:.6rem .9rem;margin:1rem 0;font-size:.95rem}
ul{list-style:none;padding:0;margin:1rem 0}
li{padding:.4rem 0;border-bottom:1px solid #334155;font-size:.9rem}
li:last-child{border-bottom:none}
.scope{font-weight:600}
.actions{display:flex;gap:.75rem;margin-top:1.5rem}
button{flex:1;padding:.65rem 1rem;border-radius:8px;border:0;font-size:.95rem;cursor:pointer}
.approve{background:#38bdf8;color:#0f172a;font-weight:600}
.deny{background:transparent;color:#e2e8f0;border:1px solid #475569}
</style></head><body><div class="card">`)
	b.WriteString(`<h1>` + html.EscapeString(clientHost) + `</h1>`)
	b.WriteString(`<p class="muted">wants to sign in to your account on ` + html.EscapeString(siteName) + `:</p>`)
	b.WriteString(`<div class="acct"><strong>` + html.EscapeString("@"+account.Handle) + `</strong><br><span class="muted">` + html.EscapeString(account.Did) + `</span></div>`)
	b.WriteString(`<p class="muted">This grants it:</p><ul>`)
	for _, s := range scopes {
		b.WriteString(`<li><span class="scope">` + html.EscapeString(s[0]) + `</span><br><span class="muted">` + html.EscapeString(s[1]) + `</span></li>`)
	}
	b.WriteString(`</ul><form method="post" action="/oauth/authorize">`)
	b.WriteString(hidden("client_id", row.ClientID))
	b.WriteString(hidden("request_uri", row.RequestUri))
	b.WriteString(hidden("consent_token", consentToken))
	b.WriteString(`<div class="actions"><button type="submit" name="action" value="deny" class="deny">Deny</button>`)
	b.WriteString(`<button type="submit" name="action" value="approve" class="approve">Authorize</button></div>`)
	b.WriteString(`</form><p class="muted" style="margin-top:1rem">Access tokens are bound to this application's keys and expire quickly. You can revoke access any time from your account settings.</p>`)
	b.WriteString(`</div></body></html>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

func (h *Handlers) renderOAuthError(w http.ResponseWriter, msg string) {
	siteName := h.siteName
	if siteName == "" {
		siteName = "JapaConnect"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Authorization error — ` +
		html.EscapeString(siteName) + `</title></head><body style="font-family:system-ui;display:flex;justify-content:center;padding:3rem">` +
		`<div><h1>Authorization error</h1><p>` + html.EscapeString(msg) + `</p></div></body></html>`))
}

// ---------------------------------------------------------------------------
// Token — POST /oauth/token (atproto client_id shapes)
// ---------------------------------------------------------------------------

// HandleToken serves authorization_code and refresh_token grants for
// atproto clients. DPoP + client auth re-verify on every call; the grant
// itself enforces the PAR-row bindings (client, redirect, PKCE, jkts).
func (h *Handlers) HandleToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !h.service.featureOn(r.Context()) {
		writeOAuthErr(w, http.StatusNotFound, "invalid_request", "not found")
		return
	}

	proof, err := h.service.verifyDPoP(r.Context(), r)
	h.service.setNonceHeader(w)
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	if err := h.service.checkNonce(proof); err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "request must be application/x-www-form-urlencoded")
		return
	}
	f := r.PostForm

	clientID := f.Get("client_id")
	if clientID == "" {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}
	doc, err := h.service.fetchClientDoc(r.Context(), clientID)
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	clientJKT := ""
	if f.Get("client_assertion") != "" || f.Get("client_assertion_type") != "" || doc.confidential() {
		clientJKT, err = h.service.verifyClientAssertion(r.Context(), doc, issuerOrigin(r),
			f.Get("client_assertion_type"), f.Get("client_assertion"))
		if err != nil {
			h.writeOAuthServiceErr(w, err)
			return
		}
	}

	var resp *oauthTokenResponse
	switch f.Get("grant_type") {
	case "authorization_code":
		resp, err = h.service.ExchangeCode(r.Context(),
			f.Get("code"), clientID, clientJKT,
			f.Get("redirect_uri"), f.Get("code_verifier"), proof.jkt)
	case "refresh_token":
		if len(doc.GrantTypes) > 0 && !hasScopeWord(strings.Join(doc.GrantTypes, " "), "refresh_token") {
			writeOAuthErr(w, http.StatusBadRequest, "unauthorized_client", "client does not declare the refresh_token grant")
			return
		}
		resp, err = h.service.rotateGrant(r.Context(),
			f.Get("refresh_token"), clientID, clientJKT, proof.jkt)
	default:
		writeOAuthErr(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
		return
	}
	if err != nil {
		h.writeOAuthServiceErr(w, err)
		return
	}
	writeOAuthJSONDoc(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Revoke — POST /oauth/revoke (atp_* tokens)
// ---------------------------------------------------------------------------

// HandleRevoke is the atproto branch of RFC 7009 — always 200 on
// well-formed input. Confidential clients must authenticate to revoke;
// the token hash alone is never proof of ownership.
func (h *Handlers) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	if !h.service.featureOn(r.Context()) {
		w.WriteHeader(http.StatusOK)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}
	tok := r.PostForm.Get("token")
	if !IsAtprotoToken(tok) {
		w.WriteHeader(http.StatusOK)
		return
	}
	// Confidential clients authenticate the revocation; public clients
	// revoke by possession (the token hash is the credential).
	if clientID := r.PostForm.Get("client_id"); IsAtprotoClientID(clientID) {
		if doc, err := h.service.fetchClientDoc(r.Context(), clientID); err == nil && doc.confidential() {
			if _, verr := h.service.verifyClientAssertion(r.Context(), doc, issuerOrigin(r),
				r.PostForm.Get("client_assertion_type"), r.PostForm.Get("client_assertion")); verr != nil {
				h.writeOAuthServiceErr(w, verr)
				return
			}
		}
	}
	h.service.RevokeOAuthToken(r.Context(), tok)
	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Discovery — the two well-known documents
// ---------------------------------------------------------------------------

// HandleProtectedResourceMetadata serves the atproto RFC 9728 resource
// document — only for requests on the PDS host (server.go dispatches).
func (h *Handlers) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.service.featureOn(r.Context()) {
		writeOAuthErr(w, http.StatusNotFound, "invalid_request", "not found")
		return
	}
	writeOAuthJSONDoc(w, http.StatusOK, map[string]any{
		"resource":                 h.service.cfg.PDSURL,
		"authorization_servers":    []string{issuerOrigin(r)},
		"scopes_supported":         []string{"atproto", "transition:generic", "transition:chat.bsky", "transition:email"},
		"bearer_methods_supported": []string{"header"},
	})
}

// HandleAuthorizationServerMetadata serves the merged RFC 8414 document
// — issuer is the request origin, so the doc is correct under either
// issuer (the shared API/PDS host, or a separate PDS host). The
// PAR/DPoP/client-metadata fields satisfy the atproto profile; the
// shared endpoints keep it honest for §120 clients.
func (h *Handlers) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if !h.service.featureOn(r.Context()) {
		return // §120 doc owns the root when atproto is off
	}
	origin := issuerOrigin(r)
	writeOAuthJSONDoc(w, http.StatusOK, map[string]any{
		"issuer":                                           origin,
		"authorization_endpoint":                           origin + "/oauth/authorize",
		"token_endpoint":                                   origin + "/oauth/token",
		"revocation_endpoint":                              origin + "/oauth/revoke",
		"registration_endpoint":                            origin + "/oauth/register",
		"pushed_authorization_request_endpoint":            origin + "/oauth/par",
		"require_pushed_authorization_requests":            true,
		"response_types_supported":                         []string{"code"},
		"grant_types_supported":                            []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":                 []string{"S256"},
		"token_endpoint_auth_methods_supported":            []string{"none", "private_key_jwt"},
		"token_endpoint_auth_signing_alg_values_supported": []string{"ES256"},
		"scopes_supported": []string{
			"atproto", "transition:generic", "transition:chat.bsky", "transition:email",
			"openid", "agent.use", "api.read.digest", "api.read.content",
			"api.read.me", "api.read.safety", "api.read.spaces", "api.read.search",
		},
		"dpop_signing_alg_values_supported":              []string{"ES256"},
		"client_id_metadata_document_supported":          true,
		"authorization_response_iss_parameter_supported": true,
	})
}

// ---------------------------------------------------------------------------
// Member grant management — /api/v1/me/atproto/oauth-sessions
// ---------------------------------------------------------------------------

// HandleListOAuthSessions serves GET /me/atproto/oauth-sessions — the
// member's live grants (distinct client, scope, binding).
func (h *Handlers) HandleListOAuthSessions(w http.ResponseWriter, r *http.Request) {
	account, ok := h.myAccount(w, r)
	if !ok {
		return
	}
	grants, err := h.service.ListOAuthGrants(r.Context(), account.ID)
	if err != nil {
		slog.Error("atproto oauth: list grants", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list OAuth sessions")
		return
	}
	type grantView struct {
		ClientID       string  `json:"client_id"`
		Scope          string  `json:"scope"`
		ClientJKT      *string `json:"client_jkt"`
		FirstGrantedAt string  `json:"first_granted_at"`
		LastGrantedAt  string  `json:"last_granted_at"`
	}
	out := make([]grantView, 0, len(grants))
	for _, g := range grants {
		first, _ := g.FirstGrantedAt.(time.Time)
		last, _ := g.LastGrantedAt.(time.Time)
		out = append(out, grantView{
			ClientID:       g.ClientID,
			Scope:          g.Scope,
			ClientJKT:      g.ClientJkt,
			FirstGrantedAt: first.Format(time.RFC3339),
			LastGrantedAt:  last.Format(time.RFC3339),
		})
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleRevokeOAuthSession serves DELETE /me/atproto/oauth-sessions —
// drops the whole client×account family. Idempotent 200.
func (h *Handlers) HandleRevokeOAuthSession(w http.ResponseWriter, r *http.Request) {
	clientID := r.URL.Query().Get("client_id")
	if clientID == "" || len(clientID) > 2048 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_client_id", "client_id is required")
		return
	}
	account, ok := h.myAccount(w, r)
	if !ok {
		return
	}
	if err := h.service.RevokeOAuthGrantForClient(r.Context(), account.ID, clientID); err != nil {
		slog.Error("atproto oauth: revoke grant", "error", err, "client_id", clientID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to revoke the session")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}
