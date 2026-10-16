package publicapi

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Extension tokens are ordinary api_keys rows whose name carries the
// "extension:" prefix. They are minted only through
// POST /me/extension-tokens so the key gets the narrow read scope set the
// browser extension needs — no agent.use, no premium search/brief scopes.
// Revocation reuses DELETE /me/api-keys/{id}.
//
// Ref: japaconnect-engineering/sections/47-public-api-developer-portal.md
// (Browser-Extension Digest).

const extensionTokenPrefix = "extension:"

// extensionTokenScopes is the fixed scope set an extension token carries —
// read-only, no agent.use (MCP), no api.read.search/api.read.brief
// (premium surfaces stay web/paid-API in v1).
var extensionTokenScopes = []string{
	"api.read.digest",
	"api.read.content",
	"api.read.me",
	"api.read.safety",
}

// ErrInvalidInstallID is returned when install_id is present but not a UUID.
var ErrInvalidInstallID = errors.New("install_id must be a UUID")

// extensionTokenName builds the api_keys.name value. installID is optional —
// the web settings page cannot know the extension's generated ID, so an
// absent ID yields a generated suffix.
func extensionTokenName(installID string) (string, error) {
	if installID == "" {
		return extensionTokenPrefix + uuid.NewString(), nil
	}

	if _, err := uuid.Parse(installID); err != nil {
		return "", ErrInvalidInstallID
	}

	return extensionTokenPrefix + installID, nil
}

// isExtensionToken reports whether an api_keys row is an extension token.
func isExtensionToken(name string) bool {
	return strings.HasPrefix(name, extensionTokenPrefix)
}

// CreateExtensionToken mints a narrow-scoped API key for the browser
// extension. The full secret is returned once, exactly like CreateAPIKey.
// installID is optional; when present it must be a UUID.
func (s *Service) CreateExtensionToken(ctx context.Context, userID pgtype.UUID, installID string) (CreateAPIKeyResponse, error) {
	name, err := extensionTokenName(installID)
	if err != nil {
		return CreateAPIKeyResponse{}, err
	}

	return s.CreateAPIKey(ctx, userID, CreateAPIKeyInput{
		Name:          name,
		Scopes:        extensionTokenScopes,
		RateLimitTier: "standard",
	})
}

// ListExtensionTokens returns the caller's active extension-scoped API
// keys. Revoked keys are soft-deleted but still surface from ListAPIKeys
// (the dev portal shows them), so the connect list filters them out.
func (s *Service) ListExtensionTokens(ctx context.Context, userID pgtype.UUID) ([]APIKey, error) {
	keys, err := s.ListAPIKeys(ctx, userID)
	if err != nil {
		return nil, err
	}

	out := make([]APIKey, 0, len(keys))

	for _, k := range keys {
		if isExtensionToken(k.Name) && k.Status == "active" {
			out = append(out, k)
		}
	}

	return out, nil
}
