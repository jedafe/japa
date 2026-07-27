// Package auth — OAuth 2.0 provider integration for social login.
//
// Supports Google (via OpenID Connect) and GitHub (via OAuth 2.0).
// Provider configuration is read from environment variables at startup.
// Tokens are NEVER stored — only the provider's subject identifier and
// the linkage to our user_id are persisted in the oauth_accounts table.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// OAuthProvider defines the operations each OAuth provider must implement.
type OAuthProvider interface {
	// Name returns the provider identifier (e.g. "google", "github").
	Name() string
	// AuthURL returns the authorization URL the user should be redirected to.
	AuthURL(state string) string
	// Exchange exchanges an authorization code for user info.
	Exchange(ctx context.Context, code string) (*OAuthUserInfo, error)
}

// OAuthUserInfo holds the minimal user information returned by a provider.
type OAuthUserInfo struct {
	Provider       string
	ProviderUserID string
	Email          string
	Name           string
}

// OAuthStore defines the database operations for OAuth account management.
type OAuthStore interface {
	GetOAuthAccount(ctx context.Context, tenantID pgtype.UUID, provider, providerUserID string) (*OAuthAccount, error)
	GetOAuthAccountByUser(ctx context.Context, tenantID, userID pgtype.UUID, provider string) (*OAuthAccount, error)
	LinkOAuthAccount(ctx context.Context, tenantID, userID pgtype.UUID, provider, providerUserID string) error
	GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error)
}

// OAuthAccount represents a linked OAuth provider account.
type OAuthAccount struct {
	ID             pgtype.UUID
	UserID         pgtype.UUID
	Provider       string
	ProviderUserID string
}

// OAuthConfig holds the configuration for all supported OAuth providers.
type OAuthConfig struct {
	GoogleClientID     string
	GoogleClientSecret string
	GitHubClientID     string
	GitHubClientSecret string
	AppleClientID      string
	AppleTeamID        string
	AppleKeyID         string
	ApplePrivateKeyPEM string // PEM-encoded private key content (or path resolved by caller)
	FacebookAppID      string
	FacebookAppSecret  string
	RedirectBase       string
}

// SupportedProviders returns the list of configured provider names.
func (c OAuthConfig) SupportedProviders() []string {
	var providers []string
	if c.GoogleClientID != "" {
		providers = append(providers, "google")
	}

	if c.GitHubClientID != "" {
		providers = append(providers, "github")
	}

	if c.AppleClientID != "" {
		providers = append(providers, "apple")
	}

	if c.FacebookAppID != "" {
		providers = append(providers, "facebook")
	}

	return providers
}

// oauthHTTPClient is a hardened HTTP client shared by all OAuth provider
// calls (SEC-006). It enforces a 30s timeout and caps redirects at 10 to
// prevent SSRF via redirect chains and resource exhaustion via hanging
// connections. http.DefaultClient has no timeout and unlimited redirects.
var oauthHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	},
}

// NewProvider creates an OAuthProvider for the given provider name.
// Returns nil if the provider is not configured or not supported.
func (c OAuthConfig) NewProvider(provider string) OAuthProvider {
	switch strings.ToLower(provider) {
	case "google":
		if c.GoogleClientID == "" || c.GoogleClientSecret == "" {
			return nil
		}

		return &googleProvider{
			clientID:     c.GoogleClientID,
			clientSecret: c.GoogleClientSecret,
			redirectURL:  c.RedirectBase + "/api/v1/auth/oauth/google/callback",
		}
	case "github":
		if c.GitHubClientID == "" || c.GitHubClientSecret == "" {
			return nil
		}

		return &githubProvider{
			clientID:     c.GitHubClientID,
			clientSecret: c.GitHubClientSecret,
			redirectURL:  c.RedirectBase + "/api/v1/auth/oauth/github/callback",
		}
	case "apple":
		if c.AppleClientID == "" || c.AppleTeamID == "" || c.AppleKeyID == "" {
			return nil
		}

		return &appleProvider{
			clientID:      c.AppleClientID,
			teamID:        c.AppleTeamID,
			keyID:         c.AppleKeyID,
			privateKeyPEM: c.ApplePrivateKeyPEM,
			redirectURL:   c.RedirectBase + "/api/v1/auth/oauth/apple/callback",
		}
	case "facebook":
		if c.FacebookAppID == "" || c.FacebookAppSecret == "" {
			return nil
		}

		return &facebookProvider{
			appID:       c.FacebookAppID,
			appSecret:   c.FacebookAppSecret,
			redirectURL: c.RedirectBase + "/api/v1/auth/oauth/facebook/callback",
		}
	default:
		return nil
	}
}

// --- Google Provider (OpenID Connect) ---

type googleProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
}

func (g *googleProvider) Name() string { return "google" }

func (g *googleProvider) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {g.clientID},
		"redirect_uri":  {g.redirectURL},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
		"state":         {state},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
	}

	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
}

func (g *googleProvider) Exchange(ctx context.Context, code string) (*OAuthUserInfo, error) {
	// Exchange code for tokens
	tokenResp, err := exchangeCode(ctx, "https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"redirect_uri":  {g.redirectURL},
		"grant_type":    {"authorization_code"},
	})
	if err != nil {
		return nil, fmt.Errorf("google token exchange: %w", err)
	}

	// Fetch user info from OpenID Connect userinfo endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v3/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("creating userinfo request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching google userinfo: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google userinfo returned %d: %s", resp.StatusCode, body)
	}

	var info struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding google userinfo: %w", err)
	}

	return &OAuthUserInfo{
		Provider:       "google",
		ProviderUserID: info.Sub,
		Email:          info.Email,
		Name:           info.Name,
	}, nil
}

// --- GitHub Provider ---

type githubProvider struct {
	clientID     string
	clientSecret string
	redirectURL  string
}

func (g *githubProvider) Name() string { return "github" }

func (g *githubProvider) AuthURL(state string) string {
	params := url.Values{
		"client_id":    {g.clientID},
		"redirect_uri": {g.redirectURL},
		"scope":        {"read:user user:email"},
		"state":        {state},
	}

	return "https://github.com/login/oauth/authorize?" + params.Encode()
}

func (g *githubProvider) Exchange(ctx context.Context, code string) (*OAuthUserInfo, error) {
	// Exchange code for access token
	tokenResp, err := exchangeCode(ctx, "https://github.com/login/oauth/access_token", url.Values{
		"code":          {code},
		"client_id":     {g.clientID},
		"client_secret": {g.clientSecret},
		"redirect_uri":  {g.redirectURL},
	})
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}

	// Fetch user info
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	if err != nil {
		return nil, fmt.Errorf("creating github user request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching github user: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("github user returned %d: %s", resp.StatusCode, body)
	}

	var user struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding github user: %w", err)
	}

	// GitHub may not return email in the /user response; fetch from emails endpoint
	if user.Email == "" {
		user.Email = fetchGitHubEmail(ctx, tokenResp.AccessToken)
	}

	return &OAuthUserInfo{
		Provider:       "github",
		ProviderUserID: strconv.Itoa(user.ID),
		Email:          user.Email,
		Name:           user.Name,
	}, nil
}

// fetchGitHubEmail fetches the primary email from the GitHub emails API.
func fetchGitHubEmail(ctx context.Context, accessToken string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user/emails", nil)
	if err != nil {
		return ""
	}

	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return ""
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var emails []struct {
		Email   string `json:"email"`
		Primary bool   `json:"primary"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return ""
	}

	for _, e := range emails {
		if e.Primary {
			return e.Email
		}
	}

	if len(emails) > 0 {
		return emails[0].Email
	}

	return ""
}

// --- Apple Provider (Sign in with Apple) ---

// appleJWKS caches Apple's public keys used for ID token signature verification.
// Keys auto-refresh every hour with a mutex-guarded on-demand fetch.
var (
	appleJWKS     map[string]interface{}
	appleJWKSMu   sync.RWMutex
	appleJWKSFetchedAt time.Time
)

// appleJWKSKeys represents the JSON structure of Apple's JWKS endpoint.
type appleJWKSKeys struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

type appleProvider struct {
	clientID      string
	teamID        string
	keyID         string
	privateKeyPEM string
	redirectURL   string
	httpClient    *http.Client
}

func (a *appleProvider) Name() string { return "apple" }

func (a *appleProvider) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {a.clientID},
		"redirect_uri":  {a.redirectURL},
		"response_type": {"code id_token"},
		"scope":         {"name email"},
		"state":         {state},
		"response_mode": {"form_post"},
	}

	return "https://appleid.apple.com/auth/authorize?" + params.Encode()
}

func (a *appleProvider) Exchange(ctx context.Context, code string) (*OAuthUserInfo, error) {
	clientSecret, err := a.generateClientSecret()
	if err != nil {
		return nil, fmt.Errorf("apple client secret generation: %w", err)
	}

	// Exchange code for tokens
	tokenURL := "https://appleid.apple.com/auth/token" //nolint:gosec // G101: Apple OAuth endpoint URL, not a credential
	params := url.Values{
		"client_id":     {a.clientID},
		"client_secret": {clientSecret},
		"code":          {code},
		"redirect_uri":  {a.redirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating apple token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing apple token request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading apple token response: %w", err)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		TokenType   string `json:"token_type"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding apple token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("apple token exchange error: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Apple returns user info in the ID token JWT — decode it to extract email and sub
	claims, err := a.parseIDToken(tokenResp.IDToken)
	if err != nil {
		return nil, fmt.Errorf("parsing apple id token: %w", err)
	}

	return &OAuthUserInfo{
		Provider:       "apple",
		ProviderUserID: claims.Subject,
		Email:          claims.Email,
		Name:           "", // Apple only provides name on first authorization; stored client-side
	}, nil
}

// appleIDTokenClaims holds the claims we care about from Apple's ID token.
type appleIDTokenClaims struct {
	Subject string `json:"sub"`
	Email   string `json:"email"`
	jwt.RegisteredClaims
}

// parseIDToken parses and verifies Apple's ID token against Apple's JWKS endpoint.
// Keys are fetched from https://appleid.apple.com/auth/keys, cached for up to 1 hour,
// and refreshed on demand when the token's kid is not found in the cache.
func (a *appleProvider) parseIDToken(idToken string) (*appleIDTokenClaims, error) {
	httpClient := a.httpClient
	if httpClient == nil {
		httpClient = oauthHTTPClient // SEC-006: hardened client, not DefaultClient
	}

	// Determine the token's key ID to decide if we need a fresh fetch.
	kid := extractUnverifiedKid(idToken)

	// Fetch JWKS if cache is empty, stale, or missing the required key.
	if shouldRefreshAppleJWKS(kid) {
		keys, err := fetchAppleJWKS(httpClient)
		if err != nil {
			return nil, fmt.Errorf("fetching Apple JWKS: %w", err)
		}
		appleJWKSMu.Lock()
		appleJWKS = keys
		appleJWKSFetchedAt = time.Now()
		appleJWKSMu.Unlock()
	}

	appleJWKSMu.RLock()
	keys := appleJWKS
	appleJWKSMu.RUnlock()

	if keys == nil {
		return nil, fmt.Errorf("apple JWKS not available — cannot verify ID token")
	}

	var claims appleIDTokenClaims
	token, err := jwt.ParseWithClaims(idToken, &claims, func(token *jwt.Token) (interface{}, error) {
		kidVal, ok := token.Header["kid"]
		if !ok {
			return nil, fmt.Errorf("token missing kid header")
		}
		kidStr, ok := kidVal.(string)
		if !ok {
			return nil, fmt.Errorf("token kid is not a string")
		}
		key, ok := keys[kidStr]
		if !ok {
			return nil, fmt.Errorf("key %q not found in JWKS", kidStr)
		}
		return key, nil
	}, jwt.WithLeeway(30*time.Second), jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		return nil, fmt.Errorf("verifying Apple ID token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("apple ID token invalid")
	}

	if claims.Issuer != "https://appleid.apple.com" {
		return nil, fmt.Errorf("unexpected issuer: %s", claims.Issuer)
	}
	if claims.Audience == nil || !containsAudience(claims.Audience, a.clientID) {
		return nil, fmt.Errorf("token audience does not include client_id")
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("token missing subject")
	}

	return &claims, nil
}

// extractUnverifiedKid extracts the kid header from a JWT without verification.
func extractUnverifiedKid(rawToken string) string {
	parts := strings.Split(rawToken, ".")
	if len(parts) < 2 {
		return ""
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return ""
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if json.Unmarshal(headerJSON, &header) != nil {
		return ""
	}
	return header.Kid
}

// shouldRefreshAppleJWKS returns true if the JWKS cache should be refreshed.
func shouldRefreshAppleJWKS(kid string) bool {
	appleJWKSMu.RLock()
	defer appleJWKSMu.RUnlock()
	if appleJWKS == nil {
		return true
	}
	if time.Since(appleJWKSFetchedAt) > time.Hour {
		return true
	}
	if kid != "" {
		_, ok := appleJWKS[kid]
		return !ok
	}
	return false
}

// fetchAppleJWKS fetches and parses Apple's JWKS endpoint into a key map.
func fetchAppleJWKS(httpClient *http.Client) (map[string]interface{}, error) {
	const appleJWKSEndpoint = "https://appleid.apple.com/auth/keys" //nolint:gosec

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, appleJWKSEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("creating JWKS request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching Apple JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading JWKS response: %w", err)
	}

	var jwksResp appleJWKSKeys
	if err := json.Unmarshal(body, &jwksResp); err != nil {
		return nil, fmt.Errorf("parsing JWKS response: %w", err)
	}

	keys := make(map[string]interface{}, len(jwksResp.Keys))
	for _, k := range jwksResp.Keys {
		if k.Kid == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		eInt := 0
		for _, b := range eBytes {
			eInt = eInt<<8 + int(b)
		}
		pubKey := &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: eInt,
		}
		keys[k.Kid] = pubKey
	}

	return keys, nil
}

// containsAudience checks if aud (which may be a string or []string) includes
// the given clientID.
func containsAudience(aud jwt.ClaimStrings, clientID string) bool {
	for _, a := range aud {
		if a == clientID {
			return true
		}
	}
	return false
}

// generateClientSecret creates a JWT signed with the Apple private key.
// Apple requires a client_secret that is a JWT with specific claims.
func (a *appleProvider) generateClientSecret() (string, error) {
	key, err := parseECPrivateKey(a.privateKeyPEM)
	if err != nil {
		return "", fmt.Errorf("parsing Apple private key: %w", err)
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss": a.teamID,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour * 24 * 180).Unix(), // 6 months max
		"aud": "https://appleid.apple.com",
		"sub": a.clientID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = a.keyID

	signed, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("signing Apple client secret JWT: %w", err)
	}

	return signed, nil
}

// parseECPrivateKey parses a PEM-encoded EC private key for Apple Sign-In.
func parseECPrivateKey(pemContent string) (*ecdsa.PrivateKey, error) {
	block, err := jwt.ParseECPrivateKeyFromPEM([]byte(pemContent))
	if err != nil {
		return nil, fmt.Errorf("failed to parse EC private key: %w", err)
	}

	return block, nil
}

// --- Facebook Provider ---

type facebookProvider struct {
	appID       string
	appSecret   string
	redirectURL string
}

func (f *facebookProvider) Name() string { return "facebook" }

func (f *facebookProvider) AuthURL(state string) string {
	params := url.Values{
		"client_id":     {f.appID},
		"redirect_uri":  {f.redirectURL},
		"scope":         {"email,public_profile"},
		"state":         {state},
		"response_type": {"code"},
	}

	return "https://www.facebook.com/v18.0/dialog/oauth?" + params.Encode()
}

func (f *facebookProvider) Exchange(ctx context.Context, code string) (*OAuthUserInfo, error) {
	// Exchange code for access token
	tokenURL := "https://graph.facebook.com/v18.0/oauth/access_token" //nolint:gosec // G101: Facebook OAuth endpoint URL, not a credential
	params := url.Values{
		"client_id":     {f.appID},
		"client_secret": {f.appSecret},
		"code":          {code},
		"redirect_uri":  {f.redirectURL},
		"grant_type":    {"authorization_code"},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating facebook token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing facebook token request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading facebook token response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding facebook token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("facebook token exchange error: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	// Fetch user info from Graph API
	userURL := "https://graph.facebook.com/v18.0/me?fields=id,email,name&access_token=" + url.QueryEscape(tokenResp.AccessToken)

	userReq, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating facebook user request: %w", err)
	}

	userResp, err := oauthHTTPClient.Do(userReq)
	if err != nil {
		return nil, fmt.Errorf("fetching facebook user: %w", err)
	}

	defer func() { _ = userResp.Body.Close() }()

	if userResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(userResp.Body)
		return nil, fmt.Errorf("facebook user returned %d: %s", userResp.StatusCode, respBody)
	}

	var user struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	if err := json.NewDecoder(userResp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("decoding facebook user: %w", err)
	}

	return &OAuthUserInfo{
		Provider:       "facebook",
		ProviderUserID: user.ID,
		Email:          user.Email,
		Name:           user.Name,
	}, nil
}

// --- Shared helpers ---

// tokenResponse is the common structure for OAuth token exchange responses.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// exchangeCode performs a token exchange POST and returns the parsed response.
func exchangeCode(ctx context.Context, tokenURL string, params url.Values) (*tokenResponse, error) {
	params.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("creating token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing token request: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	var tokenResp tokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token exchange error: %s — %s", tokenResp.Error, tokenResp.ErrorDesc)
	}

	return &tokenResp, nil
}

// GenerateOAuthState creates a cryptographically random state parameter
// for CSRF protection during the OAuth authorization flow.
func GenerateOAuthState() (string, error) {
	return generateRefreshToken() // reuses the 32-byte random generator
}

// ValidateOAuthState checks that the callback state matches the expected value.
// The state should be stored server-side (e.g. in a session cookie or Valkey)
// during the initiation phase and validated here.
func ValidateOAuthState(expected, received string) bool {
	if expected == "" || received == "" {
		return false
	}

	return expected == received
}
