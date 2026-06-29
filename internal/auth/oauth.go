// Package auth — OAuth 2.0 provider integration for social login.
//
// Supports Google (via OpenID Connect) and GitHub (via OAuth 2.0).
// Provider configuration is read from environment variables at startup.
// Tokens are NEVER stored — only the provider's subject identifier and
// the linkage to our user_id are persisted in the oauth_accounts table.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

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
	GetOAuthAccountByUser(ctx context.Context, tenantID pgtype.UUID, userID pgtype.UUID, provider string) (*OAuthAccount, error)
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
	return providers
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching google userinfo: %w", err)
	}
	defer resp.Body.Close()

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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching github user: %w", err)
	}
	defer resp.Body.Close()

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
		ProviderUserID: fmt.Sprintf("%d", user.ID),
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing token request: %w", err)
	}
	defer resp.Body.Close()

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
