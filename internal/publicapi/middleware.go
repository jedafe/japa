package publicapi

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/clientip"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// oauthBearerTokenPrefix is the §120 access-token prefix ("jcpat_"). It is
// a wire contract, not an implementation detail: the middleware gates the
// resolver call on it so foreign Bearer tokens (platform JWTs, other
// schemes, garbage) fall through to the anonymous path exactly as before
// instead of 401ing through the OAuth resolver.
const oauthBearerTokenPrefix = "jcpat_"

// BearerTokenResolver resolves a non-API-key bearer credential (a §120
// OAuth access token) into the synthetic key context RequireScope enforces.
// Implemented by mcp.OAuthService.ResolveBearer; nil disables bearer auth.
type BearerTokenResolver func(ctx context.Context, token string) (*APIKeyContext, error)

// APIKeyMiddleware extracts the X-API-Key header, validates it, and
// injects the API key context into the request. Downstream handlers
// can read the key info via APIKeyFromContext. When no X-API-Key is
// present, an `Authorization: Bearer jcpat_*` token is resolved through
// resolveBearer (§120 v1.4 — "Login with JapaConnect" OAuth grants work
// anywhere an API key is accepted on the public surface).
func APIKeyMiddleware(service *Service, resolveBearer BearerTokenResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				// §120 v1.4: OAuth bearer fallback — a resolved token
				// produces the same APIKeyContext an API key would. Only
				// jcpat_-prefixed tokens go to the resolver; any other
				// Bearer value leaves the request unauthenticated, so
				// anonymous-allowed routes (OptionalScope) keep working
				// for callers carrying unrelated Authorization headers.
				if resolveBearer != nil {
					if bearer := bearerToken(r); strings.HasPrefix(bearer, oauthBearerTokenPrefix) {
						keyCtx, err := resolveBearer(r.Context(), bearer)
						if err != nil {
							errpkg.WriteError(w, http.StatusUnauthorized, "invalid_bearer_token", "Invalid or expired bearer token")
							return
						}
						r = r.WithContext(context.WithValue(r.Context(), apiKeyContextKey, keyCtx))
					}
				}
				next.ServeHTTP(w, r)
				return
			}

			keyCtx, err := service.ValidateAPIKey(r.Context(), apiKey)
			if err != nil {
				slog.Warn("api key validation failed", "error", err)
				errpkg.WriteError(w, http.StatusUnauthorized, "invalid_api_key", "Invalid or expired API key")

				return
			}

			// IP allowlist enforcement: if the key has IP restrictions,
			// verify the client IP is within the allowed CIDR ranges.
			// An unidentifiable client IP (empty peer address — unix
			// socket, broken transport) fails closed when restrictions
			// exist: an allowlisted key must never pass unchecked.
			// Unrestricted keys (no allowlist rows) pass through.
			clientIP := extractClientIP(r)
			if clientIP == "" {
				restricted, err := service.HasIPRestrictions(r.Context(), keyCtx.KeyID)
				if err != nil {
					slog.Error("api key: IP restriction check failed", "error", err)
					errpkg.WriteError(w, http.StatusInternalServerError, "ip_check_error", "Failed to verify IP access")
					return
				}
				if restricted {
					slog.Warn("api key: empty client IP, denying allowlisted key",
						"key_id", keyCtx.KeyID.String())
					errpkg.WriteError(w, http.StatusForbidden, "ip_not_allowed", "Your IP address is not authorized for this API key")
					return
				}
			} else {
				allowed, err := service.HasIPAccess(r.Context(), keyCtx.KeyID, clientIP)
				if err != nil {
					slog.Error("api key: IP allowlist check failed", "error", err)
					errpkg.WriteError(w, http.StatusInternalServerError, "ip_check_error", "Failed to verify IP access")

					return
				}

				if !allowed {
					slog.Warn("api key: IP not in allowlist",
						"key_id", keyCtx.KeyID.String(),
						"client_ip", clientIP)
					errpkg.WriteError(w, http.StatusForbidden, "ip_not_allowed", "Your IP address is not authorized for this API key")

					return
				}
			}

			ctx := context.WithValue(r.Context(), apiKeyContextKey, keyCtx)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the raw token from an Authorization: Bearer header.
func bearerToken(r *http.Request) string {
	ah := r.Header.Get("Authorization")
	if ah == "" {
		return ""
	}
	parts := strings.SplitN(ah, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// extractClientIP resolves the client's IP address, trusting forwarding
// headers only from known proxies. The per-key IP allowlist is enforced on
// this value, so an attacker-supplied header must never reach it.
func extractClientIP(r *http.Request) string {
	return clientip.FromRequest(r)
}

// RequireScope checks that the validated API key includes the given scope.
// Scopes follow the pattern "domain.action" (e.g., "content.create").
// Fail-closed: if no API key is present, returns 401 instead of passing
// through (PUB-015).
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyCtx := APIKeyFromContext(r.Context())
			if keyCtx == nil {
				errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "An X-API-Key header or OAuth Bearer access token is required")
				return
			}

			if !HasScope(keyCtx.Scopes, scope) {
				errpkg.WriteError(w, http.StatusForbidden, "insufficient_scope", "credential lacks required scope: "+scope)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OptionalScope is RequireScope's sibling for routes that also serve
// anonymous callers on a named allowlist. Used by feeds/{name} (Section 47
// extension digest): a request with no API key passes only when the
// chi {name} URL param is in anonNames; any other anonymous name still
// fails closed with 401. A present key is scope-enforced exactly like
// RequireScope — anonymous leniency never weakens a keyed caller's check.
//
// r.With middleware runs after routing, so chi.URLParam is populated.
func OptionalScope(scope string, anonNames ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyCtx := APIKeyFromContext(r.Context())
			if keyCtx == nil {
				name := chi.URLParam(r, "name")
				for _, anon := range anonNames {
					if name == anon {
						next.ServeHTTP(w, r)
						return
					}
				}
				errpkg.WriteError(w, http.StatusUnauthorized, "api_key_required", "An X-API-Key header or OAuth Bearer access token is required")
				return
			}

			if !HasScope(keyCtx.Scopes, scope) {
				errpkg.WriteError(w, http.StatusForbidden, "insufficient_scope", "credential lacks required scope: "+scope)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AnonymousRateLimit applies inner only to requests without a validated
// API key. The feeds/{name} route uses it so anonymous callers get the
// loose IP-based limit while keyed callers keep only their per-key tier
// limiter — the two must not stack (a shared-NAT keyed caller would be
// capped below its tier).
func AnonymousRateLimit(inner func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := inner(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if APIKeyFromContext(r.Context()) == nil {
				limited.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HasScope checks if a scope list includes the given scope or a wildcard match.
func HasScope(scopes []string, required string) bool {
	for _, s := range scopes {
		if s == required || s == "*" {
			return true
		}
		// Wildcard matching: "content.*" matches "content.create"
		if strings.HasSuffix(s, ".*") {
			prefix := s[:len(s)-2]
			if strings.HasPrefix(required, prefix+".") {
				return true
			}
		}
	}

	return false
}

// RateLimitMiddleware enforces per-API-key rate limiting using a sliding-window
// counter backed by Valkey sorted sets. Keys without a validated context
// (no X-API-Key header) are passed through — they are already covered by the
// loose tier on /api/v1. Keys with an "unlimited" tier bypass the limiter.
// When Valkey is unreachable the middleware degrades gracefully and allows
// the request through.
//
// Deprecated: Use SlidingWindowRateLimit(rdb) directly. This wrapper
// exists for backward compatibility.
func RateLimitMiddleware(rdb valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return SlidingWindowRateLimit(rdb)
}

// getTierLimit returns the per-minute request limit for a rate limit tier.
// These are the compiled-in fallbacks used when no settings reader is wired
// (SEC-012). When a settings reader is available, resolveTierLimit reads
// the `api.rate_limit.{tier}` system_settings keys instead.
func getTierLimit(tier string) int {
	switch tier {
	case "standard":
		return 1000
	case "premium":
		return 10000
	default:
		return 100 // basic
	}
}

// resolveTierLimit returns the per-minute request limit for a tier, reading
// from system_settings when a settings reader is wired (SEC-012 / Rule 3),
// falling back to the compiled-in getTierLimit defaults otherwise. The
// settings keys are `api.rate_limit.basic`, `api.rate_limit.standard`,
// `api.rate_limit.premium` (see system-settings-catalog).
func resolveTierLimit(ctx context.Context, tier string, sr rateLimitSettingsReader) int {
	if sr == nil {
		return getTierLimit(tier)
	}

	switch tier {
	case "standard":
		return sr.GetIntOr(ctx, "api.rate_limit.standard", 1000)
	case "premium":
		return sr.GetIntOr(ctx, "api.rate_limit.premium", 10000)
	default:
		return sr.GetIntOr(ctx, "api.rate_limit.basic", 100)
	}
}

// userTierToRateLimitCap maps a unified user tier to the highest API rate
// limit tier that user may receive. regular -> basic, pro -> standard,
// business -> premium.
var userTierToRateLimitCap = map[string]string{
	"regular":  "basic",
	"pro":      "standard",
	"business": "premium",
}

// rateLimitTierRanks orders API key rate limit tiers from lowest to highest.
var rateLimitTierRanks = map[string]int{
	"basic":     1,
	"standard":  2,
	"premium":   3,
	"unlimited": 4,
}

// rateLimitRankToTier is the inverse of rateLimitTierRanks.
var rateLimitRankToTier = map[int]string{
	1: "basic",
	2: "standard",
	3: "premium",
	4: "unlimited",
}

// CapRateLimitTier resolves the user's unified tier and returns the lower of
// the API key's rate_limit_tier and the user's cap tier. If the user tier
// resolver fails, the cap falls back to "basic" for safety. Exported for
// credential resolvers (§120 OAuth bearer tokens) that mint a synthetic
// key context rather than reading a stored key tier.
func CapRateLimitTier(ctx context.Context, tr TierResolver, userID pgtype.UUID, keyTier string) string {
	userTier := "regular"

	if tr != nil {
		resolved, err := tr.ResolveUserTier(ctx, userID)
		if err == nil && resolved != "" {
			userTier = resolved
		}
	}

	capTier, ok := userTierToRateLimitCap[userTier]
	if !ok {
		capTier = "basic"
	}

	keyRank, ok := rateLimitTierRanks[keyTier]
	if !ok {
		keyRank = rateLimitTierRanks["basic"]
	}

	capRank, ok := rateLimitTierRanks[capTier]
	if !ok {
		capRank = rateLimitTierRanks["basic"]
	}

	if keyRank <= capRank {
		return keyTier
	}

	return rateLimitRankToTier[capRank]
}
