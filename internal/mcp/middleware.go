package mcp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/publicapi"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

const agentUsePermission = "agent.use"

// AuthMiddleware returns middleware that accepts either a Bearer JWT or an
// X-API-Key header for the MCP endpoint, then enforces the agent.use
// permission/scope before handing the request to the MCP handler.
func AuthMiddleware(kr *auth.KeyRotation, vkClient valkeycompat.Cmdable, publicAPIService *publicapi.Service, resolver *rbac.RBAC) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, apiKey := authOrAPIKey(r, kr, vkClient, publicAPIService)
			if user == nil || !user.UserID.Valid {
				http.Error(w, `{"error":{"code":"unauthenticated","message":"Authentication required"}}`, http.StatusUnauthorized)
				return
			}

			ctx := r.Context()
			if apiKey != nil {
				if !publicapi.HasScope(apiKey.Scopes, agentUsePermission) {
					http.Error(w, `{"error":{"code":"forbidden","message":"API key lacks required scope: agent.use"}}`, http.StatusForbidden)
					return
				}
				// Expose the key to the public API rate limiter and mark the
				// caller so tools/list + tools/call restrict to read-only.
				ctx = publicapi.WithAPIKeyContext(ctx, apiKey)
				ctx = context.WithValue(ctx, apiKeyCallerKey, true)
			} else {
				if !resolver.HasPermission(r.Context(), user.UserID, agentUsePermission) {
					http.Error(w, `{"error":{"code":"forbidden","message":"You do not have permission to use the agent"}}`, http.StatusForbidden)
					return
				}
			}

			ctx = context.WithValue(ctx, auth.UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func authOrAPIKey(r *http.Request, kr *auth.KeyRotation, vkClient valkeycompat.Cmdable, publicAPIService *publicapi.Service) (*auth.AuthenticatedUser, *publicapi.APIKeyContext) {
	// Prefer JWT Bearer when both are present.
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != "" {
			user, err := validateBearer(r.Context(), parts[1], kr, vkClient)
			if err == nil {
				return user, nil
			}
		}
	}

	// Fall back to X-API-Key.
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		keyCtx, user, err := validateAPIKey(r.Context(), r, apiKey, publicAPIService)
		if err == nil {
			return user, keyCtx
		}
	}

	return nil, nil
}

func validateBearer(ctx context.Context, token string, kr *auth.KeyRotation, vkClient valkeycompat.Cmdable) (*auth.AuthenticatedUser, error) {
	claims, err := kr.ValidateAccessToken(token)
	if err != nil {
		return nil, err
	}

	if auth.IsTokenRevoked(ctx, claims.JTI, vkClient) {
		return nil, errors.New("token revoked")
	}

	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		return nil, err
	}

	return &auth.AuthenticatedUser{
		UserID: userID,

		SessionID: claims.JTI,
	}, nil
}

func validateAPIKey(ctx context.Context, r *http.Request, apiKey string, publicAPIService *publicapi.Service) (*publicapi.APIKeyContext, *auth.AuthenticatedUser, error) {
	keyCtx, err := publicAPIService.ValidateAPIKey(ctx, apiKey)
	if err != nil {
		return nil, nil, err
	}

	// IP allowlist enforcement (same as public API middleware).
	clientIP := clientip.FromRequest(r)
	if clientIP != "" {
		allowed, err := publicAPIService.HasIPAccess(ctx, keyCtx.KeyID, clientIP)
		if err != nil {
			return nil, nil, err
		}

		if !allowed {
			return nil, nil, errors.New("IP not allowed")
		}
	}

	user := &auth.AuthenticatedUser{
		UserID: keyCtx.UserID,
	}

	return keyCtx, user, nil
}
