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

// AuthMiddleware returns middleware that accepts a Bearer JWT, an X-API-Key
// header, or a §120 OAuth bearer token for the MCP endpoint, then enforces
// the agent.use permission/scope before handing the request to the MCP
// handler. oauthSvc may be nil — OAuth bearer auth is then disabled.
// issuer is the API host's public base URL; it feeds the WWW-Authenticate
// resource_metadata pointer that lets MCP clients discover the AS.
func AuthMiddleware(kr *auth.KeyRotation, vkClient valkeycompat.Cmdable, publicAPIService *publicapi.Service, resolver *rbac.RBAC, oauthSvc *OAuthService, issuer string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, apiKey := authOrAPIKey(r, kr, vkClient, publicAPIService, oauthSvc)
			if user == nil || !user.UserID.Valid {
				challenge := "Bearer"
				if issuer != "" {
					challenge = `Bearer resource_metadata="` + issuer + `/.well-known/oauth-protected-resource"`
				}
				// RFC 6750 §3.1: distinguish "no credentials" from
				// "credentials presented but rejected".
				if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
					challenge += `, error="invalid_token"`
				}
				w.Header().Set("WWW-Authenticate", challenge)
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
				// OAuth callers reuse this path — they carry the synthetic
				// key context produced by ResolveAccessToken.
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

func authOrAPIKey(r *http.Request, kr *auth.KeyRotation, vkClient valkeycompat.Cmdable, publicAPIService *publicapi.Service, oauthSvc *OAuthService) (*auth.AuthenticatedUser, *publicapi.APIKeyContext) {
	// Prefer Bearer when present. JWT first; a token that fails JWT
	// validation falls through to the §120 OAuth access-token lookup.
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] != "" {
			if user, err := validateBearer(r.Context(), parts[1], kr, vkClient); err == nil {
				return user, nil
			}
			if keyCtx, user, err := validateOAuthBearer(r.Context(), parts[1], oauthSvc); err == nil {
				return user, keyCtx
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

// validateOAuthBearer resolves a §120 opaque access token into a synthetic
// APIKeyContext so the read-only tool gate, per-scope checks, and rate
// limiter apply unchanged. Only tokens carrying the MCP prefix are looked
// up — an arbitrary non-JWT bearer string never reaches the DB.
func validateOAuthBearer(ctx context.Context, token string, oauthSvc *OAuthService) (*publicapi.APIKeyContext, *auth.AuthenticatedUser, error) {
	if oauthSvc == nil || !strings.HasPrefix(token, oauthAccessTokenPrefix) {
		return nil, nil, errors.New("not an oauth access token")
	}

	userID, scopes, clientID, err := oauthSvc.ResolveAccessToken(ctx, token)
	if err != nil {
		return nil, nil, err
	}

	keyCtx := &publicapi.APIKeyContext{
		UserID:        userID,
		Scopes:        scopes,
		KeyPrefix:     "oauth:" + clientID,
		RateLimitTier: "basic",
	}
	user := &auth.AuthenticatedUser{UserID: userID}

	return keyCtx, user, nil
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
