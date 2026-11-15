// A6-5 full ATProto OAuth — service core (§127).
//
// The atproto OAuth profile mandates PAR, DPoP-bound tokens with
// server-issued nonces, client-metadata discovery, and a bare-origin AS
// issuer. The issuer shares the host with the §120 MCP OAuth server —
// the endpoints unify (§127 A6-5 layout) and requests discriminate by
// shape. This file owns everything the handlers need that is not HTTP:
// client metadata fetch/cache + validation, DPoP proof verification,
// nonce mint/check, private_key_jwt assertions, and the opaque grant
// store (issue / rotate / revoke / member management).
package atproto

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	gen "github.com/jedafe/japa/internal/db/gen"
)

const (
	oauthAccessTokenPrefix  = "atp_at_"
	oauthRefreshTokenPrefix = "atp_rt_"
	oauthRequestURIPrefix   = "urn:ietf:params:oauth:request_uri:"

	// Profile ceilings: access tokens are opaque but revocable, so the
	// ≤30-minute bound applies; 15 minutes leaves headroom for client
	// clock skew and matches the profile's conservative end.
	oauthAccessTTL         = 15 * time.Minute
	oauthRefreshPublicTTL  = 14 * 24 * time.Hour
	oauthRefreshConfidTTL  = 180 * 24 * time.Hour
	oauthPARTTL            = 10 * time.Minute
	oauthCodeExchangeTTL   = 5 * time.Minute // enforced by ConsumeOAuthCode
	oauthDPoPMaxSkew       = 5 * time.Minute
	oauthNonceMaxAge       = 10 * time.Minute
	oauthClientCacheTTL    = time.Hour
	oauthClientMaxDocBytes = 32 << 10
	oauthJWKSMaxBytes      = 64 << 10
	oauthConsentTokenTTL   = 10 * time.Minute

	clientAssertionType = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
)

var (
	ErrOAuthInvalidRequest = errors.New("invalid_request")
	ErrOAuthInvalidClient  = errors.New("invalid_client")
	ErrOAuthInvalidGrant   = errors.New("invalid_grant")
	ErrOAuthInvalidScope   = errors.New("invalid_scope")
	ErrOAuthDPoP           = errors.New("invalid_dpop_proof")
	ErrOAuthNonce          = errors.New("use_dpop_nonce")
)

// oauthError wraps a sentinel with the human description the wire carries.
type oauthError struct {
	code string
	desc string
}

func (e *oauthError) Error() string { return e.desc }
func (e *oauthError) Unwrap() error {
	switch e.code {
	case "invalid_request":
		return ErrOAuthInvalidRequest
	case "invalid_client":
		return ErrOAuthInvalidClient
	case "invalid_grant":
		return ErrOAuthInvalidGrant
	case "invalid_scope":
		return ErrOAuthInvalidScope
	case "invalid_dpop_proof":
		return ErrOAuthDPoP
	case "use_dpop_nonce":
		return ErrOAuthNonce
	}
	return ErrOAuthInvalidRequest
}

func oauthErr(code, desc string) *oauthError { return &oauthError{code: code, desc: desc} }

// ---------------------------------------------------------------------------
// Small primitives — random tokens, hashes, ES256 JWKs
// ---------------------------------------------------------------------------

func oauthRandom(prefix string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

func oauthHash(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

// ecPublicKeyFromJWK builds a P-256 public key from a decoded JWK map —
// the only key shape the profile permits (ES256).
func ecPublicKeyFromJWK(jwk map[string]any) (*ecdsa.PublicKey, error) {
	kty, _ := jwk["kty"].(string)
	crv, _ := jwk["crv"].(string)
	xs, _ := jwk["x"].(string)
	ys, _ := jwk["y"].(string)
	if kty != "EC" || crv != "P-256" || xs == "" || ys == "" {
		return nil, fmt.Errorf("jwk is not an EC P-256 key")
	}
	xb, err := base64.RawURLEncoding.DecodeString(xs)
	if err != nil {
		return nil, err
	}
	yb, err := base64.RawURLEncoding.DecodeString(ys)
	if err != nil {
		return nil, err
	}
	x := new(big.Int).SetBytes(xb)
	y := new(big.Int).SetBytes(yb)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, fmt.Errorf("jwk point not on P-256")
	}
	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}

// jwkThumbprint computes the RFC 7638 thumbprint of an EC JWK —
// {"crv","kty","x","y"} in lexicographic order, sha256, b64url. This is
// the jkt the PAR row and every grant bind to.
func jwkThumbprint(jwk map[string]any) (string, error) {
	crv, _ := jwk["crv"].(string)
	kty, _ := jwk["kty"].(string)
	x, _ := jwk["x"].(string)
	y, _ := jwk["y"].(string)
	if crv == "" || kty == "" || x == "" || y == "" {
		return "", fmt.Errorf("jwk missing members")
	}
	canonical := fmt.Sprintf(`{"crv":%q,"kty":%q,"x":%q,"y":%q}`, crv, kty, x, y)
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// verifyES256JWT splits "h.p.s", checks alg=ES256 (and typ when wantTyp
// is non-empty), and verifies the signature against pub. Returns the
// decoded header + claims maps.
func verifyES256JWT(tok, wantTyp string, pub *ecdsa.PublicKey) (map[string]any, map[string]any, error) {
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return nil, nil, fmt.Errorf("malformed jwt")
	}
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, err
	}
	var hdr map[string]any
	if json.Unmarshal(hdrRaw, &hdr) != nil {
		return nil, nil, fmt.Errorf("bad jwt header")
	}
	if alg, _ := hdr["alg"].(string); alg != "ES256" {
		return nil, nil, fmt.Errorf("jwt alg must be ES256")
	}
	if wantTyp != "" {
		if typ, _ := hdr["typ"].(string); !strings.EqualFold(typ, wantTyp) {
			return nil, nil, fmt.Errorf("jwt typ must be %s", wantTyp)
		}
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, err
	}
	var claims map[string]any
	if json.Unmarshal(claimsRaw, &claims) != nil {
		return nil, nil, fmt.Errorf("bad jwt claims")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, nil, fmt.Errorf("bad jwt signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return nil, nil, fmt.Errorf("jwt signature invalid")
	}
	return hdr, claims, nil
}

// ---------------------------------------------------------------------------
// DPoP — proof verification, server nonces
// ---------------------------------------------------------------------------

// dpopProof is a verified DPoP JWT: the sender's key thumbprint plus the
// claims the caller checked (ath binding is verified by the caller,
// which owns the access token).
type dpopProof struct {
	jkt   string
	jti   string
	nonce string
	ath   string
}

// requestHTU is the URL a DPoP proof must name — the profile compares
// against what the client sent, so either http or https scheme is
// accepted when the host+path match (a TLS-terminating proxy would
// otherwise false-reject every proof).
func requestHTU(r *http.Request) (string, string) {
	scheme := "https"
	if p := r.Header.Get("X-Forwarded-Proto"); p == "http" || p == "https" {
		scheme = p
	} else if r.URL != nil && r.URL.Scheme != "" {
		scheme = r.URL.Scheme
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		host = h
	}
	return scheme + "://" + host + r.URL.Path, host + r.URL.Path
}

// verifyDPoP validates the DPoP header on r. htm must match the method,
// htu must match scheme+host+path (either scheme — see requestHTU), iat
// within ±5min, and jti is replay-cached for the nonce window. ath is
// checked by the caller when an access token rides the request.
func (s *Service) verifyDPoP(ctx context.Context, r *http.Request) (*dpopProof, error) {
	raw := r.Header.Get("DPoP")
	if raw == "" {
		return nil, oauthErr("invalid_dpop_proof", "DPoP header is required")
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return nil, oauthErr("invalid_dpop_proof", "malformed DPoP proof")
	}
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, oauthErr("invalid_dpop_proof", "malformed DPoP proof")
	}
	var hdr map[string]any
	if json.Unmarshal(hdrRaw, &hdr) != nil {
		return nil, oauthErr("invalid_dpop_proof", "malformed DPoP proof")
	}
	if alg, _ := hdr["alg"].(string); alg != "ES256" {
		return nil, oauthErr("invalid_dpop_proof", "DPoP alg must be ES256")
	}
	if typ, _ := hdr["typ"].(string); !strings.EqualFold(typ, "dpop+jwt") {
		return nil, oauthErr("invalid_dpop_proof", "DPoP typ must be dpop+jwt")
	}
	jwk, _ := hdr["jwk"].(map[string]any)
	pub, err := ecPublicKeyFromJWK(jwk)
	if err != nil {
		return nil, oauthErr("invalid_dpop_proof", "DPoP jwk must be an EC P-256 key")
	}
	jkt, err := jwkThumbprint(jwk)
	if err != nil {
		return nil, oauthErr("invalid_dpop_proof", "DPoP jwk must be an EC P-256 key")
	}
	var claims map[string]any
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, oauthErr("invalid_dpop_proof", "bad DPoP signature")
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || json.Unmarshal(claimsRaw, &claims) != nil {
		return nil, oauthErr("invalid_dpop_proof", "bad DPoP claims")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:],
		new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, oauthErr("invalid_dpop_proof", "DPoP signature invalid")
	}

	htm, _ := claims["htm"].(string)
	if !strings.EqualFold(htm, r.Method) {
		return nil, oauthErr("invalid_dpop_proof", "DPoP htm does not match the request method")
	}
	htu, _ := claims["htu"].(string)
	full, hostPath := requestHTU(r)
	if htu != full && htu != "https://"+hostPath && htu != "http://"+hostPath {
		return nil, oauthErr("invalid_dpop_proof", "DPoP htu does not match the request URL")
	}
	iatf, ok := claims["iat"].(float64)
	if !ok {
		return nil, oauthErr("invalid_dpop_proof", "DPoP iat is required")
	}
	iat := time.Unix(int64(iatf), 0)
	if d := time.Since(iat); d > oauthDPoPMaxSkew || d < -oauthDPoPMaxSkew {
		return nil, oauthErr("invalid_dpop_proof", "DPoP iat is outside the acceptance window")
	}
	jti, _ := claims["jti"].(string)
	if jti == "" || len(jti) > 128 {
		return nil, oauthErr("invalid_dpop_proof", "DPoP jti is required")
	}
	// Replay guard — the proof is only fresh while its nonce is, so the
	// cache window tracks the nonce max age, not the 5min iat skew.
	if s.vk != nil {
		ok, err := s.vk.SetNX(ctx, "jc:atp:oauth:dpop:"+jti, "1", oauthNonceMaxAge).Result()
		if err == nil && !ok {
			return nil, oauthErr("invalid_dpop_proof", "DPoP proof already used")
		}
	}
	nonce, _ := claims["nonce"].(string)
	ath, _ := claims["ath"].(string)
	return &dpopProof{jkt: jkt, jti: jti, nonce: nonce, ath: ath}, nil
}

// checkNonce enforces the server-issued nonce the profile mandates.
// Nonces are stateless HMAC-signed minute buckets — a fresh proof must
// carry one minted within oauthNonceMaxAge.
func (s *Service) checkNonce(proof *dpopProof) error {
	if proof.nonce == "" || !s.validNonce(proof.nonce) {
		return oauthErr("use_dpop_nonce", "a fresh server nonce is required")
	}
	return nil
}

// mintNonce produces the stateless nonce: minute bucket + HMAC keyed on
// the session secret. Stateless so no Valkey write rides every request.
func (s *Service) mintNonce() string {
	bucket := strconv.FormatInt(time.Now().Unix()/60, 10)
	mac := hmac.New(sha256.New, s.jwtKey)
	mac.Write([]byte("atpnonce|" + bucket))
	return bucket + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
}

func (s *Service) validNonce(nonce string) bool {
	parts := strings.Split(nonce, ".")
	if len(parts) != 2 {
		return false
	}
	bucket, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false
	}
	age := time.Now().Unix()/60 - bucket
	if age < 0 || age > int64(oauthNonceMaxAge/time.Minute) {
		return false
	}
	mac := hmac.New(sha256.New, s.jwtKey)
	mac.Write([]byte("atpnonce|" + parts[0]))
	expected := base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:16])
	return hmac.Equal([]byte(parts[1]), []byte(expected))
}

func (s *Service) setNonceHeader(w http.ResponseWriter) {
	w.Header().Set("DPoP-Nonce", s.mintNonce())
}

// ---------------------------------------------------------------------------
// Client metadata — fetch, cache, validate
// ---------------------------------------------------------------------------

type oauthClientDoc struct {
	ClientID                string     `json:"client_id"`
	ClientName              string     `json:"client_name"`
	ClientURI               string     `json:"client_uri"`
	RedirectURIs            []string   `json:"redirect_uris"`
	GrantTypes              []string   `json:"grant_types"`
	Scope                   string     `json:"scope"`
	TokenEndpointAuthMethod string     `json:"token_endpoint_auth_method"`
	ApplicationType         string     `json:"application_type"`
	DPoPBoundAccessTokens   bool       `json:"dpop_bound_access_tokens"`
	JWKS                    *oauthJWKS `json:"jwks"`
	JWKSUri                 string     `json:"jwks_uri"`
	// jwksResolved is JWKSUri's fetched key set — folded into JWKS before
	// the doc is cached so a cache hit needs no refetch.
	jwksResolved []map[string]any
}

type oauthJWKS struct {
	Keys []map[string]any `json:"keys"`
}

func (d *oauthClientDoc) confidential() bool {
	return d.TokenEndpointAuthMethod == "private_key_jwt"
}

// fetchClientDoc retrieves and validates the client_id metadata document.
// The client_id is itself the doc URL — https only (the profile's
// http://localhost dev loop is a bare host, no port). The fetch follows
// no redirects, caps the body, and caches the parsed doc for an hour.
func (s *Service) fetchClientDoc(ctx context.Context, clientID string) (*oauthClientDoc, error) {
	u, err := url.Parse(clientID)
	if err != nil || u.Host == "" || u.Fragment != "" || u.User != nil {
		return nil, oauthErr("invalid_client_id", "client_id must be a URL")
	}
	localhost := u.Scheme == "http" && u.Hostname() == "localhost" && u.Port() == ""
	if u.Scheme != "https" && !localhost {
		return nil, oauthErr("invalid_client_id", "client_id must be an https URL")
	}
	cacheKey := "jc:atp:oauth:client:" + oauthHash(clientID)
	if s.vk != nil {
		if raw, err := s.vk.Get(ctx, cacheKey).Bytes(); err == nil {
			var doc oauthClientDoc
			if json.Unmarshal(raw, &doc) == nil && doc.ClientID == clientID {
				// jwksResolved is unexported — rehydrate it from the
				// serialized JWKS (resolved keys are folded in at fetch).
				if doc.JWKS != nil {
					doc.jwksResolved = doc.JWKS.Keys
				}
				return &doc, nil
			}
		}
	}
	doc, err := s.getClientDoc(ctx, clientID)
	if err != nil {
		return nil, err
	}
	if doc.ClientID != clientID {
		return nil, oauthErr("invalid_client", "client metadata does not echo the client_id")
	}
	if doc.confidential() {
		if err := s.resolveJWKS(ctx, doc); err != nil {
			return nil, err
		}
		// Fold resolved jwks_uri keys into the doc so the cached copy
		// never needs the second fetch.
		if doc.JWKS == nil && len(doc.jwksResolved) > 0 {
			doc.JWKS = &oauthJWKS{Keys: doc.jwksResolved}
		}
	}
	if s.vk != nil {
		if raw, err := json.Marshal(doc); err == nil {
			_ = s.vk.Set(ctx, cacheKey, raw, oauthClientCacheTTL).Err()
		}
	}
	return doc, nil
}

func (s *Service) getClientDoc(ctx context.Context, clientID string) (*oauthClientDoc, error) {
	body, status, err := s.fetchURL(ctx, clientID, oauthClientMaxDocBytes)
	if err != nil {
		return nil, oauthErr("invalid_client_id", "client metadata is unreachable")
	}
	if status != http.StatusOK {
		return nil, oauthErr("invalid_client_id", "client metadata fetch failed")
	}
	var doc oauthClientDoc
	if json.Unmarshal(body, &doc) != nil {
		return nil, oauthErr("invalid_client", "client metadata is not a valid document")
	}
	return &doc, nil
}

// fetchURL is the single guarded egress point for client metadata +
// jwks_uri — https enforced by the callers' URL validation, no
// redirects, 10s cap, body capped at max. Tests substitute the hook.
func (s *Service) fetchURL(ctx context.Context, rawURL string, maxBytes int64) ([]byte, int, error) {
	if s.oauthFetch != nil {
		return s.oauthFetch(ctx, rawURL, maxBytes)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(nil, resp.Body, maxBytes))
	return body, resp.StatusCode, err
}

// resolveJWKS fills doc.jwksKeys from the inline jwks or jwks_uri —
// the latter fetched with the same guards as the metadata doc.
func (s *Service) resolveJWKS(ctx context.Context, doc *oauthClientDoc) error {
	if doc.JWKS != nil && len(doc.JWKS.Keys) > 0 {
		doc.jwksResolved = doc.JWKS.Keys
		return nil
	}
	if doc.JWKSUri == "" {
		return oauthErr("invalid_client", "confidential client declares no jwks or jwks_uri")
	}
	u, err := url.Parse(doc.JWKSUri)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return oauthErr("invalid_client", "client jwks_uri must be an https URL")
	}
	body, status, err := s.fetchURL(ctx, doc.JWKSUri, oauthJWKSMaxBytes)
	if err != nil || status != http.StatusOK {
		return oauthErr("invalid_client", "client jwks_uri is unreachable")
	}
	var jwks oauthJWKS
	if json.Unmarshal(body, &jwks) != nil {
		return oauthErr("invalid_client", "client jwks is not a valid document")
	}
	doc.jwksResolved = jwks.Keys
	return nil
}

// validateRedirectURI applies the profile's redirect rules: the URI must
// be in the doc's redirect_uris, and its shape depends on the declared
// application_type — web clients redirect https-only; native clients may
// use a loopback http URI or a custom scheme in reverse-domain order to
// the client_id host (plus an https same-origin URI).
func validateRedirectURI(doc *oauthClientDoc, clientID, redirectURI string) error {
	found := false
	for _, ru := range doc.RedirectURIs {
		if ru == redirectURI {
			found = true
			break
		}
	}
	if !found {
		return oauthErr("invalid_request", "redirect_uri is not in the client metadata")
	}
	ru, err := url.Parse(redirectURI)
	if err != nil || ru.Scheme == "" || ru.User != nil || ru.Fragment != "" {
		return oauthErr("invalid_request", "redirect_uri is malformed")
	}
	cu, _ := url.Parse(clientID)
	if doc.ApplicationType == "native" {
		switch {
		case ru.Scheme == "https" && strings.EqualFold(ru.Hostname(), cu.Hostname()):
			return nil
		case ru.Scheme == "http" && isLoopbackHost(ru.Hostname()):
			return nil
		case ru.Scheme != "http" && ru.Scheme != "https":
			// Custom-scheme deep link — the scheme must be the client_id
			// host's labels in reverse order (app.example.com →
			// com.example.app:...; both scheme:/path and scheme:opaque
			// parse shapes count).
			if reversedHost(cu.Hostname()) == ru.Scheme {
				return nil
			}
		}
		return oauthErr("invalid_request", "redirect_uri is not valid for a native client")
	}
	if ru.Scheme == "https" {
		return nil
	}
	if ru.Scheme == "http" && isLoopbackHost(ru.Hostname()) && cu.Scheme == "http" {
		return nil // localhost dev loop
	}
	return oauthErr("invalid_request", "redirect_uri must be https")
}

func isLoopbackHost(h string) bool {
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "[::1]"
}

func reversedHost(host string) string {
	parts := strings.Split(host, ".")
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, ".")
}

// scopeAllowed checks the requested scope against the doc's declared
// scope string (space-separated) — absent declaration means no client
// ceiling beyond the mandatory `atproto` presence the caller checks.
func scopeAllowed(doc *oauthClientDoc, scope string) bool {
	if doc.Scope == "" {
		return true
	}
	declared := make(map[string]struct{})
	for _, s := range strings.Fields(doc.Scope) {
		declared[s] = struct{}{}
	}
	for _, s := range strings.Fields(scope) {
		if _, ok := declared[s]; !ok {
			return false
		}
	}
	return true
}

func hasScopeWord(scope, want string) bool {
	for _, s := range strings.Fields(scope) {
		if s == want {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// private_key_jwt — confidential-client authentication
// ---------------------------------------------------------------------------

// verifyClientAssertion validates the client_assertion pair against the
// client doc's key set and returns the assertion key's binding string
// (kid|alg|jkt) stored on the grant — refresh and revoke must present a
// matching binding.
func (s *Service) verifyClientAssertion(ctx context.Context, doc *oauthClientDoc, issuer, assertType, assertion string) (string, error) {
	if !doc.confidential() {
		return "", oauthErr("invalid_client", "this client does not use client authentication")
	}
	if assertType != clientAssertionType {
		return "", oauthErr("invalid_client", "unsupported client_assertion_type")
	}
	if assertion == "" {
		return "", oauthErr("invalid_client", "client_assertion is required")
	}
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		return "", oauthErr("invalid_client", "malformed client_assertion")
	}
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", oauthErr("invalid_client", "malformed client_assertion")
	}
	var hdr map[string]any
	if json.Unmarshal(hdrRaw, &hdr) != nil {
		return "", oauthErr("invalid_client", "malformed client_assertion")
	}
	if alg, _ := hdr["alg"].(string); alg != "ES256" {
		return "", oauthErr("invalid_client", "client_assertion alg must be ES256")
	}
	kid, _ := hdr["kid"].(string)

	keys := doc.jwksResolved
	if len(keys) == 0 && doc.JWKS != nil {
		keys = doc.JWKS.Keys // cache-hit docs carry keys only via JWKS
	}
	var pub *ecdsa.PublicKey
	var keyJWK map[string]any
	for _, k := range keys {
		if kk, _ := k["kty"].(string); kk != "EC" {
			continue
		}
		if crv, _ := k["crv"].(string); crv != "P-256" {
			continue
		}
		if use, _ := k["use"].(string); use != "" && use != "sig" {
			continue
		}
		if kid != "" {
			if kkid, _ := k["kid"].(string); kkid != kid {
				continue
			}
		}
		if a, _ := k["alg"].(string); a != "" && a != "ES256" {
			continue
		}
		if p, err := ecPublicKeyFromJWK(k); err == nil {
			pub, keyJWK = p, k
			break
		}
	}
	if pub == nil {
		return "", oauthErr("invalid_client", "no usable ES256 key in the client jwks")
	}
	if _, claims, err := verifyES256JWT(assertion, "", pub); err != nil {
		return "", oauthErr("invalid_client", "client_assertion signature is invalid")
	} else {
		if iss, _ := claims["iss"].(string); iss != doc.ClientID {
			return "", oauthErr("invalid_client", "client_assertion iss must equal client_id")
		}
		if sub, _ := claims["sub"].(string); sub != doc.ClientID {
			return "", oauthErr("invalid_client", "client_assertion sub must equal client_id")
		}
		audOK := false
		switch aud := claims["aud"].(type) {
		case string:
			audOK = aud == issuer
		case []any:
			for _, a := range aud {
				if s, _ := a.(string); s == issuer {
					audOK = true
				}
			}
		}
		if !audOK {
			return "", oauthErr("invalid_client", "client_assertion aud must be the issuer")
		}
		expf, _ := claims["exp"].(float64)
		if expf == 0 || time.Now().After(time.Unix(int64(expf), 0)) ||
			time.Unix(int64(expf), 0).Sub(time.Now()) > 10*time.Minute {
			return "", oauthErr("invalid_client", "client_assertion exp is invalid")
		}
		iatf, _ := claims["iat"].(float64)
		// Fresh-window bound: the profile tolerates ≤1min future iat.
		if iatf == 0 || time.Since(time.Unix(int64(iatf), 0)) > 10*time.Minute ||
			time.Until(time.Unix(int64(iatf), 0)) > time.Minute {
			return "", oauthErr("invalid_client", "client_assertion iat is outside the acceptance window")
		}
		jti, _ := claims["jti"].(string)
		if jti == "" || len(jti) > 128 {
			return "", oauthErr("invalid_client", "client_assertion jti is required")
		}
		if s.vk != nil {
			ok, err := s.vk.SetNX(ctx, "jc:atp:oauth:cjti:"+jti, "1", 10*time.Minute).Result()
			if err == nil && !ok {
				return "", oauthErr("invalid_client", "client_assertion already used")
			}
		}
	}
	jkt, err := jwkThumbprint(keyJWK)
	if err != nil {
		return "", oauthErr("invalid_client", "client jwk is not thumbnailable")
	}
	return kid + "|ES256|" + jkt, nil
}

// ---------------------------------------------------------------------------
// Grants — issue, rotate, revoke, member management
// ---------------------------------------------------------------------------

type oauthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
	Sub          string `json:"sub"`
}

// mintGrant issues a fresh access+refresh pair on a new grant row.
func (s *Service) mintGrant(ctx context.Context, accountID pgtype.UUID, clientID, clientJKT, scope, dpopJKT string) (*oauthTokenResponse, error) {
	access := oauthRandom(oauthAccessTokenPrefix)
	refresh := oauthRandom(oauthRefreshTokenPrefix)
	refreshTTL := oauthRefreshPublicTTL
	if clientJKT != "" {
		refreshTTL = oauthRefreshConfidTTL
	}
	now := time.Now()
	row, err := s.store.q.CreateOAuthGrant(ctx, gen.CreateOAuthGrantParams{
		AccountID:        accountID,
		ClientID:         clientID,
		ClientJkt:        strPtr(clientJKT),
		Scope:            scope,
		DpopJkt:          dpopJKT,
		AccessHash:       oauthHash(access),
		RefreshHash:      oauthHash(refresh),
		AccessExpiresAt:  pgtype.Timestamptz{Time: now.Add(oauthAccessTTL), Valid: true},
		RefreshExpiresAt: pgtype.Timestamptz{Time: now.Add(refreshTTL), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("atproto: create oauth grant: %w", err)
	}
	_ = row
	acct, err := s.sessionAccountByID(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return &oauthTokenResponse{
		AccessToken:  access,
		TokenType:    "DPoP",
		ExpiresIn:    int64(oauthAccessTTL / time.Second),
		RefreshToken: refresh,
		Scope:        scope,
		Sub:          acct.DID,
	}, nil
}

// rotateGrant consumes a refresh token → a new pair on the same grant.
// Single-use: a stale-hash replay on a live grant revokes the family
// (theft signal); anything else is a plain invalid_grant.
func (s *Service) rotateGrant(ctx context.Context, refreshTok, clientID, clientJKT, dpopJKT string) (*oauthTokenResponse, error) {
	hash := oauthHash(refreshTok)
	grant, err := s.store.q.GetOAuthGrantByAnyHash(ctx, hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, oauthErr("invalid_grant", "the refresh token is unknown")
		}
		return nil, fmt.Errorf("atproto: grant lookup: %w", err)
	}
	if grant.ClientID != clientID {
		return nil, oauthErr("invalid_grant", "client_id does not match the grant")
	}
	if grant.RevokedAt.Valid {
		return nil, oauthErr("invalid_grant", "the grant is revoked")
	}
	if time.Now().After(grant.RefreshExpiresAt.Time) {
		return nil, oauthErr("invalid_grant", "the refresh token is expired")
	}
	if grant.RefreshHash != hash {
		// Stale-hash replay on a live grant — the old refresh token
		// escaped. Revoke the family the way atproto_sessions does; a
		// presented *access* token is just the wrong grant, no signal.
		if grant.PrevRefreshHash != nil && *grant.PrevRefreshHash == hash {
			_, _ = s.store.q.RevokeOAuthGrant(ctx, grant.ID)
		}
		return nil, oauthErr("invalid_grant", "the refresh token is no longer current")
	}
	if grant.DpopJkt != dpopJKT {
		return nil, oauthErr("invalid_grant", "DPoP key does not match the grant")
	}
	if (grant.ClientJkt == nil && clientJKT != "") ||
		(grant.ClientJkt != nil && *grant.ClientJkt != clientJKT) {
		return nil, oauthErr("invalid_client", "client key binding does not match the grant")
	}
	access := oauthRandom(oauthAccessTokenPrefix)
	refresh := oauthRandom(oauthRefreshTokenPrefix)
	row, err := s.store.q.RotateOAuthGrant(ctx, gen.RotateOAuthGrantParams{
		RefreshHash:     hash,
		NewAccessHash:   oauthHash(access),
		NewRefreshHash:  oauthHash(refresh),
		AccessExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(oauthAccessTTL), Valid: true},
	})
	if err != nil {
		return nil, oauthErr("invalid_grant", "the refresh token is no longer current")
	}
	_ = row
	acct, err := s.sessionAccountByID(ctx, grant.AccountID)
	if err != nil {
		return nil, err
	}
	return &oauthTokenResponse{
		AccessToken:  access,
		TokenType:    "DPoP",
		ExpiresIn:    int64(oauthAccessTTL / time.Second),
		RefreshToken: refresh,
		Scope:        grant.Scope,
		Sub:          acct.DID,
	}, nil
}

// sessionAccountByID is the grant→SessionAccount resolution the RS path
// uses — identical liveness rules to sessionAccountByDID.
func (s *Service) sessionAccountByID(ctx context.Context, id pgtype.UUID) (*SessionAccount, error) {
	acct, err := s.store.q.GetAtprotoSessionAccountByID(ctx, id)
	if err != nil {
		return nil, ErrAuthRequired
	}
	if acct.Status == AccountStatusTombstone || acct.UserStatus != "active" {
		return nil, ErrAuthRequired
	}
	return &SessionAccount{
		AccountID:      acct.ID,
		UserID:         acct.UserID,
		DID:            acct.Did,
		Handle:         acct.Handle,
		Status:         acct.Status,
		DisabledReason: acct.DisabledReason,
		Email:          acct.UserEmail,
	}, nil
}

// AuthenticateDPoP resolves `Authorization: DPoP {atp_at_…}` + the DPoP
// proof to a SessionAccount. The proof jkt must equal the grant's bound
// jkt, ath must bind the presented token, and the nonce rule applies.
// scope rides along so getSession can gate email on transition:email.
func (s *Service) AuthenticateDPoP(ctx context.Context, r *http.Request, w http.ResponseWriter) (*SessionAccount, error) {
	if !s.featureOn(ctx) {
		return nil, ErrAuthRequired
	}
	raw := ""
	if ah := r.Header.Get("Authorization"); len(ah) > 5 && strings.EqualFold(ah[:5], "dpop ") {
		raw = strings.TrimSpace(ah[5:])
	}
	if !strings.HasPrefix(raw, oauthAccessTokenPrefix) {
		return nil, ErrAuthRequired
	}
	grant, err := s.store.q.GetOAuthGrantByAccessHash(ctx, oauthHash(raw))
	if err != nil || grant.RevokedAt.Valid || time.Now().After(grant.AccessExpiresAt.Time) {
		if w != nil {
			w.Header().Set("WWW-Authenticate", `DPoP algs="ES256", error="invalid_token"`)
		}
		return nil, ErrAuthRequired
	}
	proof, err := s.verifyDPoP(ctx, r)
	if err != nil {
		if w != nil {
			s.setNonceHeader(w)
			code := "invalid_token"
			var oe *oauthError
			if errors.As(err, &oe) && oe.code != "" {
				code = oe.code
			}
			w.Header().Set("WWW-Authenticate", `DPoP algs="ES256", error="`+code+`"`)
		}
		return nil, ErrAuthRequired
	}
	if err := s.checkNonce(proof); err != nil {
		if w != nil {
			s.setNonceHeader(w)
			w.Header().Set("WWW-Authenticate", `DPoP algs="ES256", error="use_dpop_nonce"`)
		}
		return nil, ErrAuthRequired
	}
	if w != nil {
		s.setNonceHeader(w)
	}
	if proof.jkt != grant.DpopJkt {
		return nil, ErrAuthRequired
	}
	tokSum := sha256.Sum256([]byte(raw))
	if proof.ath != base64.RawURLEncoding.EncodeToString(tokSum[:]) {
		return nil, ErrAuthRequired
	}
	acct, err := s.sessionAccountByID(ctx, grant.AccountID)
	if err != nil {
		return nil, err
	}
	acct.OAuthScopes = strings.Fields(grant.Scope)
	acct.OAuthClientID = grant.ClientID
	return acct, nil
}

// ListOAuthGrants is the member's live-grant view — one row per
// (client_id, scope, binding) group.
func (s *Service) ListOAuthGrants(ctx context.Context, accountID pgtype.UUID) ([]gen.ListOAuthGrantsForAccountRow, error) {
	return s.store.q.ListOAuthGrantsForAccount(ctx, accountID)
}

// RevokeOAuthGrantForClient drops the whole client×account family — the
// member's "delete this OAuth session" surface.
func (s *Service) RevokeOAuthGrantForClient(ctx context.Context, accountID pgtype.UUID, clientID string) error {
	if clientID == "" || len(clientID) > 2048 {
		return ErrInvalidID
	}
	_, err := s.store.q.RevokeOAuthGrantsForClient(ctx, gen.RevokeOAuthGrantsForClientParams{
		AccountID: accountID, ClientID: clientID,
	})
	return err
}

// RevokeOAuthForAccount is the tombstone/erasure step — every live grant
// dies with the account, alongside sessions and app passwords.
func (s *Service) RevokeOAuthForAccount(ctx context.Context, accountID pgtype.UUID) error {
	_, err := s.store.q.RevokeOAuthGrantsForAccount(ctx, accountID)
	return err
}

// RevokeOAuthToken implements RFC 7009 for atp_* tokens — always
// well-formed-200 at the handler; the store only sees hashes.
func (s *Service) RevokeOAuthToken(ctx context.Context, token string) {
	hash := oauthHash(token)
	grant, err := s.store.q.GetOAuthGrantByAnyHash(ctx, hash)
	if err != nil || grant.RevokedAt.Valid {
		return
	}
	_, _ = s.store.q.RevokeOAuthGrant(ctx, grant.ID)
}

// ---------------------------------------------------------------------------
// Consent token — HMAC binding the POST to the rendered GET
// ---------------------------------------------------------------------------

// consentTokenValue signs request_uri|user_id|expiry — the POST replays
// the request shape via the PAR row, so binding the request_uri covers
// every field (client, redirect, scope, challenge are all on the row).
func (s *Service) consentTokenValue(requestURI string, userID pgtype.UUID, expiry time.Time) string {
	payload := strings.Join([]string{"v2", requestURI, userID.String(), strconv.FormatInt(expiry.Unix(), 10)}, "|")
	mac := hmac.New(sha256.New, s.jwtKey)
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)) + "." + strconv.FormatInt(expiry.Unix(), 10)
}

func (s *Service) verifyConsentToken(token, requestURI string, userID pgtype.UUID) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := s.consentTokenValue(requestURI, userID, time.Unix(exp, 0))
	return hmac.Equal([]byte(token), []byte(expected))
}

// ---------------------------------------------------------------------------
// PAR + code — the request/authorize half
// ---------------------------------------------------------------------------

// CreateOAuthRequest validates and stores a PAR. The doc + redirect +
// scope + DPoP binding all live on the row — authorize and exchange
// never re-derive them.
func (s *Service) CreateOAuthRequest(ctx context.Context, p *gen.CreateOAuthRequestParams) (*gen.AtprotoOauthRequest, error) {
	row, err := s.store.q.CreateOAuthRequest(ctx, *p)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, oauthErr("invalid_request", "code_challenge was already used")
		}
		return nil, fmt.Errorf("atproto: create oauth request: %w", err)
	}
	return &row, nil
}

func isUniqueViolation(err error) bool {
	var pge *pgconn.PgError
	return errors.As(err, &pge) && pge.Code == "23505"
}

// ExchangeCode consumes a code (CAS) and verifies the exchange request
// against the PAR row — client, redirect, PKCE, DPoP binding all match
// or the grant is refused. A replayed code revokes the grant it minted.
func (s *Service) ExchangeCode(ctx context.Context, code, clientID, clientJKT, redirectURI, verifier, dpopJKT string) (*oauthTokenResponse, error) {
	hash := oauthHash(code)
	row, err := s.store.q.ConsumeOAuthCode(ctx, &hash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Replay path — the code mapped to an already-consumed
			// request. The grant it minted dies with the detection.
			if prev, perr := s.store.q.GetOAuthRequestByCodeHash(ctx, &hash); perr == nil && prev.AccountID.Valid {
				_, _ = s.store.q.RevokeOAuthGrantsForRequest(ctx, gen.RevokeOAuthGrantsForRequestParams{
					AccountID: prev.AccountID, ClientID: prev.ClientID,
				})
			}
			return nil, oauthErr("invalid_grant", "the authorization code is invalid, expired, or already used")
		}
		return nil, fmt.Errorf("atproto: consume code: %w", err)
	}
	if row.ClientID != clientID {
		return nil, oauthErr("invalid_grant", "client_id does not match the authorization request")
	}
	if row.RedirectUri != redirectURI {
		return nil, oauthErr("invalid_grant", "redirect_uri does not match the authorization request")
	}
	if (row.ClientJkt == nil) != (clientJKT == "") ||
		(row.ClientJkt != nil && *row.ClientJkt != clientJKT) {
		return nil, oauthErr("invalid_client", "client key binding does not match the authorization request")
	}
	sum := sha256.Sum256([]byte(verifier))
	if row.CodeChallenge != base64.RawURLEncoding.EncodeToString(sum[:]) {
		return nil, oauthErr("invalid_grant", "code_verifier does not satisfy the challenge")
	}
	if row.DpopJkt != dpopJKT {
		return nil, oauthErr("invalid_grant", "DPoP key does not match the authorization request")
	}
	return s.mintGrant(ctx, row.AccountID, clientID, clientJKT, row.Scope, dpopJKT)
}
