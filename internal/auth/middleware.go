package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// UserContextKey is the context key for the authenticated user.
type contextKey struct{ name string }

// UserContextKey is the user context key.
var UserContextKey = contextKey{"user"}

// AuthenticatedUser holds the user info extracted from a validated JWT.
type AuthenticatedUser struct {
	UserID   pgtype.UUID
	TenantID pgtype.UUID
}

// FromContext extracts the AuthenticatedUser from the request context.
func FromContext(ctx context.Context) *AuthenticatedUser {
	u, _ := ctx.Value(UserContextKey).(*AuthenticatedUser)
	return u
}

// RequireAuth is middleware that requires a valid Bearer token.
// It validates the JWT, checks if the JTI has been revoked via Valkey,
// and stores the AuthenticatedUser in context.
// Returns 401 if no token, invalid token, wrong scheme, or revoked token.
func RequireAuth(kr *KeyRotation, vkClient valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeError(w, http.StatusUnauthorized, "missing_token", "Authorization header is required")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				writeError(w, http.StatusUnauthorized, "invalid_auth_scheme", "Expected: Bearer <token>")
				return
			}

			claims, err := kr.ValidateAccessToken(parts[1])
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Token is invalid or expired")
				return
			}

			// Check if this specific token has been revoked.
			if IsTokenRevoked(r.Context(), claims.JTI, vkClient) {
				writeError(w, http.StatusUnauthorized, "token_revoked", "Token has been revoked")
				return
			}

			var userID pgtype.UUID
			if err := userID.Scan(claims.UserID); err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Token contains invalid user ID")
				return
			}

			var tenantID pgtype.UUID
			if err := tenantID.Scan(claims.TenantID); err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_token", "Token contains invalid tenant ID")
				return
			}

			user := &AuthenticatedUser{
				UserID:   userID,
				TenantID: tenantID,
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth is middleware that validates a Bearer token if present
// but does not require it. Useful for endpoints that show different
// content for authenticated vs anonymous users. Also checks JTI
// revocation if a Valkey client is provided.
func OptionalAuth(kr *KeyRotation, vkClient valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
				next.ServeHTTP(w, r)
				return
			}

			claims, err := kr.ValidateAccessToken(parts[1])
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}

			// Check if this specific token has been revoked.
			if IsTokenRevoked(r.Context(), claims.JTI, vkClient) {
				next.ServeHTTP(w, r)
				return
			}

			var userID pgtype.UUID
			if err := userID.Scan(claims.UserID); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			var tenantID pgtype.UUID
			if err := tenantID.Scan(claims.TenantID); err != nil {
				next.ServeHTTP(w, r)
				return
			}

			user := &AuthenticatedUser{
				UserID:   userID,
				TenantID: tenantID,
			}

			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
