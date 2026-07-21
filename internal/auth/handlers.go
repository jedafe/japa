package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/email"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/csrf"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// AuthHandlers holds dependencies for auth HTTP handlers.
type AuthHandlers struct { //nolint:revive // stuttering type name preserved for API compatibility
	DB           Querier
	Queries      *gen.Queries
	Database     *db.DB         // Raw DB access for operations not yet in sqlc (e.g., backup code update)
	KeyRotation  *KeyRotation
	BruteForce   BruteForceStore
	Policy       PasswordPolicy
	AsynqClient  *asynq.Client
	VKClient     *valkey.Client   // Valkey client for per-email rate limiting
	Settings     SettingsReader   // Hot-reloadable system settings for auth token TTLs
	SiteName     string           // Brand name for email templates (e.g. "JapaConnect")
	SupportURL   string           // Support page URL for email footers
	CookieDomain  string // Cookie domain for cross-subdomain sharing (e.g. ".example.com"), empty = exact host
	SecureCookies bool   // When true, session/CSRF cookies are marked Secure (production / TLS)
}

// Querier is the interface for database operations needed by auth handlers.
type Querier interface {
	GetUserByEmail(ctx context.Context, tenantID pgtype.UUID, email string) (*User, error)
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error

	// Password reset
	GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error)
	MarkResetTokenUsed(ctx context.Context, tokenHash string) error
	UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error

	// Email verification
	GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error)
	MarkEmailVerified(ctx context.Context, userID pgtype.UUID) error

	// Password history
	GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error)

	// Group membership
	IsUserInGroup(ctx context.Context, arg gen.IsUserInGroupParams) (bool, error)
}

// User represents a user record from the database.
type User struct {
	ID              pgtype.UUID
	TenantID        pgtype.UUID
	Email           string
	PasswordHash    pgtype.Text
	Username        string
	ProfileType     string
	Status          string
	EmailVerifiedAt pgtype.Timestamptz
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

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode response", "error", err)
	}
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
		// Check if this is a breach error — different error code for monitoring.
		var breached *BreachedError
		if errors.As(err, &breached) {
			slog.Warn(
				"registration blocked: breached password",
				"breach_count", breached.Count,
			)
			writeError(w, http.StatusBadRequest, "password_breached",
				"This password has appeared in a data breach and cannot be used. Please choose a different password.")

			return
		}

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

	userID, err := h.Queries.CreateUser(r.Context(), gen.CreateUserParams{
		TenantID:     tenant.ID,
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: &passwordHash,
	})
	if err != nil {
		if errors.Is(err, errpkg.ErrDuplicate) {
			writeError(w, http.StatusConflict, "email_exists", "Email or username already in use")
			return
		}

		writeError(w, http.StatusInternalServerError, "create_failed", "Failed to create user")

		return
	}

	// Record the initial password in history so it cannot be reused later.
	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		TenantID:     tenant.ID,
		UserID:       userID,
		PasswordHash: passwordHash,
	}); err != nil {
		slog.Warn("register: failed to record password history", "error", err)
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
	refreshTTL := getRefreshTokenTTL(r.Context(), h.Settings, tenant.ID)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		TenantID:  tenant.ID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		Domain:   h.CookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
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

	// Check brute-force lockout — read max attempts from tenant settings.
	maxAttempts := DefaultMaxLoginAttempts
	if h.Settings != nil {
		maxAttempts = MaxLoginAttempts(r.Context(), h.Settings, tenant.ID)
	}
	allowed, retryAfter := CheckLoginAttempt(r.Context(), h.BruteForce, bruteKey, maxAttempts)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "Too many failed login attempts. Please try again later.")

		return
	}

	user, err := h.DB.GetUserByEmail(r.Context(), tenant.ID, req.Email)
	if err != nil || user == nil {
		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	if !user.PasswordHash.Valid {
		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	if err := VerifyPassword(user.PasswordHash.String, req.Password); err != nil {
		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		writeError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	if user.Status != "active" {
		writeError(w, http.StatusForbidden, "account_inactive", "Account is not active")
		return
	}

	// Check email verification requirement from system_settings.
	// Super admins bypass email verification — they are platform operators.
	if EmailVerifyRequired(r.Context(), h.Settings, tenant.ID) && !user.EmailVerifiedAt.Valid {
		isSuperAdmin, err := h.DB.IsUserInGroup(r.Context(), gen.IsUserInGroupParams{
			TenantID:  tenant.ID,
			UserID:   user.ID,
			GroupSlug: "super_admin",
		})
		if err != nil {
			slog.Warn("check super admin group", "error", err)
		}

		if !isSuperAdmin {
			writeError(w, http.StatusForbidden, "email_not_verified", "Email verification is required before login")
			return
		}
	}

	// Reset brute-force counter on successful login
	if err := ResetLoginAttempts(r.Context(), h.BruteForce, bruteKey); err != nil {
		slog.Warn("reset login attempts", "error", err)
	}

	// Check if 2FA is enabled — if so, require TOTP challenge before
	// issuing a full session (AUDIT-BE-001).
	hasTOTP, err := h.Queries.CheckTOTPEnabled(r.Context(), gen.CheckTOTPEnabledParams{
		TenantID: user.TenantID,
		UserID:   user.ID,
	})
	if err != nil {
		slog.Warn("check TOTP enabled", "error", err)
		hasTOTP = false
	}

	if hasTOTP {
		challengeToken, err := h.KeyRotation.MintTOTPChallengeToken(user.ID, user.TenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "token_error", "Failed to create challenge token")
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"totp_required":   true,
				"challenge_token": challengeToken,
				"user": map[string]interface{}{
					"id":       user.ID.String(),
					"email":    user.Email,
					"username": user.Username,
				},
			},
		})

		return
	}

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
	refreshTTL := getRefreshTokenTTL(r.Context(), h.Settings, user.TenantID)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		TenantID:  user.TenantID,
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		Domain:   h.CookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	// Issue the double-submit CSRF cookie so the SPA can include it as the
	// X-CSRF-Token header on subsequent mutating requests (audit finding C1).
	csrf.SetCookie(w, h.CookieDomain, h.SecureCookies)

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

// VerifyLoginChallenge handles POST /api/v1/auth/totp/verify-challenge.
// Accepts a challenge token from Login (when totp_required=true) and a TOTP
// code. On success, issues a full access+refresh token pair.
func (h *AuthHandlers) VerifyLoginChallenge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChallengeToken string `json:"challenge_token"`
		Code           string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ChallengeToken == "" || req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "challenge_token and code are required")
		return
	}

	claims, err := h.KeyRotation.ValidateChallengeToken(req.ChallengeToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid or expired challenge token")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(claims.TenantID); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "Verification code is required")
		return
	}

	totpRecord, err := h.Queries.GetTOTPCredentials(r.Context(), gen.GetTOTPCredentialsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil || !totpRecord.EnabledAt.Valid {
		writeError(w, http.StatusBadRequest, "totp_error", "2FA not set up")
		return
	}

	plaintext, err := decryptSecret(totpRecord.SecretEncrypted)
	if err != nil {
		slog.Error("decrypt TOTP secret", "error", err)
		writeError(w, http.StatusInternalServerError, "totp_error", "Failed to verify 2FA")
		return
	}

	if !VerifyTOTP(plaintext, req.Code) {
		writeError(w, http.StatusUnauthorized, "invalid_totp", "Invalid verification code")
		return
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID, tenantID)
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
	refreshTTL := getRefreshTokenTTL(r.Context(), h.Settings, tenantID)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		Domain:   h.CookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	csrf.SetCookie(w, h.CookieDomain, h.SecureCookies)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token": accessToken,
		},
	})
}

// ConsumeBackupCode handles POST /api/v1/auth/totp/backup.
// Accepts a challenge token and a one-time backup code. On success,
// the backup code is consumed and a full session is issued.
func (h *AuthHandlers) ConsumeBackupCode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ChallengeToken string `json:"challenge_token"`
		BackupCode     string `json:"backup_code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ChallengeToken == "" || req.BackupCode == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "challenge_token and backup_code are required")
		return
	}

	claims, err := h.KeyRotation.ValidateChallengeToken(req.ChallengeToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid or expired challenge token")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	var tenantID pgtype.UUID
	if err := tenantID.Scan(claims.TenantID); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	req.BackupCode = sanitizeCode(req.BackupCode)

	backupRecord, err := h.Queries.GetTOTPCredentials(r.Context(), gen.GetTOTPCredentialsParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		writeError(w, http.StatusUnauthorized, "no_backup_codes", "No backup codes found")
		return
	}

	matched, remaining, err := consumeBackupCode(r.Context(), h.Database, &backupRecord, req.BackupCode, tenantID, userID)
	if err != nil {
		slog.Error("consume backup code", "error", err)
		writeError(w, http.StatusInternalServerError, "backup_error", "Failed to verify backup code")
		return
	}

	if !matched {
		writeError(w, http.StatusUnauthorized, "invalid_backup", "Invalid backup code")
		return
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID, tenantID)
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
	refreshTTL := getRefreshTokenTTL(r.Context(), h.Settings, tenantID)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		TenantID:  tenantID,
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1/auth",
		Domain:   h.CookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	csrf.SetCookie(w, h.CookieDomain, h.SecureCookies)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token":    accessToken,
			"codes_remaining": remaining,
		},
	})
}

// Logout handles POST /api/v1/auth/logout.
// Revokes the refresh token, revokes the current access token's JTI,
// and clears the cookie.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("jc_refresh")
	if err == nil {
		tokenHash := hashRefreshToken(cookie.Value)
		if err := h.DB.RevokeRefreshToken(r.Context(), tokenHash); err != nil {
			slog.Error("revoke refresh token", "error", err)
		}
	}

	// Revoke the current access token's JTI if present in the Authorization header.
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
			if claims, err := h.KeyRotation.ValidateAccessToken(parts[1]); err == nil {
				if h.VKClient != nil {
					if err := RevokeToken(r.Context(), claims.JTI, claims.ExpiresAt.Time, h.VKClient.Redis()); err != nil {
						slog.Error("revoke access token JTI", "error", err)
					}
				}
			}
		}
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    "",
		Path:     "/api/v1/auth",
		Domain:   h.CookieDomain,
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

// rateLimitCheck performs a fixed-window rate limit check against Valkey using
// INCR + EXPIRE. Returns true when the request is allowed (count <= maxRequests),
// false when the limit has been exceeded. When Valkey is unreachable, the check
// degrades gracefully and allows the request through — a missing cache layer
// should never block a legitimate password reset or verification email.
func rateLimitCheck(ctx context.Context, rdb valkeycompat.Cmdable, key string, maxRequests int, window time.Duration) (bool, error) {
	if rdb == nil {
		return true, nil
	}

	pipe := rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window+time.Second)

	if _, err := pipe.Exec(ctx); err != nil {
		// Valkey unreachable — degrade gracefully, allow the request.
		return true, err
	}

	return incrCmd.Val() <= int64(maxRequests), nil
}

// HandleForgotPassword handles POST /api/v1/auth/forgot-password.
// Always returns 200 to prevent email enumeration. Rate limited to 3 requests per 10 minutes per email.
func (h *AuthHandlers) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Email is required")
		return
	}

	// Per-email rate limit: 3 requests per 10 minutes. Prevents a single
	// email address from being used to flood the reset-email queue.
	if h.VKClient != nil {
		emailKey := "jc:ratelimit:forgot_password:" + req.Email

		ok, _ := rateLimitCheck(r.Context(), h.VKClient.Redis(), emailKey, 3, 10*time.Minute)
		if !ok {
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many password reset requests. Please try again later.")
			return
		}
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Always return 200 to prevent email enumeration — whether the user exists or not.
	defer func() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "If an account with that email exists, a reset link has been sent.",
			},
		})
	}()

	user, err := h.DB.GetUserByEmail(r.Context(), t.ID, req.Email)
	if err != nil || user == nil {
		// Silently return success — don't reveal whether the email exists.
		return
	}

	// Only generate tokens for active users with a password (not OAuth-only).
	if user.Status != "active" || !user.PasswordHash.Valid {
		return
	}

	token, hash, err := GenerateResetToken()
	if err != nil {
		slog.Error("forgot-password: failed to generate token", "error", err)
		return
	}

	expiresAt := time.Now().Add(ResetTokenTTL)
	if err := h.Queries.CreatePasswordResetToken(r.Context(), gen.CreatePasswordResetTokenParams{
		TenantID:  t.ID,
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		slog.Error("forgot-password: failed to store token", "error", err)
		return
	}

	// Build the base URL from the request to support any deployment domain.
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}

	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Render the HTML email body.
	siteName := h.SiteName
	if siteName == "" {
		siteName = "App"
	}

	supportURL := h.SupportURL
	if supportURL == "" {
		supportURL = baseURL + "/support"
	}

	htmlBody, err := email.RenderPasswordResetEmail(email.TemplateData{
		SiteName:   siteName,
		SiteURL:    baseURL,
		UserName:   user.Username,
		ActionURL:  fmt.Sprintf("%s/auth/reset-password?token=%s", baseURL, token),
		Expiry:     "1 hour",
		SupportURL: supportURL,
	})
	if err != nil {
		slog.Error("forgot-password: failed to render email template", "error", err)
		return
	}

	// Enqueue email delivery — best effort, failures are logged.
	h.enqueueEmail(user.Email, "Reset Your Password", htmlBody, true)
}

// HandleResetPasswordPage handles GET /api/v1/auth/reset-password.
// Validates the token and returns whether it's valid without consuming it.
func (h *AuthHandlers) HandleResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing_token", "Token query parameter is required")
		return
	}

	tokenHash := hashRefreshToken(token)

	rec, err := h.DB.GetValidResetToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{"valid": false},
		})

		return
	}

	// Check expiry and whether it's been used.
	if time.Now().After(rec.ExpiresAt) || rec.UsedAt.Valid {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{"valid": false},
		})

		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"valid": true},
	})
}

// HandleResetPassword handles POST /api/v1/auth/reset-password.
// Validates the token, updates the password, and invalidates all sessions.
func (h *AuthHandlers) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.Token == "" || req.NewPassword == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Token and new_password are required")
		return
	}

	if err := ValidatePassword(req.NewPassword, h.Policy); err != nil {
		// Check if this is a breach error — different error code for monitoring.
		var breached *BreachedError
		if errors.As(err, &breached) {
			slog.Warn(
				"password reset blocked: breached password",
				"token_prefix", req.Token[:min(8, len(req.Token))],
				"breach_count", breached.Count,
			)
			writeError(w, http.StatusBadRequest, "password_breached",
				"This password has appeared in a data breach and cannot be used. Please choose a different password.")

			return
		}

		writeError(w, http.StatusBadRequest, "weak_password", err.Error())

		return
	}

	tokenHash := hashRefreshToken(req.Token)

	rec, err := h.DB.GetValidResetToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired reset token")
		return
	}

	if time.Now().After(rec.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "expired_token", "Reset token has expired")
		return
	}

	if rec.UsedAt.Valid {
		writeError(w, http.StatusUnauthorized, "used_token", "Reset token has already been used")
		return
	}

	// Check password history before allowing the reset.
	if err := CheckPasswordHistory(r.Context(), h.DB, rec.UserID, req.NewPassword, h.Policy); err != nil {
		var reused *PasswordReusedError
		if errors.As(err, &reused) {
			slog.Warn(
				"password reset blocked: reused password",
				"token_prefix", req.Token[:min(8, len(req.Token))],
			)
			writeError(w, http.StatusBadRequest, "password_reused",
				"New password cannot be the same as a recent password. Please choose a different password.")

			return
		}
	}

	// Mark the token as used before updating the password.
	if err := h.DB.MarkResetTokenUsed(r.Context(), tokenHash); err != nil {
		slog.Error("reset-password: failed to mark token used", "error", err)
		writeError(w, http.StatusInternalServerError, "token_error", "Failed to process reset token")

		return
	}

	passwordHash, err := HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "hash_error", "Failed to secure password")
		return
	}

	if err := h.DB.UpdateUserPassword(r.Context(), rec.UserID, passwordHash); err != nil {
		slog.Error("reset-password: failed to update password", "error", err)
		writeError(w, http.StatusInternalServerError, "update_failed", "Failed to update password")

		return
	}

	// Record the new password in history to prevent reuse.
	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		TenantID:     rec.TenantID,
		UserID:       rec.UserID,
		PasswordHash: passwordHash,
	}); err != nil {
		slog.Warn("reset-password: failed to record password history", "error", err)
	}

	// Invalidate all sessions for this user.
	if err := InvalidateAllUserSessions(r.Context(), h.Queries, rec.UserID, rec.TenantID); err != nil {
		slog.Error("reset-password: failed to invalidate sessions", "user_id", rec.UserID.String(), "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{
			"message": "Password has been reset. Please log in with your new password.",
		},
	})
}

// HandleVerifyEmail handles GET /api/v1/auth/verify-email.
// Validates the verification token and marks the email as verified.
func (h *AuthHandlers) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		writeError(w, http.StatusBadRequest, "missing_token", "Token query parameter is required")
		return
	}

	tokenHash := hashRefreshToken(token)

	rec, err := h.DB.GetValidVerificationToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired verification token")
		return
	}

	if time.Now().After(rec.ExpiresAt) {
		writeError(w, http.StatusUnauthorized, "expired_token", "Verification token has expired")
		return
	}

	if rec.VerifiedAt.Valid {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "Email has already been verified.",
			},
		})

		return
	}

	if err := h.DB.MarkEmailVerified(r.Context(), rec.UserID); err != nil {
		slog.Error("verify-email: failed to mark verified", "error", err)
		writeError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify email")

		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{
			"message": "Email has been verified successfully.",
		},
	})
}

// HandleResendVerification handles POST /api/v1/auth/resend-verification.
// Generates a new verification token and enqueues an email. Rate limited to 1 request per 5 minutes per email.
func (h *AuthHandlers) HandleResendVerification(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "Email is required")
		return
	}

	// Per-email rate limit: 1 request per 5 minutes. Prevents a single
	// email address from being used to flood the verification-email queue.
	if h.VKClient != nil {
		emailKey := "jc:ratelimit:resend_verification:" + req.Email

		ok, _ := rateLimitCheck(r.Context(), h.VKClient.Redis(), emailKey, 1, 5*time.Minute)
		if !ok {
			writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many verification email requests. Please try again later.")
			return
		}
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Always return 200 to prevent email enumeration.
	defer func() {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "If an account with that email exists and is unverified, a new verification link has been sent.",
			},
		})
	}()

	user, err := h.DB.GetUserByEmail(r.Context(), t.ID, req.Email)
	if err != nil || user == nil {
		return
	}

	// Don't resend if already verified.
	if user.Status != "active" {
		return
	}

	token, hash, err := GenerateVerificationToken()
	if err != nil {
		slog.Error("resend-verification: failed to generate token", "error", err)
		return
	}

	expiresAt := time.Now().Add(VerificationTokenTTL)
	if err := h.Queries.CreateEmailVerification(r.Context(), gen.CreateEmailVerificationParams{
		TenantID:  t.ID,
		UserID:    user.ID,
		Email:     user.Email,
		TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		slog.Error("resend-verification: failed to store token", "error", err)
		return
	}

	// Build the base URL from the request to support any deployment domain.
	scheme := "https"
	if r.TLS == nil && !strings.HasPrefix(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "http"
	}

	baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)

	// Render the HTML email body.
	siteName := h.SiteName
	if siteName == "" {
		siteName = "App"
	}

	supportURL := h.SupportURL
	if supportURL == "" {
		supportURL = baseURL + "/support"
	}

	htmlBody, err := email.RenderEmailVerification(email.TemplateData{
		SiteName:   siteName,
		SiteURL:    baseURL,
		UserName:   user.Username,
		ActionURL:  fmt.Sprintf("%s/auth/verify-email?token=%s", baseURL, token),
		Expiry:     "24 hours",
		SupportURL: supportURL,
	})
	if err != nil {
		slog.Error("resend-verification: failed to render email template", "error", err)
		return
	}

	h.enqueueEmail(user.Email, "Verify Your Email", htmlBody, true)
}

// enqueueEmail enqueues an email delivery task via Asynq. Failures are logged but not returned.
// When isHTML is true, the worker sends the body as text/html; otherwise as text/plain.
func (h *AuthHandlers) enqueueEmail(to, subject, body string, isHTML bool) {
	if h.AsynqClient == nil {
		slog.Warn("auth: enqueueEmail called without asynq client", "to", to)
		return
	}

	payload, err := json.Marshal(map[string]any{
		"to":      to,
		"subject": subject,
		"body":    body,
		"html":    isHTML,
	})
	if err != nil {
		slog.Error("auth: failed to marshal email payload", "error", err)
		return
	}

	task := asynq.NewTask(worker.TypeEmailSend, payload)
	// Use QueueForTask to route to the correct tier (default for emails).
	if _, err := h.AsynqClient.Enqueue(task, asynq.Queue(worker.QueueForTask(worker.TypeEmailSend)), asynq.Unique(1*time.Hour)); err != nil {
		slog.Error("auth: failed to enqueue email", "to", to, "error", err)
	}
}
