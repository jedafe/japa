// Package mcp — oauth_handlers.go: HTTP surface of the §120 OAuth 2.1
// authorization server: discovery metadata, dynamic client registration,
// the authorize/consent flow, token issuance, and revocation.
package mcp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const (
	// maxOAuthFormBytes caps urlencoded bodies on /oauth/token, /oauth/revoke,
	// and POST /oauth/authorize — ParseForm has no built-in body limit.
	maxOAuthFormBytes = 64 << 10 // 64 KiB is generous for OAuth form fields

	// maxRedirectURIs / maxRedirectURILen bound RFC 7591 registration input.
	maxRedirectURIs    = 10
	maxRedirectURILen  = 2048
	maxOAuthStateLen   = 1024
	maxClientNameLen   = 200
	maxOAuthScopeField = 1024
)

// OAuthHandlers serves the §120 OAuth 2.1 endpoints. The issuer is the
// public base URL of the API host (e.g. https://api.example.com).
type OAuthHandlers struct {
	svc           *OAuthService
	issuer        string
	siteURL       string
	siteName      string
	kr            *auth.KeyRotation
	vk            valkeycompat.Cmdable
	consentSecret []byte
	perms         oauthPermChecker
}

// oauthPermChecker is the user-capability lookup the consent flow uses to
// intersect a requested grant with what the resource owner can actually do
// (§120 v1.4 — agent.use requires the agent.use RBAC node).
type oauthPermChecker interface {
	HasPermission(ctx context.Context, userID pgtype.UUID, permKey string) bool
}

// NewOAuthHandlers creates the handler set. consentSecret signs the consent
// token binding the approve/deny POST to the rendered GET page (CSRF).
// Callers should pass JC_ENCRYPTION_KEY; empty falls back to the JWT secret.
func NewOAuthHandlers(svc *OAuthService, issuer, siteURL, siteName string, kr *auth.KeyRotation, vk valkeycompat.Cmdable, consentSecret string) *OAuthHandlers {
	return &OAuthHandlers{
		svc:           svc,
		issuer:        strings.TrimRight(issuer, "/"),
		siteURL:       strings.TrimRight(siteURL, "/"),
		siteName:      siteName,
		kr:            kr,
		vk:            vk,
		consentSecret: []byte(consentSecret),
	}
}

// SetPermissionResolver wires the RBAC resolver used to gate user-scoped
// grants (agent.use). Called post-construction to match the codebase's
// setter-injection pattern for services built early in NewServer.
func (h *OAuthHandlers) SetPermissionResolver(p oauthPermChecker) {
	h.perms = p
}

// ---------------------------------------------------------------------------
// Discovery (RFC 8414 + RFC 9728)
// ---------------------------------------------------------------------------

// HandleProtectedResourceMetadata serves RFC 9728 metadata for the MCP
// protected resource. Also answers the resource-path form
// /.well-known/oauth-protected-resource/api/v1/mcp.
func (h *OAuthHandlers) HandleProtectedResourceMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"resource":                 h.issuer + "/api/v1/mcp",
		"authorization_servers":    []string{h.issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         oauthGrantableScopes(),
	})
}

// HandleAuthorizationServerMetadata serves RFC 8414 metadata.
func (h *OAuthHandlers) HandleAuthorizationServerMetadata(w http.ResponseWriter, _ *http.Request) {
	writeOAuthJSON(w, http.StatusOK, map[string]any{
		"issuer":                                h.issuer,
		"authorization_endpoint":                h.issuer + "/oauth/authorize",
		"token_endpoint":                        h.issuer + "/oauth/token",
		"registration_endpoint":                 h.issuer + "/oauth/register",
		"revocation_endpoint":                   h.issuer + "/oauth/revoke",
		"userinfo_endpoint":                     h.issuer + "/oauth/userinfo",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"none"},
		"scopes_supported":                      oauthGrantableScopes(),
		"service_documentation":                 h.siteURL + "/docs",
	})
}

// oauthGrantableScopes returns the scope values an OAuth grant may carry:
// every non-admin entry of the §47 catalog plus the OAuth-only `openid`
// pseudo-scope (§120 v1.4 — unlocks /oauth/userinfo, inert on API routes).
func oauthGrantableScopes() []string {
	out := make([]string, 0, len(publicapi.ScopeCatalog)+1)
	for _, s := range publicapi.SelfServiceScopes() {
		out = append(out, s.Value)
	}

	out = append(out, oauthScopeOpenID)
	sort.Strings(out)

	return out
}

// oauthDefaultScopeSet is what a client gets when it doesn't request scopes.
var oauthDefaultScopeSet = oauthGrantableScopes()

// filterGrantableScopes intersects requested scopes with the grantable set.
// Unknown and admin-only scopes are dropped (never granted silently).
func filterGrantableScopes(requested []string) []string {
	allowed := make(map[string]struct{}, len(oauthDefaultScopeSet))
	for _, s := range oauthDefaultScopeSet {
		allowed[s] = struct{}{}
	}

	var out []string

	seen := map[string]struct{}{}

	for _, s := range requested {
		if _, ok := allowed[s]; !ok {
			continue
		}

		if _, dup := seen[s]; dup {
			continue
		}

		seen[s] = struct{}{}

		out = append(out, s)
	}

	sort.Strings(out)

	return out
}

// ---------------------------------------------------------------------------
// Dynamic client registration (RFC 7591)
// ---------------------------------------------------------------------------

type registerRequest struct {
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	Scope                   string   `json:"scope"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	LogoURI                 string   `json:"logo_uri"`
	ClientURI               string   `json:"client_uri"`
}

// HandleRegister implements RFC 7591 client registration. Public clients
// only: token_endpoint_auth_method is forced to "none" and no client_secret
// is issued — MCP hosts authenticate with PKCE.
func (h *OAuthHandlers) HandleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "request body must be JSON")
		return
	}

	if len(req.RedirectURIs) == 0 {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}

	if len(req.RedirectURIs) > maxRedirectURIs {
		writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "too many redirect_uris")
		return
	}

	for _, u := range req.RedirectURIs {
		if len(u) > maxRedirectURILen || !isValidRedirectURI(u) {
			writeOAuthError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris must be absolute https URLs (localhost allowed in dev)")
			return
		}
	}

	if len(req.Scope) > maxOAuthScopeField {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "scope field too long")
		return
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code", "refresh_token"}
	}

	for _, g := range grantTypes {
		if g != "authorization_code" && g != "refresh_token" {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "unsupported grant_type: "+g)
			return
		}
	}

	scopes := filterGrantableScopes(strings.Fields(req.Scope))
	if len(scopes) == 0 {
		scopes = oauthDefaultScopeSet
	}

	name := strings.TrimSpace(req.ClientName)
	if name == "" {
		name = "MCP Client"
	}

	if len(name) > maxClientNameLen {
		name = name[:maxClientNameLen]
	}

	// §120: hard cap on registered clients — DCR is public, so per-IP rate
	// limits alone cannot bound a distributed burst between sweeps.
	if max := h.svc.maxClients(r.Context()); max > 0 {
		n, cerr := h.svc.store.CountClients(r.Context())
		if cerr != nil {
			slog.Error("oauth: count clients", "error", cerr)
			writeOAuthError(w, http.StatusInternalServerError, "server_error", "registration unavailable")

			return
		}

		if n >= max {
			writeOAuthError(w, http.StatusBadRequest, "invalid_client_metadata", "client registration limit reached")
			return
		}
	}

	clientID, err := newRandomToken(oauthClientIDPrefix, 12)
	if err != nil {
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to generate client id")
		return
	}

	client := OAuthClient{
		ClientID:                clientID,
		ClientName:              name,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              grantTypes,
		Scopes:                  scopes,
		TokenEndpointAuthMethod: "none",
	}
	if err := h.svc.store.CreateClient(r.Context(), client); err != nil {
		slog.Error("oauth: create client", "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "failed to register client")

		return
	}

	if err := h.svc.store.AuditOAuthEvent(r.Context(), "oauth.client_registered", pgtype.UUID{}, map[string]any{
		"client_id":   client.ClientID,
		"client_name": client.ClientName,
		"scopes":      client.Scopes,
	}, clientip.FromRequest(r)); err != nil {
		slog.Warn("oauth: audit client_registered", "error", err)
	}

	writeOAuthJSON(w, http.StatusCreated, map[string]any{
		"client_id":                  client.ClientID,
		"client_name":                client.ClientName,
		"redirect_uris":              client.RedirectURIs,
		"grant_types":                client.GrantTypes,
		"scope":                      strings.Join(client.Scopes, " "),
		"token_endpoint_auth_method": "none",
		"client_id_issued_at":        time.Now().Unix(),
	})
}

// isValidRedirectURI accepts absolute https URIs, plus http on
// localhost/loopback for local development and testing. Fragments and
// userinfo are rejected — RFC 6749 §3.1.2 forbids fragments on the
// redirection endpoint, and userinfo in a registered URI is a phishing aid.
func isValidRedirectURI(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return false
	}

	if u.Scheme == "https" {
		return true
	}

	if u.Scheme == "http" {
		host := u.Hostname()
		return host == "localhost" || host == "127.0.0.1" || host == "::1"
	}

	return false
}

// ---------------------------------------------------------------------------
// Authorize + consent
// ---------------------------------------------------------------------------

// authorizeParams carries the validated authorization request between the
// GET (render consent) and POST (decide) handlers.
type authorizeParams struct {
	ClientID            string
	RedirectURI         string
	Scope               string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
}

// HandleAuthorize serves GET (validate + render consent, or redirect to
// login) and POST (consent decision → code grant → client redirect).
func (h *OAuthHandlers) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	p, verr := h.validateAuthorizeRequest(r)
	if verr != nil {
		// RFC 6749 §4.1.2.1: once client_id + redirect_uri have been
		// proven against the registered set, rejections go back to the
		// client as error params. Before that point an error page is the
		// only safe channel — redirecting would hand control to an
		// unvalidated URI.
		if verr.fatal || p == nil {
			renderOAuthErrorPage(w, h.siteName, verr.desc)
			return
		}

		h.redirectWithError(w, r, p, verr.code, verr.desc)

		return
	}

	user := h.authorizeUser(r)
	if user == nil {
		if r.Method == http.MethodGet {
			h.redirectToLogin(w, r)
			return
		}

		renderOAuthErrorPage(w, h.siteName, "Your session has expired. Please try connecting again.")

		return
	}

	// §120 v1.4: the grant cannot exceed what the resource owner can do —
	// agent.use requires the user to hold the agent.use RBAC node. Fail at
	// consent rather than issuing a token that 403s on every call.
	if err := h.checkUserCapability(r.Context(), p.Scope, user.UserID); err != nil {
		// access_denied — the AS denied the grant on capability grounds;
		// the client gets a normal error redirect, not a dead-end page.
		h.redirectWithError(w, r, p, "access_denied", err.Error())
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.renderConsent(w, r, p, user)
	case http.MethodPost:
		h.handleConsentDecision(w, r, p, user)
	default:
		w.Header().Set("Allow", "GET, POST")
		renderOAuthErrorPage(w, h.siteName, "Method not allowed")
	}
}

// oauthReqError carries an RFC 6749 authorization error. fatal marks the
// cases §4.1.2.1 forbids redirecting on — an unparseable request, an
// unknown client_id, or a redirect_uri outside the registered set — where
// redirecting would hand control to an unvalidated URI.
type oauthReqError struct {
	code  string
	desc  string
	fatal bool
}

func reqErr(code, desc string) *oauthReqError {
	return &oauthReqError{code: code, desc: desc}
}

func fatalReqErr(desc string) *oauthReqError {
	return &oauthReqError{code: "invalid_request", desc: desc, fatal: true}
}

// validateAuthorizeRequest checks the OAuth params on GET (query) or POST
// (form) and returns the canonical request. On failure the error says
// whether the client may be redirected: once client_id + redirect_uri are
// proven (p non-nil), rejections are redirectable RFC 6749 error params.
func (h *OAuthHandlers) validateAuthorizeRequest(r *http.Request) (*authorizeParams, *oauthReqError) {
	var q url.Values

	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(nil, r.Body, maxOAuthFormBytes)
		if err := r.ParseForm(); err != nil {
			return nil, fatalReqErr("invalid request body")
		}

		q = r.PostForm
	} else {
		q = r.URL.Query()
	}

	// Fatal phase: nothing may redirect until client_id + redirect_uri
	// are proven against the registered set.
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")

	if clientID == "" || redirectURI == "" {
		return nil, fatalReqErr("client_id and redirect_uri are required")
	}

	if len(clientID) > 128 || len(redirectURI) > maxRedirectURILen {
		return nil, fatalReqErr("client_id or redirect_uri is too long")
	}

	client, err := h.svc.store.GetClientByClientID(r.Context(), clientID)
	if err != nil {
		return nil, fatalReqErr("unknown client_id")
	}

	matched := false

	for _, ru := range client.RedirectURIs {
		if ru == redirectURI {
			matched = true
			break
		}
	}

	if !matched {
		return nil, fatalReqErr("redirect_uri does not match the registered URIs for this client")
	}

	p := &authorizeParams{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		State:       q.Get("state"),
	}

	if len(p.State) > maxOAuthStateLen {
		return p, reqErr("invalid_request", "state is too long")
	}

	if q.Get("response_type") != "code" {
		return p, reqErr("unsupported_response_type", `only response_type "code" is supported`)
	}

	if q.Get("code_challenge_method") != "S256" {
		return p, reqErr("invalid_request", "code_challenge_method must be S256")
	}

	if q.Get("code_challenge") == "" {
		return p, reqErr("invalid_request", "code_challenge is required")
	}

	if len(q.Get("code_challenge")) > 512 || len(q.Get("scope")) > maxOAuthScopeField {
		return p, reqErr("invalid_request", "request field too long")
	}

	// §120: a client registered without authorization_code (e.g. refresh-only)
	// can never exchange the code — fail before rendering consent.
	grantOK := false

	for _, g := range client.GrantTypes {
		if g == "authorization_code" {
			grantOK = true
			break
		}
	}

	if !grantOK {
		return p, reqErr("unauthorized_client", "client is not registered for the authorization_code grant")
	}

	scope := q.Get("scope")
	if scope == "" {
		scope = strings.Join(client.Scopes, " ")
	} else {
		// Requested scope must be a subset of the client's registered set.
		clientSet := make(map[string]struct{}, len(client.Scopes))
		for _, s := range client.Scopes {
			clientSet[s] = struct{}{}
		}

		for _, s := range strings.Fields(scope) {
			if _, ok := clientSet[s]; !ok {
				return p, reqErr("invalid_scope", "requested scope exceeds what this client registered for")
			}
		}
	}
	// §120 v1.4: agent.use is mandatory only for clients registered for
	// agent access — an MCP client must carry it in every grant, while a
	// non-agent client (extension, identity-only) cannot even request it
	// (scope ⊆ client scopes). Whether the *user* holds agent.use is a
	// consent-time check in HandleAuthorize, not a request-shape check.
	if publicapi.HasScope(client.Scopes, agentUsePermission) &&
		!publicapi.HasScope(strings.Fields(scope), agentUsePermission) {
		return p, reqErr("invalid_scope", "the agent.use scope is required for MCP access")
	}

	p.Scope = scope
	p.CodeChallenge = q.Get("code_challenge")
	p.CodeChallengeMethod = "S256"

	return p, nil
}

// checkUserCapability intersects the effective grant scope with the user's
// own capability. The only user-gated scope today is agent.use (§03 RBAC
// node); every other grantable scope is self-service, matching what
// /me/api-keys may mint. A nil resolver fails closed — in production it is
// always wired, and a grant we cannot verify must not be issued.
func (h *OAuthHandlers) checkUserCapability(ctx context.Context, scope string, userID pgtype.UUID) error {
	if !publicapi.HasScope(strings.Fields(scope), agentUsePermission) {
		return nil
	}

	if h.perms == nil || !h.perms.HasPermission(ctx, userID, agentUsePermission) {
		return errors.New("this application requires agent access, which is not enabled on your account")
	}

	return nil
}

// authorizeUser resolves the resource owner from the Bearer JWT or the
// jc_token session cookie — both are platform access tokens validated
// identically to RequireAuth.
func (h *OAuthHandlers) authorizeUser(r *http.Request) *auth.AuthenticatedUser {
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

// redirectToLogin sends an unauthenticated user to the main-site login page
// with the full authorize URL as the post-login target. The login form reads
// the `redirect` query param and navigates to it after sign-in.
//
// When the authorize request arrived on the site host itself (the §47
// browser extension path — nginx proxies /oauth/* to the API on the site
// domain), the target stays site-relative: the login page allows relative
// paths unconditionally, and keeping the bounce on the site host guarantees
// the jc_token cookie is in scope regardless of the cookie domain or API
// hostname.
func (h *OAuthHandlers) redirectToLogin(w http.ResponseWriter, r *http.Request) {
	target := h.issuer + r.URL.RequestURI()
	if u, err := url.Parse(h.siteURL); err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host) {
		target = r.URL.RequestURI()
	}

	login := h.siteURL + "/login?redirect=" + url.QueryEscape(target)
	http.Redirect(w, r, login, http.StatusFound)
}

// consentTokenValue produces the HMAC token binding a consent POST to the
// rendered GET page. Payload: user_id|client_id|redirect_uri|scope|state|challenge|expiry.
func (h *OAuthHandlers) consentTokenValue(p *authorizeParams, userID pgtype.UUID, expiry time.Time) string {
	payload := strings.Join([]string{
		"v1",
		userID.String(),
		p.ClientID,
		p.RedirectURI,
		p.Scope,
		p.State,
		p.CodeChallenge,
		strconv.FormatInt(expiry.Unix(), 10),
	}, "|")
	mac := hmac.New(sha256.New, h.consentSecret)
	mac.Write([]byte(payload))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) + "." +
		strconv.FormatInt(expiry.Unix(), 10)
}

func (h *OAuthHandlers) verifyConsentToken(token string, p *authorizeParams, userID pgtype.UUID) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}

	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}

	expected := h.consentTokenValue(p, userID, time.Unix(exp, 0))

	return hmac.Equal([]byte(token), []byte(expected))
}

// handleConsentDecision processes the consent POST: verify the consent
// token, mint a code on approve, and redirect back to the client.
func (h *OAuthHandlers) handleConsentDecision(w http.ResponseWriter, r *http.Request, p *authorizeParams, user *auth.AuthenticatedUser) {
	token := r.PostForm.Get("consent_token")
	if !h.verifyConsentToken(token, p, user.UserID) {
		renderOAuthErrorPage(w, h.siteName, "The consent form has expired or is invalid. Please try connecting again.")
		return
	}

	if r.PostForm.Get("action") != "approve" {
		h.redirectWithError(w, r, p, "access_denied", "The user denied the request")
		return
	}

	code, err := newRandomToken(oauthAuthCodePrefix, 24)
	if err != nil {
		renderOAuthErrorPage(w, h.siteName, "Failed to issue an authorization code. Please try again.")
		return
	}

	if err := h.svc.store.CreateAuthorizationCode(r.Context(), hashToken(code), OAuthAuthorizationCode{
		ClientID:            p.ClientID,
		UserID:              user.UserID,
		RedirectURI:         p.RedirectURI,
		Scope:               p.Scope,
		CodeChallenge:       p.CodeChallenge,
		CodeChallengeMethod: "S256",
		ExpiresAt:           pgtype.Timestamptz{Time: time.Now().Add(authorizationCodeTTL), Valid: true},
	}); err != nil {
		slog.Error("oauth: create authorization code", "error", err)
		renderOAuthErrorPage(w, h.siteName, "Failed to issue an authorization code. Please try again.")

		return
	}

	if err := h.svc.store.AuditOAuthEvent(r.Context(), "oauth.authorize", user.UserID, map[string]any{
		"client_id": p.ClientID,
		"scope":     p.Scope,
	}, clientip.FromRequest(r)); err != nil {
		slog.Warn("oauth: audit authorize", "error", err)
	}

	target, perr := url.Parse(p.RedirectURI)
	if perr != nil {
		renderOAuthErrorPage(w, h.siteName, "Registered redirect URI is malformed. Please re-register the client.")
		return
	}

	qq := target.Query()
	qq.Set("code", code)

	if p.State != "" {
		qq.Set("state", p.State)
	}

	target.RawQuery = qq.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound) // #nosec G710 -- redirect_uri was exact-match validated against the registered client
}

// redirectWithError appends RFC 6749 error params to the (already
// validated) client redirect URI.
func (h *OAuthHandlers) redirectWithError(w http.ResponseWriter, r *http.Request, p *authorizeParams, code, desc string) {
	target, perr := url.Parse(p.RedirectURI)
	if perr != nil {
		renderOAuthErrorPage(w, h.siteName, desc)
		return
	}

	qq := target.Query()
	qq.Set("error", code)
	qq.Set("error_description", desc)

	if p.State != "" {
		qq.Set("state", p.State)
	}

	target.RawQuery = qq.Encode()
	http.Redirect(w, r, target.String(), http.StatusFound) // #nosec G710 -- redirect_uri was exact-match validated against the registered client
}

// ---------------------------------------------------------------------------
// Token + revoke
// ---------------------------------------------------------------------------

// HandleToken serves the token endpoint: authorization_code (PKCE S256)
// and refresh_token grants. Public clients authenticate by client_id only.
func (h *OAuthHandlers) HandleToken(w http.ResponseWriter, r *http.Request) {
	// RFC 6749 §5.1 — token responses (success AND error) are never cached.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "request must be application/x-www-form-urlencoded")
		return
	}

	clientID := r.PostForm.Get("client_id")
	if clientID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	client, err := h.svc.store.GetClientByClientID(r.Context(), clientID)
	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_client", "unknown client_id")
		return
	}

	grantType := r.PostForm.Get("grant_type")
	if grantType == "" {
		// RFC 6749 §5.2 — a missing grant_type is invalid_request, not
		// unauthorized_client (which describes a registered client asking
		// for a grant it did not register).
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "grant_type is required")
		return
	}

	grantAllowed := false

	for _, g := range client.GrantTypes {
		if g == grantType {
			grantAllowed = true
			break
		}
	}

	if !grantAllowed {
		writeOAuthError(w, http.StatusBadRequest, "unauthorized_client", "client is not registered for this grant_type")
		return
	}

	var resp *tokenResponse

	switch grantType {
	case "authorization_code":
		resp, err = h.svc.ExchangeAuthorizationCode(r.Context(),
			r.PostForm.Get("code"), clientID,
			r.PostForm.Get("redirect_uri"), r.PostForm.Get("code_verifier"))
	case "refresh_token":
		// §120 v1.4: re-check the resource owner's capability on every
		// rotation — consent ran at authorize time, but a rotating refresh
		// chain must not outlive a revoked agent.use node forever.
		resp, err = h.svc.RotateRefreshToken(r.Context(),
			r.PostForm.Get("refresh_token"), clientID,
			func(ctx context.Context, userID pgtype.UUID, scope string) error {
				return h.checkUserCapability(ctx, scope, userID)
			})
	default:
		writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code or refresh_token")
		return
	}

	if err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "the grant is invalid, expired, or already used")
		return
	}

	if aerr := h.svc.store.AuditOAuthEvent(r.Context(), "oauth.token_grant", resp.userID, map[string]any{
		"client_id":  clientID,
		"grant_type": grantType,
		"scope":      resp.Scope,
	}, clientip.FromRequest(r)); aerr != nil {
		slog.Warn("oauth: audit token_grant", "error", aerr)
	}

	writeOAuthJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// User grant management (§120 connected apps)
// ---------------------------------------------------------------------------

// grantView is the per-client row on GET /me/oauth/grants.
type grantView struct {
	ClientID       string   `json:"client_id"`
	ClientName     string   `json:"client_name"`
	Scopes         []string `json:"scopes"`
	FirstGrantedAt string   `json:"first_granted_at"`
	LastGrantedAt  string   `json:"last_granted_at"`
}

// HandleListGrants serves GET /api/v1/me/oauth/grants — the caller's
// connected agent clients (distinct client_id over live oauth_tokens rows).
func (h *OAuthHandlers) HandleListGrants(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	grants, err := h.svc.ListGrants(r.Context(), user.UserID)
	if err != nil {
		slog.Error("oauth: list grants", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list connected apps")

		return
	}

	out := make([]grantView, 0, len(grants))
	for _, g := range grants {
		out = append(out, grantView{
			ClientID:       g.ClientID,
			ClientName:     g.ClientName,
			Scopes:         g.Scopes,
			FirstGrantedAt: g.FirstGrantedAt.Time.Format(time.RFC3339),
			LastGrantedAt:  g.LastGrantedAt.Time.Format(time.RFC3339),
		})
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": out})
}

// HandleRevokeGrant serves DELETE /api/v1/me/oauth/grants/{clientId} —
// revokes the whole client/user token family. Idempotent: an unknown or
// already-dead family is still a 200 so the UI can fire-and-forget.
func (h *OAuthHandlers) HandleRevokeGrant(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return
	}

	clientID := chi.URLParam(r, "clientId")
	if clientID == "" || len(clientID) > 128 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_client_id", "clientId is required")
		return
	}

	if err := h.svc.RevokeUserGrant(r.Context(), clientID, user.UserID); err != nil {
		slog.Error("oauth: revoke grant", "error", err, "client_id", clientID)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to disconnect the app")

		return
	}

	if err := h.svc.store.AuditOAuthEvent(r.Context(), "oauth.token_family_revoked", user.UserID, map[string]any{
		"client_id": clientID,
		"reason":    "user_revoked",
	}, clientip.FromRequest(r)); err != nil {
		slog.Warn("oauth: audit grant revoke", "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "revoked"}})
}

// HandleUserinfo serves GET /oauth/userinfo (§120 v1.4): the OIDC-style
// claims endpoint "Login with JapaConnect" clients call to resolve the
// signed-in identity. Requires a live jcpat_ access token with the openid
// scope. Responses are never cached and errors follow the RFC 6750 shape.
func (h *OAuthHandlers) HandleUserinfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")

	token := ""

	if ah := r.Header.Get("Authorization"); ah != "" {
		parts := strings.SplitN(ah, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			token = parts[1]
		}
	}

	if !strings.HasPrefix(token, oauthAccessTokenPrefix) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo"`)
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "a valid access token is required")

		return
	}

	userID, scopes, clientID, err := h.svc.ResolveAccessToken(r.Context(), token)
	if err != nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo", error="invalid_token"`)
		writeOAuthError(w, http.StatusUnauthorized, "invalid_token", "the access token is invalid, expired, or revoked")

		return
	}

	if !publicapi.HasScope(scopes, oauthScopeOpenID) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="userinfo", error="insufficient_scope"`)
		writeOAuthError(w, http.StatusForbidden, "insufficient_scope", "the openid scope is required")

		return
	}

	ident, err := h.svc.store.GetUserIdentity(r.Context(), userID)
	if err != nil {
		slog.Error("oauth: userinfo lookup", "error", err, "client_id", clientID)
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "identity unavailable")

		return
	}

	name := ident.Username
	if ident.DisplayName != nil && *ident.DisplayName != "" {
		name = *ident.DisplayName
	}

	claims := map[string]any{
		"sub":                ident.UserID.String(),
		"preferred_username": ident.Username,
		"name":               name,
		"email":              ident.Email,
		"email_verified":     ident.EmailVerified,
	}
	if ident.AvatarURL != nil && *ident.AvatarURL != "" {
		claims["picture"] = *ident.AvatarURL
	}

	writeOAuthJSON(w, http.StatusOK, claims)
}

// HandleRevoke implements RFC 7009 — always 200 on well-formed input.
func (h *OAuthHandlers) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxOAuthFormBytes)
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if tok := r.PostForm.Get("token"); tok != "" {
		// Tombstone access tokens; drop refresh tokens from the store path.
		if strings.HasPrefix(tok, oauthAccessTokenPrefix) || strings.HasPrefix(tok, oauthRefreshTokenPrefix) {
			if err := h.svc.RevokeToken(r.Context(), tok); err != nil {
				slog.Warn("oauth: revoke token", "error", err)
			} else {
				kind := "access"
				if strings.HasPrefix(tok, oauthRefreshTokenPrefix) {
					kind = "refresh"
				}

				if aerr := h.svc.store.AuditOAuthEvent(r.Context(), "oauth.token_revoke", pgtype.UUID{}, map[string]any{
					"token_type": kind,
				}, clientip.FromRequest(r)); aerr != nil {
					slog.Warn("oauth: audit token_revoke", "error", aerr)
				}
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}

// ---------------------------------------------------------------------------
// Consent page + error responses
// ---------------------------------------------------------------------------

// renderConsent renders the self-contained consent page. It POSTs the full
// OAuth request back to /oauth/authorize with a signed consent_token.
func (h *OAuthHandlers) renderConsent(w http.ResponseWriter, r *http.Request, p *authorizeParams, user *auth.AuthenticatedUser) {
	client, err := h.svc.store.GetClientByClientID(r.Context(), p.ClientID)
	clientName := p.ClientID

	if err == nil && client != nil && client.ClientName != "" {
		clientName = client.ClientName
	}

	consentToken := h.consentTokenValue(p, user.UserID, time.Now().Add(10*time.Minute))

	scopeLabels := scopeDescriptions(p.Scope)

	hidden := func(k, v string) string {
		return `<input type="hidden" name="` + html.EscapeString(k) + `" value="` + html.EscapeString(v) + `">`
	}

	var b strings.Builder

	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>Authorize — ` + html.EscapeString(h.siteName) + `</title>`)
	b.WriteString(`<style>
body{font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;display:flex;justify-content:center;padding:2rem;margin:0}
.card{background:#1e293b;border:1px solid #334155;border-radius:12px;max-width:28rem;width:100%;padding:2rem}
h1{font-size:1.25rem;margin:0 0 .5rem}
.muted{color:#94a3b8;font-size:.9rem}
ul{list-style:none;padding:0;margin:1rem 0}
li{padding:.4rem 0;border-bottom:1px solid #334155;font-size:.9rem}
li:last-child{border-bottom:none}
.scope{font-weight:600}
.actions{display:flex;gap:.75rem;margin-top:1.5rem}
button{flex:1;padding:.65rem 1rem;border-radius:8px;border:0;font-size:.95rem;cursor:pointer}
.approve{background:#38bdf8;color:#0f172a;font-weight:600}
.deny{background:transparent;color:#e2e8f0;border:1px solid #475569}
</style></head><body><div class="card">`)
	b.WriteString(`<h1>` + html.EscapeString(clientName) + `</h1>`)
	b.WriteString(`<p class="muted">wants to connect to your ` + html.EscapeString(h.siteName) + ` account. It will be able to use these capabilities as you:</p><ul>`)

	for _, s := range scopeLabels {
		b.WriteString(`<li><span class="scope">` + html.EscapeString(s.Label) + `</span><br><span class="muted">` + html.EscapeString(s.Description) + `</span></li>`)
	}

	b.WriteString(`</ul><form method="post" action="/oauth/authorize">`)
	b.WriteString(hidden("response_type", "code"))
	b.WriteString(hidden("client_id", p.ClientID))
	b.WriteString(hidden("redirect_uri", p.RedirectURI))
	b.WriteString(hidden("scope", p.Scope))
	b.WriteString(hidden("state", p.State))
	b.WriteString(hidden("code_challenge", p.CodeChallenge))
	b.WriteString(hidden("code_challenge_method", "S256"))
	b.WriteString(hidden("consent_token", consentToken))
	b.WriteString(`<div class="actions"><button type="submit" name="action" value="deny" class="deny">Deny</button>`)
	b.WriteString(`<button type="submit" name="action" value="approve" class="approve">Authorize</button></div>`)
	b.WriteString(`</form><p class="muted" style="margin-top:1rem">Access is read-only. The client can disconnect itself at any time, and access tokens expire automatically.</p>`)
	b.WriteString(`</div></body></html>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Self-contained page: no external assets, no framing (clickjacking).
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(b.String()))
}

type scopeLabel struct{ Label, Description string }

func scopeDescriptions(scope string) []scopeLabel {
	catalog := make(map[string]publicapi.ScopeOption, len(publicapi.ScopeCatalog))
	for _, s := range publicapi.ScopeCatalog {
		catalog[s.Value] = s
	}

	var out []scopeLabel

	for _, s := range strings.Fields(scope) {
		if s == oauthScopeOpenID {
			out = append(out, scopeLabel{Label: "Know who you are", Description: "Read your basic profile (name, username, email)"})
			continue
		}

		if opt, ok := catalog[s]; ok {
			out = append(out, scopeLabel{Label: opt.Label, Description: opt.Description})
		} else {
			out = append(out, scopeLabel{Label: s, Description: ""})
		}
	}

	return out
}

// renderOAuthErrorPage renders a minimal error page for authorize-time
// failures (never a redirect — the target URI is untrusted at this point).
func renderOAuthErrorPage(w http.ResponseWriter, siteName, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(`<!doctype html><html><head><meta charset="utf-8"><title>Authorization error — ` +
		html.EscapeString(siteName) + `</title></head><body style="font-family:system-ui;display:flex;justify-content:center;padding:3rem">` +
		`<div><h1>Authorization error</h1><p>` + html.EscapeString(msg) + `</p></div></body></html>`))
}

// writeOAuthError emits an RFC 6749 error body (not the platform envelope —
// OAuth clients parse the RFC shape).
func writeOAuthError(w http.ResponseWriter, status int, code, desc string) {
	writeOAuthJSON(w, status, map[string]any{
		"error":             code,
		"error_description": desc,
	})
}

func writeOAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("oauth: encode response", "error", err)
	}
}
