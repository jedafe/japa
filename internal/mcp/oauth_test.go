package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// fakeOAuthStore is an in-memory OAuthStore for handler/service tests.
type fakeOAuthStore struct {
	clients    map[string]*OAuthClient
	codes      map[string]*OAuthAuthorizationCode // by code_hash
	tokens     map[string]*OAuthToken             // by token_hash
	identities map[pgtype.UUID]*OAuthUserIdentity
}

func newFakeOAuthStore() *fakeOAuthStore {
	return &fakeOAuthStore{
		clients:    map[string]*OAuthClient{},
		codes:      map[string]*OAuthAuthorizationCode{},
		tokens:     map[string]*OAuthToken{},
		identities: map[pgtype.UUID]*OAuthUserIdentity{},
	}
}

func (f *fakeOAuthStore) CreateClient(_ context.Context, c OAuthClient) error {
	cp := c
	f.clients[c.ClientID] = &cp
	return nil
}

func (f *fakeOAuthStore) CountClients(_ context.Context) (int64, error) {
	return int64(len(f.clients)), nil
}

func (f *fakeOAuthStore) GetClientByClientID(_ context.Context, clientID string) (*OAuthClient, error) {
	if c, ok := f.clients[clientID]; ok {
		return c, nil
	}
	return nil, pgx.ErrNoRows
}

func (f *fakeOAuthStore) CreateAuthorizationCode(_ context.Context, codeHash string, c OAuthAuthorizationCode) error {
	cp := c
	f.codes[codeHash] = &cp
	return nil
}

func (f *fakeOAuthStore) ConsumeAuthorizationCode(_ context.Context, codeHash, clientID string) (*OAuthAuthorizationCode, error) {
	c, ok := f.codes[codeHash]
	if !ok || c.ClientID != clientID || c.ExpiresAt.Time.Before(time.Now()) {
		return nil, pgx.ErrNoRows
	}
	f.codes[codeHash] = nil
	delete(f.codes, codeHash)
	return c, nil
}

func (f *fakeOAuthStore) CreateTokenPair(_ context.Context, accessHash string, access OAuthToken, refreshHash string, refresh OAuthToken) (pgtype.UUID, error) {
	a := access
	a.TokenType = "access"
	r := refresh
	r.TokenType = "refresh"
	rid := pgtype.UUID{Bytes: [16]byte{byte(len(f.tokens) + 1)}, Valid: true}
	r.ID = rid
	f.tokens[accessHash] = &a
	f.tokens[refreshHash] = &r
	return rid, nil
}

// seedToken inserts a token row directly for test setup.
func (f *fakeOAuthStore) seedToken(tokenHash string, t OAuthToken) {
	cp := t
	f.tokens[tokenHash] = &cp
}

func (f *fakeOAuthStore) GetTokenByHash(_ context.Context, tokenType, tokenHash string) (*OAuthToken, error) {
	t, ok := f.tokens[tokenHash]
	if !ok || t.TokenType != tokenType {
		return nil, pgx.ErrNoRows
	}
	return t, nil
}

func (f *fakeOAuthStore) ConsumeRefreshToken(_ context.Context, tokenHash string) (*OAuthToken, error) {
	t, ok := f.tokens[tokenHash]
	if !ok || t.TokenType != "refresh" || t.RevokedAt != nil {
		return nil, pgx.ErrNoRows
	}
	t.RevokedAt = &pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return t, nil
}

func (f *fakeOAuthStore) RevokeTokenByHash(_ context.Context, tokenHash string) error {
	if t, ok := f.tokens[tokenHash]; ok {
		t.RevokedAt = &pgtype.Timestamptz{Time: time.Now(), Valid: true}
	}
	return nil
}

func (f *fakeOAuthStore) RevokeTokenFamily(_ context.Context, clientID string, userID pgtype.UUID) error {
	for _, t := range f.tokens {
		if t.ClientID == clientID && t.UserID == userID {
			t.RevokedAt = &pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
	}
	return nil
}

func (f *fakeOAuthStore) ListUserGrants(_ context.Context, userID pgtype.UUID) ([]OAuthGrant, error) {
	byClient := map[string]*OAuthGrant{}
	for _, t := range f.tokens {
		if t.UserID != userID || t.RevokedAt != nil || t.ExpiresAt.Time.Before(time.Now()) {
			continue
		}
		g := byClient[t.ClientID]
		if g == nil {
			g = &OAuthGrant{ClientID: t.ClientID}
			if c, ok := f.clients[t.ClientID]; ok {
				g.ClientName = c.ClientName
			}
			byClient[t.ClientID] = g
		}
		for _, sc := range strings.Fields(t.Scope) {
			dup := false
			for _, have := range g.Scopes {
				if have == sc {
					dup = true
					break
				}
			}
			if !dup {
				g.Scopes = append(g.Scopes, sc)
			}
		}
	}
	out := make([]OAuthGrant, 0, len(byClient))
	for _, g := range byClient {
		sort.Strings(g.Scopes)
		out = append(out, *g)
	}
	return out, nil
}

func (f *fakeOAuthStore) MarkTokenReplaced(_ context.Context, oldID, newID pgtype.UUID) error {
	return nil
}

func (f *fakeOAuthStore) AuditOAuthEvent(_ context.Context, _ string, _ pgtype.UUID, _ map[string]any, _ string) error {
	return nil
}

func (f *fakeOAuthStore) GetUserIdentity(_ context.Context, userID pgtype.UUID) (*OAuthUserIdentity, error) {
	if id, ok := f.identities[userID]; ok {
		return id, nil
	}
	return nil, pgx.ErrNoRows
}

func newTestOAuth() (*OAuthHandlers, *OAuthService, *fakeOAuthStore) {
	store := newFakeOAuthStore()
	svc := NewOAuthService(store, nil, nil)
	h := NewOAuthHandlers(svc, "https://api.example.com", "https://example.com", "JapaConnect", nil, nil, "test-consent-secret")
	return h, svc, store
}

func pkcePair() (verifier, challenge string) {
	verifier = strings.Repeat("a", 64)
	sum := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	return
}

func TestVerifyPKCE(t *testing.T) {
	verifier, challenge := pkcePair()
	if !verifyPKCE(verifier, challenge) {
		t.Fatal("valid verifier rejected")
	}
	if verifyPKCE("wrong", challenge) {
		t.Fatal("wrong verifier accepted")
	}
	if verifyPKCE(verifier, "") || verifyPKCE("", challenge) {
		t.Fatal("empty input accepted")
	}
}

func TestRegisterClient(t *testing.T) {
	h, _, store := newTestOAuth()

	body := `{"client_name":"Claude","redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"scope":"agent.use api.read.content api.write.content"}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleRegister(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	cid, _ := resp["client_id"].(string)
	if !strings.HasPrefix(cid, oauthClientIDPrefix) {
		t.Fatalf("client_id missing prefix: %q", cid)
	}
	if _, ok := resp["client_secret"]; ok {
		t.Fatal("public clients must not receive a client_secret")
	}
	if resp["token_endpoint_auth_method"] != "none" {
		t.Fatal("auth method must be none")
	}
	// admin-only scope must be filtered out.
	if scope := resp["scope"].(string); strings.Contains(scope, "api.write") {
		t.Fatalf("admin-only scope leaked into client: %s", scope)
	}
	if store.clients[cid] == nil {
		t.Fatal("client not persisted")
	}
}

// §120: oauth_clients is hard-capped at mcp.oauth_max_clients (default 500)
// so distributed DCR spam cannot grow the table between sweeps.
func TestRegisterClientCap(t *testing.T) {
	h, _, store := newTestOAuth()

	for i := 0; i < 500; i++ {
		cid := "jcmc_spam_" + strconv.Itoa(i)
		store.clients[cid] = &OAuthClient{ClientID: cid}
	}

	body := `{"client_name":"Spammer","redirect_uris":["https://evil.example.com/cb"]}`
	req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.HandleRegister(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid_client_metadata") {
		t.Fatalf("expected invalid_client_metadata, got: %s", rec.Body.String())
	}
}

func TestRegisterClientRejectsBadRedirect(t *testing.T) {
	h, _, _ := newTestOAuth()

	bad := []string{
		"javascript:alert(1)",
		"https://agent.example.com/cb#frag",      // RFC 6749 §3.1.2 — no fragments
		"https://user:pass@agent.example.com/cb", // userinfo is a phishing aid
		"http://agent.example.com/cb",            // http only on loopback
		"ftp://agent.example.com/cb",
	}
	for _, uri := range bad {
		body := `{"redirect_uris":[` + strconv.Quote(uri) + `]}`
		req := httptest.NewRequest(http.MethodPost, "/oauth/register", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.HandleRegister(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("redirect_uri %q returned %d, want 400", uri, rec.Code)
		}
	}
}

func TestTokenRejectsUnregisteredGrantType(t *testing.T) {
	h, _, store := newTestOAuth()

	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:     "jcmc_test",
		RedirectURIs: []string{"https://agent.example.com/cb"},
		GrantTypes:   []string{"authorization_code"}, // no refresh_token
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {"jcmc_test"},
		"refresh_token": {oauthRefreshTokenPrefix + strings.Repeat("a", 64)},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.HandleToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "unauthorized_client") {
		t.Fatalf("want unauthorized_client, got %s", rec.Body.String())
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("token endpoint must emit Cache-Control: no-store")
	}
}

func TestMetadataEndpoints(t *testing.T) {
	h, _, _ := newTestOAuth()

	rec := httptest.NewRecorder()
	h.HandleProtectedResourceMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil))
	var prm map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &prm)
	if prm["resource"] != "https://api.example.com/api/v1/mcp" {
		t.Fatalf("bad resource: %v", prm["resource"])
	}
	servers, _ := prm["authorization_servers"].([]any)
	if len(servers) != 1 || servers[0] != "https://api.example.com" {
		t.Fatalf("bad authorization_servers: %v", servers)
	}

	rec = httptest.NewRecorder()
	h.HandleAuthorizationServerMetadata(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil))
	var as map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &as)
	if as["issuer"] != "https://api.example.com" {
		t.Fatalf("bad issuer: %v", as["issuer"])
	}
	if as["token_endpoint"] != "https://api.example.com/oauth/token" {
		t.Fatalf("bad token endpoint: %v", as["token_endpoint"])
	}
	methods, _ := as["code_challenge_methods_supported"].([]any)
	if len(methods) != 1 || methods[0] != "S256" {
		t.Fatalf("must require S256: %v", methods)
	}
}

func TestAuthorizeRejectsUnknownClient(t *testing.T) {
	h, _, _ := newTestOAuth()

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=jcmc_nope&redirect_uri=https://x.com/cb&code_challenge=x&code_challenge_method=S256", nil)
	rec := httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "" {
		t.Fatalf("must not redirect on invalid request, got %q", loc)
	}
}

func TestAuthorizeRequiresPKCE(t *testing.T) {
	h, _, _ := newTestOAuth()

	req := httptest.NewRequest(http.MethodGet, "/oauth/authorize?response_type=code&client_id=jcmc_x&redirect_uri=https://x.com/cb&code_challenge=x", nil)
	rec := httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (missing S256 method)", rec.Code)
	}
}

func TestAuthorizeUnauthenticatedRedirectsToLogin(t *testing.T) {
	h, _, store := newTestOAuth()
	verifier, challenge := pkcePair()
	_ = verifier

	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:     "jcmc_test",
		ClientName:   "Test Client",
		RedirectURIs: []string{"https://agent.example.com/cb"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"agent.use", "api.read.content"},
	}

	u := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {"jcmc_test"},
		"redirect_uri":          {"https://agent.example.com/cb"},
		"scope":                 {"agent.use api.read.content"},
		"state":                 {"xyz"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	// API-host request (MCP connectors): the post-login target is the
	// issuer-absolute authorize URL.
	req := httptest.NewRequest(http.MethodGet, u, nil)
	req.Host = "api.example.com"
	rec := httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 to login", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com/login?redirect=") {
		t.Fatalf("expected login redirect, got %q", loc)
	}
	if !strings.Contains(loc, url.QueryEscape("https://api.example.com/oauth/authorize")) {
		t.Fatalf("redirect target missing authorize URL: %q", loc)
	}

	// Site-host request (the §47 extension hits {site}/oauth/*, proxied):
	// the target stays site-relative so the post-login bounce returns to
	// the host where the jc_token cookie is in scope.
	req = httptest.NewRequest(http.MethodGet, u, nil)
	req.Host = "example.com"
	rec = httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	loc = rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://example.com/login?redirect=%2Foauth%2Fauthorize") {
		t.Fatalf("site-host request should emit a site-relative redirect target, got %q", loc)
	}
}

func TestTokenExchangePKCE(t *testing.T) {
	h, svc, store := newTestOAuth()
	verifier, challenge := pkcePair()

	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:     "jcmc_test",
		RedirectURIs: []string{"https://agent.example.com/cb"},
		Scopes:       []string{"agent.use", "api.read.content"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
	}

	code := oauthAuthCodePrefix + "deadbeef"
	_ = svc.store.CreateAuthorizationCode(context.Background(), hashToken(code), OAuthAuthorizationCode{
		ClientID:      "jcmc_test",
		UserID:        testUserID,
		RedirectURI:   "https://agent.example.com/cb",
		Scope:         "agent.use api.read.content",
		CodeChallenge: challenge,
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(authorizationCodeTTL), Valid: true},
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {"jcmc_test"},
		"redirect_uri":  {"https://agent.example.com/cb"},
		"code_verifier": {verifier},
	}

	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.HandleToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var tok tokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(tok.AccessToken, oauthAccessTokenPrefix) {
		t.Fatalf("bad access token prefix: %q", tok.AccessToken)
	}
	if !strings.HasPrefix(tok.RefreshToken, oauthRefreshTokenPrefix) {
		t.Fatalf("bad refresh token prefix: %q", tok.RefreshToken)
	}
	if tok.Scope != "agent.use api.read.content" {
		t.Fatalf("scope mismatch: %q", tok.Scope)
	}

	// Code is single-use — replay must fail.
	req = httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.HandleToken(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed code returned %d, want 400", rec.Code)
	}
}

func TestTokenExchangeRejectsWrongVerifier(t *testing.T) {
	h, svc, store := newTestOAuth()
	_, challenge := pkcePair()

	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:     "jcmc_test",
		RedirectURIs: []string{"https://agent.example.com/cb"},
		GrantTypes:   []string{"authorization_code"},
	}

	code := oauthAuthCodePrefix + "cafe"
	_ = svc.store.CreateAuthorizationCode(context.Background(), hashToken(code), OAuthAuthorizationCode{
		ClientID:      "jcmc_test",
		UserID:        testUserID,
		RedirectURI:   "https://agent.example.com/cb",
		Scope:         "agent.use",
		CodeChallenge: challenge,
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
	})

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"client_id":     {"jcmc_test"},
		"redirect_uri":  {"https://agent.example.com/cb"},
		"code_verifier": {"wrong"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.HandleToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestRefreshTokenRotationAndReuse(t *testing.T) {
	svc := NewOAuthService(newFakeOAuthStore(), nil, nil)
	store := svc.store.(*fakeOAuthStore)

	// Seed a live refresh token.
	tok := "jcprt_" + strings.Repeat("f", 64)
	store.seedToken(hashToken(tok), OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "refresh",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	pair, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_test", nil)
	if err != nil {
		t.Fatalf("rotation failed: %v", err)
	}
	if pair.RefreshToken == tok {
		t.Fatal("rotation must issue a new refresh token")
	}

	// Immediate retry (lost response / concurrent rotation) — within the
	// grace window this is a plain invalid_grant, NOT a family revoke.
	if _, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_test", nil); err == nil {
		t.Fatal("reused rotated token accepted")
	}

	// The new pair must keep working — rotating the fresh refresh token
	// proves the family was not revoked by the grace-window retry.
	pair2, err := svc.RotateRefreshToken(context.Background(), pair.RefreshToken, "jcmc_test", nil)
	if err != nil {
		t.Fatalf("rotation of new token failed after grace-window retry: %v", err)
	}
	if pair2.RefreshToken == pair.RefreshToken {
		t.Fatal("second rotation must issue another refresh token")
	}
}

func TestRefreshTokenStaleReuseRevokesFamily(t *testing.T) {
	svc := NewOAuthService(newFakeOAuthStore(), nil, nil)
	store := svc.store.(*fakeOAuthStore)

	// A refresh token revoked well beyond the grace window — a replay of
	// this token is a compromise signal, not a retry.
	old := "jcprt_" + strings.Repeat("9", 64)
	stale := time.Now().Add(-time.Hour)
	store.seedToken(hashToken(old), OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "refresh",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		RevokedAt: &pgtype.Timestamptz{Time: stale, Valid: true},
	})
	// A live access token from the same family must die on detection.
	live := "jcpat_" + strings.Repeat("7", 64)
	store.seedToken(hashToken(live), OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	if _, err := svc.RotateRefreshToken(context.Background(), old, "jcmc_test", nil); err == nil {
		t.Fatal("stale replay accepted")
	}
	for _, other := range store.tokens {
		if other.ClientID == "jcmc_test" && other.UserID == testUserID && other.RevokedAt == nil {
			t.Fatal("token family not revoked after stale refresh reuse")
		}
	}
}

// Presenting another client's live refresh token must be rejected WITHOUT
// consuming it — the victim's token stays usable (consume-before-bind would
// burn it and skip reuse detection).
func TestRefreshTokenCrossClientNotConsumed(t *testing.T) {
	svc := NewOAuthService(newFakeOAuthStore(), nil, nil)
	store := svc.store.(*fakeOAuthStore)

	tok := "jcprt_" + strings.Repeat("e", 64)
	store.seedToken(hashToken(tok), OAuthToken{
		ClientID:  "jcmc_victim",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "refresh",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	if _, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_attacker", nil); err == nil {
		t.Fatal("cross-client refresh token accepted")
	}

	// The victim's token must still be live and consumable.
	if _, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_victim", nil); err != nil {
		t.Fatalf("victim token was burned by the rejected request: %v", err)
	}
}

// §120 v1.4: the resource owner's capability is re-checked on every
// rotation — a user whose agent.use node was revoked since consent must
// not keep renewing agent grants. The rejection must not consume or
// revoke the token: the capability may be restored, and the chain dies
// naturally at refresh-token expiry either way.
func TestRefreshTokenRechecksUserCapability(t *testing.T) {
	svc := NewOAuthService(newFakeOAuthStore(), nil, nil)
	store := svc.store.(*fakeOAuthStore)

	tok := "jcprt_" + strings.Repeat("c", 64)
	store.seedToken(hashToken(tok), OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use api.read.content",
		TokenType: "refresh",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	deny := func(context.Context, pgtype.UUID, string) error {
		return errors.New("this application requires agent access, which is not enabled on your account")
	}
	if _, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_test", deny); err == nil {
		t.Fatal("rotation succeeded despite revoked user capability")
	}
	// The token was NOT consumed — a denied capability is not evidence of
	// compromise and must not burn the chain.
	if _, err := svc.RotateRefreshToken(context.Background(), tok, "jcmc_test", nil); err != nil {
		t.Fatalf("token was consumed by the rejected rotation: %v", err)
	}
}

// RFC 6749 §5.2 — a missing grant_type is invalid_request.
func TestTokenMissingGrantType(t *testing.T) {
	h, _, store := newTestOAuth()
	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:   "jcmc_test",
		GrantTypes: []string{"authorization_code"},
	}

	form := url.Values{"client_id": {"jcmc_test"}}
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.HandleToken(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_request") {
		t.Fatalf("want invalid_request, got %s", rec.Body.String())
	}
}

// §120 connected apps — list the caller's live grants.
func TestListGrantsHandler(t *testing.T) {
	h, _, store := newTestOAuth()
	store.clients["jcmc_test"] = &OAuthClient{ClientID: "jcmc_test", ClientName: "Claude"}

	store.seedToken("hash_a", OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use api.read.content",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	// A revoked token for another client must not surface a grant.
	store.seedToken("hash_b", OAuthToken{
		ClientID:  "jcmc_dead",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		RevokedAt: &pgtype.Timestamptz{Time: time.Now(), Valid: true},
	})
	// Another user's grant must not leak.
	store.seedToken("hash_c", OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    mustUUID("00000000-0000-0000-0000-0000000000ff"),
		Scope:     "agent.use",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me/oauth/grants", nil).
		WithContext(withUser(t.Context(), testUserID))
	rec := httptest.NewRecorder()
	h.HandleListGrants(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []grantView `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("grants = %d, want 1: %+v", len(resp.Data), resp.Data)
	}
	if resp.Data[0].ClientName != "Claude" || resp.Data[0].ClientID != "jcmc_test" {
		t.Fatalf("bad grant row: %+v", resp.Data[0])
	}
	if len(resp.Data[0].Scopes) != 2 {
		t.Fatalf("bad scopes: %v", resp.Data[0].Scopes)
	}
}

// §120 connected apps — DELETE revokes the whole client/user family.
func TestRevokeGrantHandler(t *testing.T) {
	h, _, store := newTestOAuth()

	for _, typ := range []string{"access", "refresh"} {
		store.seedToken("hash_"+typ, OAuthToken{
			ClientID:  "jcmc_test",
			UserID:    testUserID,
			Scope:     "agent.use",
			TokenType: typ,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
		})
	}
	// A token for a different client must survive.
	store.seedToken("hash_other", OAuthToken{
		ClientID:  "jcmc_other",
		UserID:    testUserID,
		Scope:     "agent.use",
		TokenType: "refresh",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("clientId", "jcmc_test")
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/me/oauth/grants/jcmc_test", nil).
		WithContext(context.WithValue(withUser(t.Context(), testUserID), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.HandleRevokeGrant(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	for hash, tok := range store.tokens {
		if tok.ClientID == "jcmc_test" && tok.RevokedAt == nil {
			t.Fatalf("family token %s not revoked", hash)
		}
		if tok.ClientID == "jcmc_other" && tok.RevokedAt != nil {
			t.Fatal("unrelated client's token was revoked")
		}
	}
}

func TestResolveAccessToken(t *testing.T) {
	svc := NewOAuthService(newFakeOAuthStore(), nil, nil)
	store := svc.store.(*fakeOAuthStore)

	tok := "jcpat_" + strings.Repeat("1", 64)
	store.seedToken(hashToken(tok), OAuthToken{
		ClientID:  "jcmc_test",
		UserID:    testUserID,
		Scope:     "agent.use api.read.content",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	userID, scopes, clientID, err := svc.ResolveAccessToken(context.Background(), tok)
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if userID != testUserID {
		t.Fatal("wrong user")
	}
	if clientID != "jcmc_test" {
		t.Fatal("wrong client")
	}
	if len(scopes) != 2 || scopes[0] != "agent.use" {
		t.Fatalf("bad scopes: %v", scopes)
	}

	// Revoked token must fail.
	_ = svc.RevokeToken(context.Background(), tok)
	if _, _, _, err := svc.ResolveAccessToken(context.Background(), tok); err == nil {
		t.Fatal("revoked token resolved")
	}

	// Unknown token must fail.
	if _, _, _, err := svc.ResolveAccessToken(context.Background(), "jcpat_"+strings.Repeat("9", 64)); err == nil {
		t.Fatal("unknown token resolved")
	}
}

func TestConsentToken(t *testing.T) {
	h, _, _ := newTestOAuth()
	p := &authorizeParams{
		ClientID:      "jcmc_test",
		RedirectURI:   "https://agent.example.com/cb",
		Scope:         "agent.use",
		CodeChallenge: "challenge",
	}

	tok := h.consentTokenValue(p, testUserID, time.Now().Add(time.Minute))
	if !h.verifyConsentToken(tok, p, testUserID) {
		t.Fatal("valid consent token rejected")
	}

	// Tampered scope must fail.
	p2 := *p
	p2.Scope = "agent.use api.read.*"
	if h.verifyConsentToken(tok, &p2, testUserID) {
		t.Fatal("consent token accepted after scope tampering")
	}

	// Different user must fail.
	other := mustUUID("00000000-0000-0000-0000-000000000099")
	if h.verifyConsentToken(tok, p, other) {
		t.Fatal("consent token accepted for wrong user")
	}

	// Expired must fail.
	expired := h.consentTokenValue(p, testUserID, time.Now().Add(-time.Minute))
	if h.verifyConsentToken(expired, p, testUserID) {
		t.Fatal("expired consent token accepted")
	}
}

// §120 v1.4: a client registered for agent access must carry agent.use in
// every grant — the scope ceiling can't drop below the client's intent.
func TestAuthorizeAgentClientRequiresAgentUse(t *testing.T) {
	h, _, store := newTestOAuth()
	_, challenge := pkcePair()

	store.clients["jcmc_test"] = &OAuthClient{
		ClientID:     "jcmc_test",
		RedirectURIs: []string{"https://agent.example.com/cb"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"agent.use", "api.read.content"},
	}

	u := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {"jcmc_test"},
		"redirect_uri":          {"https://agent.example.com/cb"},
		"scope":                 {"api.read.content"}, // agent.use dropped
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	req := httptest.NewRequest(http.MethodGet, u, nil)
	rec := httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	// client_id + redirect_uri validated → RFC 6749 error redirect, not a
	// page: the client learns the grant was rejected and why.
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 error redirect", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://agent.example.com/cb?") ||
		!strings.Contains(loc, "error=invalid_scope") {
		t.Fatalf("expected invalid_scope redirect, got %q", loc)
	}
	if !strings.Contains(loc, "error_description=") {
		t.Fatalf("expected error_description, got %q", loc)
	}
}

// §120 v1.4: non-agent clients (extension, identity-only) are not held to
// the agent.use requirement — an openid/digest grant proceeds to the
// login redirect instead of erroring.
func TestAuthorizeNonAgentClientNoAgentUseRequired(t *testing.T) {
	h, _, store := newTestOAuth()
	_, challenge := pkcePair()

	store.clients["jcmc_ext"] = &OAuthClient{
		ClientID:     "jcmc_ext",
		ClientName:   "JapaConnect Browser Extension",
		RedirectURIs: []string{"https://abcdefghijklmnop.chromiumapp.org/"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "api.read.digest", "api.read.me"},
	}

	u := "/oauth/authorize?" + url.Values{
		"response_type":         {"code"},
		"client_id":             {"jcmc_ext"},
		"redirect_uri":          {"https://abcdefghijklmnop.chromiumapp.org/"},
		"scope":                 {"openid api.read.digest"},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	req := httptest.NewRequest(http.MethodGet, u, nil)
	rec := httptest.NewRecorder()
	h.HandleAuthorize(rec, req)

	// Unauthenticated → 302 to login (not a 400 error page).
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 to login: %s", rec.Code, rec.Body.String())
	}
}

// §120 v1.4: the grant is intersected with the resource owner's own
// capability — agent.use requires the agent.use RBAC node.
func TestCheckUserCapability(t *testing.T) {
	h, _, _ := newTestOAuth()

	// Non-agent scope passes without a resolver.
	if err := h.checkUserCapability(t.Context(), "openid api.read.digest", testUserID); err != nil {
		t.Fatalf("non-agent scope rejected: %v", err)
	}

	// Agent scope without a resolver fails closed.
	if err := h.checkUserCapability(t.Context(), "agent.use api.read.content", testUserID); err == nil {
		t.Fatal("agent.use grant allowed without a permission resolver")
	}

	h.SetPermissionResolver(&stubPerms{granted: map[string]bool{}})
	if err := h.checkUserCapability(t.Context(), "agent.use api.read.content", testUserID); err == nil {
		t.Fatal("agent.use grant allowed for a user without the RBAC node")
	}

	h.SetPermissionResolver(&stubPerms{granted: map[string]bool{"agent.use": true}})
	if err := h.checkUserCapability(t.Context(), "agent.use api.read.content", testUserID); err != nil {
		t.Fatalf("agent.use grant rejected for a user holding the node: %v", err)
	}
}

// §120 v1.4: GET /oauth/userinfo — openid-scoped tokens resolve identity.
func TestUserinfoHandler(t *testing.T) {
	h, _, store := newTestOAuth()

	name := "Ada O"
	avatar := "https://cdn.example.com/a.png"
	store.identities[testUserID] = &OAuthUserIdentity{
		UserID:        testUserID,
		Email:         "ada@example.com",
		Username:      "ada",
		EmailVerified: true,
		DisplayName:   &name,
		AvatarURL:     &avatar,
	}

	tok := "jcpat_" + strings.Repeat("2", 64)
	store.seedToken(hashToken(tok), OAuthToken{
		ClientID:  "jcmc_ext",
		UserID:    testUserID,
		Scope:     "openid api.read.digest",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})
	noOpenID := "jcpat_" + strings.Repeat("3", 64)
	store.seedToken(hashToken(noOpenID), OAuthToken{
		ClientID:  "jcmc_ext",
		UserID:    testUserID,
		Scope:     "api.read.digest",
		TokenType: "access",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Hour), Valid: true},
	})

	// No bearer → 401.
	req := httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
	rec := httptest.NewRecorder()
	h.HandleUserinfo(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-token status = %d, want 401", rec.Code)
	}

	// Token without openid → 403.
	req = httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+noOpenID)
	rec = httptest.NewRecorder()
	h.HandleUserinfo(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no-openid status = %d, want 403", rec.Code)
	}

	// openid token → 200 claims.
	req = httptest.NewRequest(http.MethodGet, "/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec = httptest.NewRecorder()
	h.HandleUserinfo(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var claims map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &claims); err != nil {
		t.Fatal(err)
	}
	if claims["sub"] != testUserID.String() || claims["preferred_username"] != "ada" ||
		claims["name"] != name || claims["email"] != "ada@example.com" ||
		claims["email_verified"] != true || claims["picture"] != avatar {
		t.Fatalf("bad claims: %v", claims)
	}
}
