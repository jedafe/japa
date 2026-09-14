// Package desktopauth provides authentication support for the JapaConnect
// desktop studio client.
// // Ref: japaconnect-engineering/sections/105-japa-content-studio-desktop.md
package desktopauth

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/publicapi"
)

// APIKeyValidator describes the publicapi.Service methods needed by the
// exchange handler. Using a narrow interface keeps the package testable.
type APIKeyValidator interface {
	ValidateAPIKey(ctx context.Context, key string) (*publicapi.APIKeyContext, error)
	HasIPAccess(ctx context.Context, keyID pgtype.UUID, clientIP string) (bool, error)
}

// RefreshTokenCreator abstracts the only sqlc query used by the exchange
// handler, allowing tests to inject an in-memory fake.
type RefreshTokenCreator interface {
	CreateRefreshToken(ctx context.Context, arg gen.CreateRefreshTokenParams) error
}

// Handlers provides the desktop auth HTTP handlers.
type Handlers struct {
	KeyRotation *auth.KeyRotation
	Queries     RefreshTokenCreator
	APIKeys     APIKeyValidator
}

// ExchangeKeyRequest is the expected body for POST /auth/exchange-key.
// The API key is also accepted via the X-API-Key header for consistency with
// the public API.
type ExchangeKeyRequest struct {
	APIKey string `json:"api_key"`
}

// ExchangeKeyResponse is returned on a successful key exchange.
type ExchangeKeyResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

// ExchangeKey swaps a valid API key for a JWT access/refresh token pair.
// This allows the desktop studio client to authenticate against admin routes
// that require a Bearer JWT.
// // Ref: japaconnect-engineering/sections/105-japa-content-studio-desktop.md.
func (h *Handlers) ExchangeKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Accept the key from the X-API-Key header, falling back to a JSON body.
	// The header is the canonical public-API transport.
	rawKey := r.Header.Get("X-API-Key")
	if rawKey == "" {
		var req ExchangeKeyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.APIKey != "" {
			rawKey = req.APIKey
		}
	}

	if rawKey == "" {
		errpkg.WriteError(w, http.StatusUnauthorized, "missing_api_key", "X-API-Key header or api_key body is required")
		return
	}

	keyCtx, err := h.APIKeys.ValidateAPIKey(ctx, rawKey)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "API key is invalid or expired")
		return
	}

	// IP allowlist enforcement: if the key has IP restrictions, verify the
	// client IP is within the allowed CIDR ranges.
	clientIP := clientip.FromRequest(r)
	if clientIP != "" {
		allowed, err := h.APIKeys.HasIPAccess(ctx, keyCtx.KeyID, clientIP)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "ip_check_error", "Failed to verify IP access")
			return
		}

		if !allowed {
			errpkg.WriteError(w, http.StatusForbidden, "ip_not_allowed", "Your IP address is not authorized for this API key")
			return
		}
	}

	// Exchange requires a write-capable API key. Read-only public keys cannot
	// be turned into admin session tokens.
	if !publicapi.HasScope(keyCtx.Scopes, "api.write.content") {
		errpkg.WriteError(w, http.StatusForbidden, "insufficient_scope", "API key must have the api.write.content scope")
		return
	}

	// Mint a short-lived access token for the API key's user.
	accessToken, err := h.KeyRotation.MintAccessTokenWithContext(ctx, keyCtx.UserID, (pgtype.UUID{}))
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to mint access token")
		return
	}

	// Generate a refresh token with the same lifetime as a normal session.
	refreshToken, err := auth.GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to generate refresh token")
		return
	}

	refreshHash := auth.HashRefreshToken(refreshToken)

	refreshTTL := auth.GetRefreshTokenTTL(ctx, h.KeyRotation.Settings)
	expiresAt := time.Now().Add(refreshTTL)

	deviceName := r.Header.Get("X-Device-Id")
	if deviceName == "" {
		deviceName = "japa-studio-desktop"
	}

	if err := h.Queries.CreateRefreshToken(ctx, gen.CreateRefreshTokenParams{

		UserID:    keyCtx.UserID,
		TokenHash: refreshHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		DeviceName: func() *string {
			s := deviceName
			return &s
		}(),
		IpAddress: func() *string {
			s := clientIP
			return &s
		}(),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to persist refresh token")
		return
	}

	accessTTL := auth.GetAccessTokenTTL(ctx, h.KeyRotation.Settings)

	resp := ExchangeKeyResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "bearer",
		ExpiresIn:    int64(accessTTL.Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
