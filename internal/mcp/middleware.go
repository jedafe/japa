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

// PublicCORS answers CORS preflights and tags responses on the machine-facing
// MCP + OAuth endpoints. These endpoints serve public OAuth clients and the
// stateless tool surface — no cookie credentials are involved — so the policy
// is permissive. It exists so browser-based MCP tooling (e.g. the Inspector
// web UI) can reach the endpoints; the hosted connectors are server-to-server
// and never need CORS. The consent page (/oauth/authorize) is deliberately
// excluded: it is a top-level navigation, and a permissive ACAO on a cookie-
// authenticated surface would be wrong.
func PublicCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, MCP-Protocol-Version, MCP-Session-Id")
		w.Header().Set("Access-Control-Expose-Headers", "WWW-Authenticate, MCP-Session-Id")
		// The global corsMiddleware may have set ACAC:true for an allowlisted
		// origin — drop it, since ACAO:* + ACAC:true is invalid for credentialed
		// fetches and these endpoints are credential-free.
		w.Header().Del("Access-Control-Allow-Credentials")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// NoContent answers OPTIONS preflights on CORS-wrapped routes. It never
// actually runs — PublicCORS short-circuits OPTIONS — but chi requires a
// registered handler for the method to match.
var NoContent = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

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

// ResolveBearer resolves a §120 opaque access token into the synthetic
// APIKeyContext the public API enforces (§120 v1.4 — "Login with
// JapaConnect" tokens work anywhere X-API-Key is accepted on /api/v1/public).
// Only tokens carrying the MCP prefix are looked up — an arbitrary non-JWT
// bearer string never reaches the DB. publicapi.APIKeyMiddleware consumes
// this via its BearerTokenResolver parameter.
func (s *OAuthService) ResolveBearer(ctx context.Context, token string) (*publicapi.APIKeyContext, error) {
	if !strings.HasPrefix(token, oauthAccessTokenPrefix) {
		return nil, errors.New("not an oauth access token")
	}

	userID, scopes, clientID, err := s.ResolveAccessToken(ctx, token)
	if err != nil {
		return nil, err
	}

	return &publicapi.APIKeyContext{
		UserID: userID,
		Scopes: scopes,
		// KeyPrefix feeds the sliding-window rate key jc:api_rate:{prefix}.
		// Without the user id, every user of one client (e.g. all claude.ai
		// users of the registered connector) would share a single rate
		// bucket and throttle each other.
		KeyPrefix: "oauth:" + clientID + ":" + userID.String(),
		// OAuth tokens don't carry a stored tier; the effective tier is the
		// owner's unified-tier cap (premium ask → per-user ceiling), which
		// falls back to "basic" when the resolver is unwired or errors.
		RateLimitTier: publicapi.CapRateLimitTier(ctx, s.tierResolver, userID, "premium"),
	}, nil
}

// validateOAuthBearer wraps ResolveBearer for the MCP auth middleware,
// adding the authenticated-user shape alongside the synthetic key context.
func validateOAuthBearer(ctx context.Context, token string, oauthSvc *OAuthService) (*publicapi.APIKeyContext, *auth.AuthenticatedUser, error) {
	if oauthSvc == nil {
		return nil, nil, errors.New("oauth disabled")
	}
	keyCtx, err := oauthSvc.ResolveBearer(ctx, token)
	if err != nil {
		return nil, nil, err
	}

	return keyCtx, &auth.AuthenticatedUser{UserID: keyCtx.UserID}, nil
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
