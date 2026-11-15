package atproto

// A6-5 full ATProto OAuth tests (§127) — unit coverage for the
// DPoP/nonce/JWK/PKCE/redirect machinery plus a DB end-to-end of the
// PAR → authorize-code → token → refresh-rotation → DPoP RS-auth →
// revoke lifecycle. DB tests skip without JC_TEST_DATABASE_URL like the
// rest of the package's integration suite.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	jdb "github.com/jedafe/japa/internal/db"
	gen "github.com/jedafe/japa/internal/db/gen"
)

// ---------------------------------------------------------------------------
// Helpers — P-256 keys, JWK maps, signed DPoP proofs, metadata stubs
// ---------------------------------------------------------------------------

func testECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa key: %v", err)
	}
	return k
}

func ecJWK(k *ecdsa.PublicKey) map[string]any {
	pad := func(b []byte) []byte {
		if len(b) < 32 {
			b = append(make([]byte, 32-len(b)), b...)
		}
		return b
	}
	return map[string]any{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pad(k.X.Bytes())),
		"y": base64.RawURLEncoding.EncodeToString(pad(k.Y.Bytes())),
	}
}

// signES256 assembles a minimal h.p.s JWT the handlers' verifier accepts.
func signES256(t *testing.T, key *ecdsa.PrivateKey, hdr, claims map[string]any) string {
	t.Helper()
	hj, _ := json.Marshal(hdr)
	cj, _ := json.Marshal(claims)
	h64 := base64.RawURLEncoding.EncodeToString(hj)
	c64 := base64.RawURLEncoding.EncodeToString(cj)
	digest := sha256.Sum256([]byte(h64 + "." + c64))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	sig := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	return h64 + "." + c64 + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func makeDPoP(t *testing.T, key *ecdsa.PrivateKey, htm, htu, nonce, ath string) string {
	t.Helper()
	claims := map[string]any{
		"jti": oauthRandom(""), "htm": htm, "htu": htu,
		"iat": float64(time.Now().Unix()),
	}
	if nonce != "" {
		claims["nonce"] = nonce
	}
	if ath != "" {
		claims["ath"] = ath
	}
	return signES256(t, key,
		map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": ecJWK(&key.PublicKey)},
		claims)
}

// oauthTestService wires the standard test Service with miniredis + a
// stubbed metadata fetch (client docs keyed by URL) so no real egress
// happens.
func oauthTestService(t *testing.T, d *jdb.DB, docs map[string]string) *Service {
	t.Helper()
	svc := newAtprotoService(t, d)
	svc.SetValkey(startTestCompat(t))
	svc.oauthFetch = func(_ context.Context, rawURL string, _ int64) ([]byte, int, error) {
		if body, ok := docs[rawURL]; ok {
			return []byte(body), http.StatusOK, nil
		}
		return nil, http.StatusNotFound, nil
	}
	return svc
}

const testClientID = "https://app.example.com/client-metadata.json"
const testRedirect = "https://app.example.com/callback"

func testClientDoc() string {
	doc, _ := json.Marshal(map[string]any{
		"client_id":                  testClientID,
		"client_name":                "Example App",
		"redirect_uris":              []string{testRedirect},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"scope":                      "atproto transition:generic transition:email",
		"token_endpoint_auth_method": "none",
		"application_type":           "web",
		"dpop_bound_access_tokens":   true,
	})
	return string(doc)
}

// ---------------------------------------------------------------------------
// Units — nonce, jkt, PKCE shape, redirect rules, scope, discrimination
// ---------------------------------------------------------------------------

func TestNormalizeHTU(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://api.dev:443/par", "api.dev/par"},
		{"http://api.dev:80/par", "api.dev/par"},
		{"https://api.dev/par", "api.dev/par"},
		{"https://api.dev:8443/par", "api.dev:8443/par"},
		{"http://localhost:8080/xrpc/x", "localhost:8080/xrpc/x"},
		{"https://[::1]:8443/par", "::1:8443/par"},
		{"https://api.dev/par?x=1", "api.dev/par"},  // query never binds
		{"https://api.dev/par/", "api.dev/par"},     // trailing slash folds
		{"https://api.dev//par", "api.dev/par"},     // double slash folds
		{"https://api.dev/x/../par", "api.dev/par"}, // dot-segments fold
		{"https://API.DEV/par", "api.dev/par"},      // host case-folds
		{"https://api.dev", "api.dev/"},
		{"not a url", ""},
	}
	for _, c := range cases {
		if got := normalizeHTU(c.in); got != c.want {
			t.Fatalf("normalizeHTU(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestClientBindingMatches(t *testing.T) {
	cases := []struct {
		stored, jkt string
		want        bool
	}{
		{"abc123", "abc123", true},                // bare jkt
		{"kid1|ES256|abc123", "abc123", true},     // legacy format, jkt segment
		{"renamed|ES256|abc123", "abc123", true},  // kid renamed, same key
		{"kid1|ES256|abc123", "different", false}, // different key
		{"abc123", "different", false},            // bare mismatch
		{"kid1|ES256|abc123", "", false},          // no assertion
	}
	for _, c := range cases {
		if got := clientBindingMatches(c.stored, c.jkt); got != c.want {
			t.Fatalf("clientBindingMatches(%q, %q) = %v, want %v", c.stored, c.jkt, got, c.want)
		}
	}
}

func TestOAuthNonceRoundTrip(t *testing.T) {
	svc := &Service{jwtKey: []byte("nonce-test")}
	n := svc.mintNonce()
	if !svc.validNonce(n) {
		t.Fatal("fresh nonce rejected")
	}
	if svc.validNonce(n + "x") {
		t.Fatal("tampered nonce accepted")
	}
	if svc.validNonce("999999999999.invalid") {
		t.Fatal("stale/forged nonce accepted")
	}
	other := &Service{jwtKey: []byte("different-secret")}
	if other.validNonce(n) {
		t.Fatal("nonce from another key accepted")
	}
}

func TestJWKThumbprintDeterministic(t *testing.T) {
	k := testECKey(t)
	j1, err := jwkThumbprint(ecJWK(&k.PublicKey))
	if err != nil {
		t.Fatalf("thumbprint: %v", err)
	}
	j2, _ := jwkThumbprint(ecJWK(&k.PublicKey))
	if j1 != j2 {
		t.Fatal("thumbprint not deterministic")
	}
	other := testECKey(t)
	j3, _ := jwkThumbprint(ecJWK(&other.PublicKey))
	if j1 == j3 {
		t.Fatal("distinct keys share a thumbprint")
	}
}

func TestValidateRedirectURI(t *testing.T) {
	web := &oauthClientDoc{
		RedirectURIs:    []string{"https://app.example.com/cb"},
		ApplicationType: "web",
	}
	if err := validateRedirectURI(web, testClientID, "https://app.example.com/cb"); err != nil {
		t.Fatalf("web https redirect rejected: %v", err)
	}
	if err := validateRedirectURI(web, testClientID, "https://other.example.com/x"); err == nil {
		t.Fatal("unregistered redirect accepted")
	}
	if err := validateRedirectURI(web, testClientID, "http://127.0.0.1/cb"); err == nil {
		t.Fatal("web client accepted loopback http")
	}
	native := &oauthClientDoc{
		RedirectURIs: []string{
			"http://127.0.0.1/cb", "com.example.app:/cb", "https://app.example.com/cb",
		},
		ApplicationType: "native",
	}
	if err := validateRedirectURI(native, testClientID, "http://127.0.0.1/cb"); err != nil {
		t.Fatalf("native loopback rejected: %v", err)
	}
	if err := validateRedirectURI(native, testClientID, "com.example.app:/cb"); err != nil {
		t.Fatalf("native custom scheme rejected: %v", err)
	}
	if err := validateRedirectURI(native, testClientID, "https://app.example.com/cb"); err != nil {
		t.Fatalf("native same-origin https rejected: %v", err)
	}
	if err := validateRedirectURI(native, testClientID, "http://evil.example.com/x"); err == nil {
		t.Fatal("native client accepted arbitrary http")
	}
}

func TestIsS256Challenge(t *testing.T) {
	sum := sha256.Sum256([]byte("verifier"))
	if !isS256Challenge(base64.RawURLEncoding.EncodeToString(sum[:])) {
		t.Fatal("real S256 challenge rejected")
	}
	if isS256Challenge("abc") || isS256Challenge("plain") {
		t.Fatal("non-S256 challenge accepted")
	}
}

func TestOAuthDiscriminators(t *testing.T) {
	if !IsAtprotoClientID("https://app.example.com/c.json") ||
		!IsAtprotoClientID("http://localhost") {
		t.Fatal("atproto client_id shapes missed")
	}
	if IsAtprotoClientID("jcmc_0123456789abcdef") || IsAtprotoClientID("") {
		t.Fatal("MCP client_id routed to atproto")
	}
	if !IsAtprotoToken("atp_at_x") || !IsAtprotoToken("atp_rt_y") {
		t.Fatal("atproto token prefixes missed")
	}
	if IsAtprotoToken("jcpat_x") || IsAtprotoToken("jcprt_y") {
		t.Fatal("MCP token routed to atproto")
	}
}

func TestScopeAllowed(t *testing.T) {
	doc := &oauthClientDoc{Scope: "atproto transition:generic"}
	if !scopeAllowed(doc, "atproto transition:generic") {
		t.Fatal("declared scope rejected")
	}
	if scopeAllowed(doc, "atproto transition:email") {
		t.Fatal("undeclared scope accepted")
	}
	if !scopeAllowed(&oauthClientDoc{}, "atproto anything") {
		t.Fatal("absent doc scope should not restrict")
	}
	if !hasScopeWord("atproto transition:email", "transition:email") ||
		hasScopeWord("atproto transition:generic", "transition:email") {
		t.Fatal("hasScopeWord broken")
	}
}

func TestConsentTokenRoundTrip(t *testing.T) {
	svc := &Service{jwtKey: []byte("consent-test")}
	var uid pgtype.UUID
	uid.Scan("11111111-2222-3333-4444-555555555555")
	tok := svc.consentTokenValue("urn:req:1", uid, time.Now().Add(time.Minute))
	if !svc.verifyConsentToken(tok, "urn:req:1", uid) {
		t.Fatal("consent token rejected")
	}
	if svc.verifyConsentToken(tok, "urn:req:2", uid) {
		t.Fatal("consent token accepted for a different request")
	}
	expired := svc.consentTokenValue("urn:req:1", uid, time.Now().Add(-time.Minute))
	if svc.verifyConsentToken(expired, "urn:req:1", uid) {
		t.Fatal("expired consent token accepted")
	}
}

// ---------------------------------------------------------------------------
// DB end-to-end — PAR → code → token → refresh → DPoP RS auth → revoke
// ---------------------------------------------------------------------------

func TestOAuthEndToEnd(t *testing.T) {
	ctx := context.Background()
	d := testDB(t)
	svc := oauthTestService(t, d, map[string]string{testClientID: testClientDoc()})
	h := NewHandlers(svc)

	userID, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("no atproto account for user: %v", err)
	}

	dpopKey := testECKey(t)
	verifier := oauthRandom("")
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
	hostPath := "api.test.dev/oauth/par"
	htu := "http://" + hostPath // httptest request has no TLS → http scheme

	// --- PAR without a server nonce → use_dpop_nonce + DPoP-Nonce header.
	parForm := "response_type=code&client_id=" + testClientID +
		"&redirect_uri=" + testRedirect + "&scope=atproto+transition:generic+transition:email" +
		"&state=st&code_challenge=" + challenge + "&code_challenge_method=S256"
	req := httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/par", strings.NewReader(parForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", htu, "", ""))
	rec := httptest.NewRecorder()
	h.HandlePAR(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("PAR without nonce: got %d body %s", rec.Code, rec.Body.String())
	}
	nonce := rec.Header().Get("DPoP-Nonce")
	if nonce == "" {
		t.Fatal("use_dpop_nonce response carried no DPoP-Nonce header")
	}

	// --- PAR with the fresh nonce → 201 + request_uri.
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/par", strings.NewReader(parForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", htu, nonce, ""))
	rec = httptest.NewRecorder()
	h.HandlePAR(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("PAR: got %d body %s", rec.Code, rec.Body.String())
	}
	var parResp struct {
		RequestURI string `json:"request_uri"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&parResp); err != nil || parResp.RequestURI == "" {
		t.Fatalf("bad PAR response: %v %q", err, rec.Body.String())
	}

	// --- Consent approve → code (the HTTP consent GET needs jc_token;
	// the CAS + redirect logic is exercised at the service layer here).
	row, err := svc.store.q.GetOAuthRequest(ctx, parResp.RequestURI)
	if err != nil {
		t.Fatalf("request row: %v", err)
	}
	code := oauthRandom("atp_cd_")
	ch := oauthHash(code)
	if _, err := svc.store.q.IssueOAuthCode(ctx, gen.IssueOAuthCodeParams{
		RequestUri: row.RequestUri, AccountID: acct.ID, CodeHash: &ch,
	}); err != nil {
		t.Fatalf("issue code: %v", err)
	}
	// Second approve on the same request_uri must be a no-op (CAS).
	if _, err := svc.store.q.IssueOAuthCode(ctx, gen.IssueOAuthCodeParams{
		RequestUri: row.RequestUri, AccountID: acct.ID, CodeHash: &ch,
	}); err == nil {
		t.Fatal("request_uri minted a second code")
	}

	// --- Code exchange → token pair bound to the DPoP key.
	tokenHTU := "http://api.test.dev/oauth/token"
	exForm := "grant_type=authorization_code&client_id=" + testClientID +
		"&code=" + code + "&redirect_uri=" + testRedirect + "&code_verifier=" + verifier
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/token", strings.NewReader(exForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", tokenHTU, svc.mintNonce(), ""))
	rec = httptest.NewRecorder()
	h.HandleToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("token exchange: got %d body %s", rec.Code, rec.Body.String())
	}
	var tok oauthTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&tok); err != nil {
		t.Fatalf("token decode: %v", err)
	}
	if tok.TokenType != "DPoP" || tok.AccessToken == "" || tok.RefreshToken == "" ||
		tok.Sub != acct.Did || !hasScopeWord(tok.Scope, "atproto") {
		t.Fatalf("bad token response: %+v", tok)
	}

	// --- Second grant BEFORE the code replay: the replay must revoke
	// only the grant its own request_uri minted — a sibling grant on
	// the same client×account survives.
	verifier2 := oauthRandom("")
	ch2sum := sha256.Sum256([]byte(verifier2))
	challenge2 := base64.RawURLEncoding.EncodeToString(ch2sum[:])
	jkt := mustJKT(t, dpopKey)
	row2uri := oauthRequestURIPrefix + oauthRandom("req_")
	// code_challenge is single-use: reusing grant 1's challenge fails.
	if _, err := svc.CreateOAuthRequest(ctx, &gen.CreateOAuthRequestParams{
		RequestUri: row2uri, ClientID: testClientID, RedirectUri: testRedirect,
		Scope: "atproto transition:generic", State: "s2",
		CodeChallenge: challenge, DpopJkt: jkt,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
	}); err == nil {
		t.Fatal("code_challenge reuse was not rejected")
	}
	if _, err := svc.CreateOAuthRequest(ctx, &gen.CreateOAuthRequestParams{
		RequestUri: row2uri, ClientID: testClientID, RedirectUri: testRedirect,
		Scope: "atproto transition:generic", State: "s2",
		CodeChallenge: challenge2, DpopJkt: jkt,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
	}); err != nil {
		t.Fatalf("second PAR: %v", err)
	}
	code2 := oauthRandom("atp_cd_")
	ch2 := oauthHash(code2)
	if _, err := svc.store.q.IssueOAuthCode(ctx, gen.IssueOAuthCodeParams{
		RequestUri: row2uri, AccountID: acct.ID, CodeHash: &ch2,
	}); err != nil {
		t.Fatalf("issue code 2: %v", err)
	}
	resp2, err := svc.ExchangeCode(ctx, code2, testClientID, "", testRedirect, verifier2, jkt)
	if err != nil {
		t.Fatalf("second exchange: %v", err)
	}

	// --- Code replay inside the reuse grace: a concurrent exchange
	// retry is benign — invalid_grant, but the minted grant stands and
	// sibling grants are untouched.
	replayCode := func() {
		req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/token", strings.NewReader(exForm))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", tokenHTU, svc.mintNonce(), ""))
		rec = httptest.NewRecorder()
		h.HandleToken(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("code replay: got %d body %s", rec.Code, rec.Body.String())
		}
	}
	replayCode()
	if grant, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(tok.AccessToken)); gerr != nil || grant.RevokedAt.Valid {
		t.Fatal("in-grace code replay revoked the minted grant")
	}

	// --- Past-grace replay = theft: backdate the code consumption,
	// replay again → the grant it minted dies, sibling survives.
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_oauth_requests SET code_used_at = now() - interval '1 minute' WHERE request_uri = $1`,
		parResp.RequestURI); err != nil {
		t.Fatalf("backdate code_used_at: %v", err)
	}
	replayCode()
	if grant, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(tok.AccessToken)); gerr != nil || !grant.RevokedAt.Valid {
		t.Fatal("past-grace code replay left the minted grant live")
	}
	if grant, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(resp2.AccessToken)); gerr != nil || grant.RevokedAt.Valid {
		t.Fatal("code replay revoked a sibling grant")
	}

	// --- Refresh grace: rotation → in-grace stale replay is a benign
	// concurrent retry — invalid_grant but the family stays live and
	// the rotated pair still rotates.
	rot1, err := svc.rotateGrant(ctx, resp2.RefreshToken, testClientID, "", jkt)
	if err != nil {
		t.Fatalf("refresh rotation: %v", err)
	}
	if rot1.RefreshToken == resp2.RefreshToken || rot1.AccessToken == resp2.AccessToken {
		t.Fatal("rotation did not issue a fresh pair")
	}
	if _, err := svc.rotateGrant(ctx, resp2.RefreshToken, testClientID, "", jkt); err == nil {
		t.Fatal("stale refresh accepted")
	}
	rot2, err := svc.rotateGrant(ctx, rot1.RefreshToken, testClientID, "", jkt)
	if err != nil {
		t.Fatalf("in-grace stale replay revoked the family: %v", err)
	}

	// --- Theft path: backdate the rotation past the grace window —
	// the same stale-hash replay now revokes the family.
	grant, err := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(rot2.AccessToken))
	if err != nil {
		t.Fatalf("grant lookup: %v", err)
	}
	if _, err := d.Writer.Exec(ctx,
		`UPDATE atproto_oauth_tokens SET refresh_rotated_at = now() - interval '1 minute' WHERE id = $1`,
		grant.ID); err != nil {
		t.Fatalf("backdate refresh_rotated_at: %v", err)
	}
	if _, err := svc.rotateGrant(ctx, rot1.RefreshToken, testClientID, "", jkt); err == nil {
		t.Fatal("past-grace stale refresh accepted")
	}
	if _, err := svc.rotateGrant(ctx, rot2.RefreshToken, testClientID, "", jkt); err == nil {
		t.Fatal("family survived a past-grace stale-hash replay")
	}

	// --- Fresh grant for the DPoP RS path (own PKCE pair — challenges
	// are single-use by constraint).
	verifier3 := oauthRandom("")
	ch3sum := sha256.Sum256([]byte(verifier3))
	challenge3 := base64.RawURLEncoding.EncodeToString(ch3sum[:])
	row3uri := oauthRequestURIPrefix + oauthRandom("req_")
	if _, err := svc.CreateOAuthRequest(ctx, &gen.CreateOAuthRequestParams{
		RequestUri: row3uri, ClientID: testClientID, RedirectUri: testRedirect,
		Scope: "atproto transition:generic transition:email", State: "s3",
		CodeChallenge: challenge3, DpopJkt: jkt,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(time.Minute), Valid: true},
	}); err != nil {
		t.Fatalf("third PAR: %v", err)
	}
	code3 := oauthRandom("atp_cd_")
	ch3 := oauthHash(code3)
	if _, err := svc.store.q.IssueOAuthCode(ctx, gen.IssueOAuthCodeParams{
		RequestUri: row3uri, AccountID: acct.ID, CodeHash: &ch3,
	}); err != nil {
		t.Fatalf("issue code 3: %v", err)
	}
	resp3, err := svc.ExchangeCode(ctx, code3, testClientID, "", testRedirect, verifier3, jkt)
	if err != nil {
		t.Fatalf("third exchange: %v", err)
	}

	athSum := sha256.Sum256([]byte(resp3.AccessToken))
	ath := base64.RawURLEncoding.EncodeToString(athSum[:])
	rsReq := httptest.NewRequest(http.MethodGet, "http://api.test.dev/xrpc/com.atproto.server.getSession", nil)
	rsReq.Header.Set("Authorization", "DPoP "+resp3.AccessToken)
	rsReq.Header.Set("DPoP", makeDPoP(t, dpopKey, "GET",
		"http://api.test.dev/xrpc/com.atproto.server.getSession", svc.mintNonce(), ath))
	rsRec := httptest.NewRecorder()
	acctOut, err := svc.AuthenticateDPoP(ctx, rsReq, rsRec)
	if err != nil || acctOut.DID != acct.Did {
		t.Fatalf("DPoP RS auth failed: %v", err)
	}
	if !hasScopeWord(strings.Join(acctOut.OAuthScopes, " "), "transition:email") {
		t.Fatal("OAuth scope not surfaced on the session account")
	}

	// Wrong-key proof must fail (jkt binding).
	wrongKey := testECKey(t)
	rsReq2 := httptest.NewRequest(http.MethodGet, "http://api.test.dev/xrpc/com.atproto.server.getSession", nil)
	rsReq2.Header.Set("Authorization", "DPoP "+resp3.AccessToken)
	rsReq2.Header.Set("DPoP", makeDPoP(t, wrongKey, "GET",
		"http://api.test.dev/xrpc/com.atproto.server.getSession", svc.mintNonce(), ath))
	if _, err := svc.AuthenticateDPoP(ctx, rsReq2, httptest.NewRecorder()); err == nil {
		t.Fatal("foreign-key DPoP proof accepted")
	}

	// Member grant management: list + revoke the client family.
	grants, err := svc.ListOAuthGrants(ctx, acct.ID)
	if err != nil || len(grants) == 0 {
		t.Fatalf("grant list: %v %d", err, len(grants))
	}
	if err := svc.RevokeOAuthGrantForClient(ctx, acct.ID, testClientID); err != nil {
		t.Fatalf("revoke family: %v", err)
	}
	if g, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(resp3.AccessToken)); gerr != nil || !g.RevokedAt.Valid {
		t.Fatal("family revoke left a live grant")
	}
}

func mustJKT(t *testing.T, k *ecdsa.PrivateKey) string {
	t.Helper()
	j, err := jwkThumbprint(ecJWK(&k.PublicKey))
	if err != nil {
		t.Fatalf("jkt: %v", err)
	}
	return j
}

// ---------------------------------------------------------------------------
// Confidential client — private_key_jwt end-to-end, incl. the client-doc
// cache-hit path (regression: resolved jwks used to vanish across the
// Valkey round-trip) and fail-closed confidential revocation.
// ---------------------------------------------------------------------------

func TestOAuthConfidentialClient(t *testing.T) {
	ctx := context.Background()
	d := testDB(t)

	clientKey := testECKey(t)
	const confClientID = "https://conf.example.com/client-metadata.json"
	const confRedirect = "https://conf.example.com/cb"
	cjwk := ecJWK(&clientKey.PublicKey)
	cjwk["kid"] = "k1"
	confDoc, _ := json.Marshal(map[string]any{
		"client_id":                  confClientID,
		"redirect_uris":              []string{confRedirect},
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"scope":                      "atproto transition:generic",
		"token_endpoint_auth_method": "private_key_jwt",
		"application_type":           "web",
		"dpop_bound_access_tokens":   true,
		"jwks":                       map[string]any{"keys": []map[string]any{cjwk}},
	})
	svc := oauthTestService(t, d, map[string]string{confClientID: string(confDoc)})
	h := NewHandlers(svc)

	userID, _ := newAtprotoUser(t, ctx, d, "active")
	if _, err := svc.EnableAccount(ctx, userID); err != nil {
		t.Fatalf("EnableAccount: %v", err)
	}
	acct, err := svc.store.accountByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("no atproto account for user: %v", err)
	}

	assertion := func() string {
		return signES256(t, clientKey,
			map[string]any{"typ": "JWT", "alg": "ES256", "kid": "k1"},
			map[string]any{
				"iss": confClientID, "sub": confClientID,
				"aud": "http://api.test.dev",
				"iat": float64(time.Now().Unix()),
				"exp": float64(time.Now().Add(2 * time.Minute).Unix()),
				"jti": oauthRandom(""),
			})
	}

	// Fetch twice — the second call is a cache hit; the assertion must
	// still verify against the rehydrated jwks.
	if _, err := svc.fetchClientDoc(ctx, confClientID); err != nil {
		t.Fatalf("first client-doc fetch: %v", err)
	}
	doc, err := svc.fetchClientDoc(ctx, confClientID)
	if err != nil {
		t.Fatalf("cached client-doc fetch: %v", err)
	}
	clientJKT, err := svc.verifyClientAssertion(ctx, doc, "http://api.test.dev", clientAssertionType, assertion())
	if err != nil || clientJKT == "" {
		t.Fatalf("assertion against cached doc: %v", err)
	}

	// --- PAR + exchange + refresh all carry the assertion.
	dpopKey := testECKey(t)
	verifier := oauthRandom("")
	challengeSum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeSum[:])
	parForm := "response_type=code&client_id=" + confClientID +
		"&redirect_uri=" + confRedirect + "&scope=atproto+transition:generic" +
		"&state=cf&code_challenge=" + challenge + "&code_challenge_method=S256" +
		"&client_assertion_type=" + clientAssertionType +
		"&client_assertion=" + assertion()
	req := httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/par", strings.NewReader(parForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", "http://api.test.dev/oauth/par", svc.mintNonce(), ""))
	rec := httptest.NewRecorder()
	h.HandlePAR(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("confidential PAR: got %d body %s", rec.Code, rec.Body.String())
	}
	var parResp struct {
		RequestURI string `json:"request_uri"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&parResp); err != nil || parResp.RequestURI == "" {
		t.Fatalf("bad PAR response: %v %q", err, rec.Body.String())
	}

	code := oauthRandom("atp_cd_")
	ch := oauthHash(code)
	if _, err := svc.store.q.IssueOAuthCode(ctx, gen.IssueOAuthCodeParams{
		RequestUri: parResp.RequestURI, AccountID: acct.ID, CodeHash: &ch,
	}); err != nil {
		t.Fatalf("issue code: %v", err)
	}
	// Missing assertion on the exchange must be refused.
	exForm := "grant_type=authorization_code&client_id=" + confClientID +
		"&code=" + code + "&redirect_uri=" + confRedirect + "&code_verifier=" + verifier
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/token", strings.NewReader(exForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", "http://api.test.dev/oauth/token", svc.mintNonce(), ""))
	rec = httptest.NewRecorder()
	h.HandleToken(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("exchange without assertion: got %d body %s", rec.Code, rec.Body.String())
	}
	// With the assertion → tokens on a confidential grant.
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/token",
		strings.NewReader(exForm+"&client_assertion_type="+clientAssertionType+"&client_assertion="+assertion()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", "http://api.test.dev/oauth/token", svc.mintNonce(), ""))
	rec = httptest.NewRecorder()
	h.HandleToken(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confidential exchange: got %d body %s", rec.Code, rec.Body.String())
	}
	var tok oauthTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&tok); err != nil {
		t.Fatalf("token decode: %v", err)
	}
	grant, err := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(tok.AccessToken))
	if err != nil || grant.ClientJkt == nil || *grant.ClientJkt != clientJKT {
		t.Fatal("grant did not record the client binding")
	}

	// Refresh with a WRONG client key's assertion → binding mismatch.
	otherKey := testECKey(t)
	otherAssertion := signES256(t, otherKey,
		map[string]any{"typ": "JWT", "alg": "ES256", "kid": "k9"},
		map[string]any{
			"iss": confClientID, "sub": confClientID, "aud": "http://api.test.dev",
			"iat": float64(time.Now().Unix()), "exp": float64(time.Now().Add(time.Minute).Unix()),
			"jti": oauthRandom(""),
		})
	rfForm := "grant_type=refresh_token&client_id=" + confClientID +
		"&refresh_token=" + tok.RefreshToken +
		"&client_assertion_type=" + clientAssertionType + "&client_assertion=" + otherAssertion
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/token", strings.NewReader(rfForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("DPoP", makeDPoP(t, dpopKey, "POST", "http://api.test.dev/oauth/token", svc.mintNonce(), ""))
	rec = httptest.NewRecorder()
	h.HandleToken(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatal("refresh accepted a foreign-key assertion")
	}

	// --- Revoke without an assertion → refused (fail closed on a
	// confidential grant).
	revForm := "token=" + tok.RefreshToken + "&client_id=" + confClientID
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/revoke", strings.NewReader(revForm))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.HandleRevoke(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoke without assertion: got %d body %s", rec.Code, rec.Body.String())
	}
	if g, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(tok.AccessToken)); gerr != nil || g.RevokedAt.Valid {
		t.Fatal("unauthenticated revoke touched the grant")
	}

	// With the assertion → 200 and the grant dies.
	req = httptest.NewRequest(http.MethodPost, "http://api.test.dev/oauth/revoke",
		strings.NewReader(revForm+"&client_assertion_type="+clientAssertionType+"&client_assertion="+assertion()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec = httptest.NewRecorder()
	h.HandleRevoke(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("confidential revoke: got %d body %s", rec.Code, rec.Body.String())
	}
	if g, gerr := svc.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(tok.AccessToken)); gerr != nil || !g.RevokedAt.Valid {
		t.Fatal("confidential revoke left the grant live")
	}
}
