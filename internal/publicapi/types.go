// Package publicapi implements the Public API & Developer Portal (Section 47).
//
// This package provides API key management for external integrations.
// API keys are stored as SHA-256 hashes — the full key is returned only
// once at creation time. Keys are validated via the X-API-Key header
// and injected into the request context for downstream authorization.
package publicapi

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// APIKey represents an API key returned to the caller (without the hash).
type APIKey struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	KeyPrefix     string              `json:"key_prefix"`
	Scopes        []string            `json:"scopes"`
	RateLimitTier string              `json:"rate_limit_tier"`
	Status        string              `json:"status"`
	AllowedIPs    []string            `json:"allowed_ips,omitempty"`
	LastUsedAt    *pgtype.Timestamptz `json:"last_used_at,omitempty"`
	RequestCount  int64               `json:"request_count"`
	ExpiresAt     *pgtype.Timestamptz `json:"expires_at,omitempty"`
	CreatedAt     pgtype.Timestamptz  `json:"created_at"`
	UpdatedAt     pgtype.Timestamptz  `json:"updated_at"`
}

// APIKeyIPAllowlist represents a single IP allowlist entry for an API key.
type APIKeyIPAllowlist struct {
	ID          string `json:"id"`
	APIKeyID    string `json:"api_key_id"`
	CIDR        string `json:"cidr"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// CreateAPIKeyInput holds the fields needed to create a new API key.
type CreateAPIKeyInput struct {
	Name          string              `json:"name"`
	Scopes        []string            `json:"scopes"`
	RateLimitTier string              `json:"rate_limit_tier"`
	ExpiresAt     *pgtype.Timestamptz `json:"expires_at,omitempty"`
}

// CreateAPIKeyResponse is returned once at creation time with the full key.
type CreateAPIKeyResponse struct {
	APIKey
	Key string `json:"key"` // Full key — shown only once
}

// APIKeyContext holds the validated API key info injected into request context.
type APIKeyContext struct {
	KeyID         pgtype.UUID
	UserID        pgtype.UUID
	TenantID      pgtype.UUID
	Scopes        []string
	KeyPrefix     string
	RateLimitTier string
}

type contextKey struct{ name string }

var apiKeyContextKey = contextKey{"api_key"}

// APIKeyFromContext extracts the APIKeyContext if present.
func APIKeyFromContext(ctx context.Context) *APIKeyContext {
	ak, _ := ctx.Value(apiKeyContextKey).(*APIKeyContext)
	return ak
}
