package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/tenant"
)

// AuthHandlers holds dependencies for auth HTTP handlers.
type AuthHandlers struct {
	DB          Querier
	KeyRotation *KeyRotation
	BruteForce  BruteForceStore
	Policy      PasswordPolicy
}

// Querier is the interface for database operations needed by auth handlers.
type Querier interface {
	CreateUser(ctx context.Context, tenantID pgtype.UUID, email, username, passwordHash string) (pgtype.UUID, error)
	GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error)
	StoreRefreshToken(ctx context.Context, tenantID, userID pgtype.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}

// User represents a user record from the database.
type User struct {
	ID           pgtype.UUID
	TenantID     pgtype.UUID
	Email        string
	PasswordHash pgtype.Text
	Username     string
	ProfileType  string
	Status       string
}

// RefreshToken represents a refresh token record.
type RefreshToken struct {
	ID        pgtype.UUID
	UserID    pgtype.UUID
	TenantID  pgtype.UUID
	TokenHash string
	ExpiresAt time.Time
	RevokedAt pgtype.Timestamptz
}

// RefreshTokenTTL is the validity period for refresh tokens.
const RefreshTokenTTL = 30 * 24 * time.Hour // 30 days

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a standardized error response.
func writeError(w http.ResponseWriter, code int, errCode, message string) {
	writeJSON(w, code, map[string]interface{}{
		"error": map[string]string{
			"code":    errCode,
			"message": message,
		},
	})
}

// Register handles POST /api/v1/auth/register.
// Creates a new user with email, username, and password.
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	if req.Email == "" || req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Email, username, and password are required")
		return
	}

	if err := ValidatePassword(req.Password, h.Policy); err != nil {
		writeError(w, http.StatusBadRequest, "weak_password", err.Error())
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash_error", "Failed to secure password")
		return
	}

	userID, err := h.DB.CreateUser(r.Context(), tenant.ID, req.Email, req.Username, passwordHash)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "email_exists", "Email or username already in use")
			return
		}
		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create user")
		return
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID, tenant.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := hashRefreshToken(refreshToken)
	expiresAt := time.Now().Add(RefreshTokenTTL)
	if err := h.DB.StoreRefreshToken(r.Context(), tenant.ID, userID, tokenHash, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		MaxAge:   int(RefreshTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token": accessToken,
			"user": map[string]interface{}{
				"id":       userID.String(),
				"email":    req.Email,
				"username": req.Username,
			},
		},
	})
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Email and password are required")
		return
	}

	tenant := tenant.FromContext(r.Context())
	if tenant == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Get client IP from request
	ip := clientIP(r)
	bruteKey := LoginIdentifier(req.Email, ip)

	// Check brute-force lockout
	allowed, retryAfter := CheckLoginAttempt(r.Context(), h.BruteForce, bruteKey)
	if !allowed {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many failed login attempts. Please try again later.")
		return
	}

	user, err := h.DB.GetUserByEmail(r.Context(), tenant.ID, req.Email)
	if err != nil || user == nil {
		_ = RecordFailedLogin(r.Context(), h.BruteForce, bruteKey)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if !user.PasswordHash.Valid {
		_ = RecordFailedLogin(r.Context(), h.BruteForce, bruteKey)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if err := VerifyPassword(user.PasswordHash.String, req.Password); err != nil {
		_ = RecordFailedLogin(r.Context(), h.BruteForce, bruteKey)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")
		return
	}

	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "account_inactive", "Account is not active")
		return
	}

	// Reset brute-force counter on successful login
	_ = ResetLoginAttempts(r.Context(), h.BruteForce, bruteKey)

	accessToken, err := h.KeyRotation.MintAccessToken(user.ID, user.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := generateRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := hashRefreshToken(refreshToken)
	expiresAt := time.Now().Add(RefreshTokenTTL)
	if err := h.DB.StoreRefreshToken(r.Context(), user.TenantID, user.ID, tokenHash, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		MaxAge:   int(RefreshTokenTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token": accessToken,
			"user": map[string]interface{}{
				"id":       user.ID.String(),
				"email":    user.Email,
				"username": user.Username,
			},
		},
	})
}

// Refresh handles POST /api/v1/auth/refresh.
// Exchanges a valid refresh token cookie for a new access token.
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("jc_refresh")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no_refresh_token", "Refresh token cookie is required")
		return
	}

	tokenHash := hashRefreshToken(cookie.Value)
	stored, err := h.DB.GetRefreshToken(r.Context(), tokenHash)
	if err != nil || stored == nil {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token is invalid or expired")
		return
	}

	if stored.RevokedAt.Valid {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has been revoked")
		return
	}

	if time.Now().After(stored.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has expired")
		return
	}

	accessToken, err := h.KeyRotation.MintAccessToken(stored.UserID, stored.TenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token": accessToken,
		},
	})
}

// Logout handles POST /api/v1/auth/logout.
// Revokes the refresh token and clears the cookie.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("jc_refresh")
	if err == nil {
		tokenHash := hashRefreshToken(cookie.Value)
		_ = h.DB.RevokeRefreshToken(r.Context(), tokenHash)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    "",
		Path:     "/api/v1/auth",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"status": "logged_out"},
	})
}

// clientIP extracts the client IP from the request, respecting
// X-Forwarded-For and X-Real-IP headers set by the reverse proxy.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr, stripping the port
	idx := strings.LastIndex(r.RemoteAddr, ":")
	if idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
