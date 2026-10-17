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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/valkey-io/valkey-go/valkeycompat"

	"github.com/jedafe/japa/internal/clientip"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/email"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/valkey"
	"github.com/jedafe/japa/internal/worker"
)

// ReferralCapturer captures a referral code at signup, creating a pending
// referral record linking the referrer to the newly registered user.
// Implemented by referral.Service; injected via SetReferralCapturer so the
// auth package does not import referral (avoids an import cycle).
// Ref: japaconnect-engineering/sections/60-referral-system.md (Attribution).
type ReferralCapturer interface {
	CaptureSignupReferral(ctx context.Context, refereeUserID pgtype.UUID, refCode string) error
}

// DefaultFolderProvider pre-seeds a user's default bookmark folders at
// registration. Implemented by bookmarkfolders.DefaultFolderResolver; the
// interface lives here so auth does not import bookmarkfolders directly.
// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md.
type DefaultFolderProvider interface {
	CreateDefaultFoldersForUser(ctx context.Context, userID pgtype.UUID) error
}

// TierManager assigns a new user to the regular/pro/business tier group and
// syncs users.profile_type. Implemented by the tiers package; the interface
// lives here so auth does not import tiers directly.
type TierManager interface {
	SetUserTier(ctx context.Context, userID pgtype.UUID, tier string) error
}

// SourceManager creates canonical intel_sources rows for new entity profiles
// (v2 source directory, Task 5).
type SourceManager interface {
	CreateSourceFromEntity(ctx context.Context, entityID, userID pgtype.UUID, entityType, name, slug, sourceURL, category string) (gen.IntelSource, error)
}

// AuthHandlers holds dependencies for auth HTTP handlers.
type AuthHandlers struct { //nolint:revive // stuttering type name preserved for API compatibility
	DB                  Querier
	Queries             *gen.Queries
	Database            *db.DB // Raw DB access for operations not yet in sqlc (e.g., backup code update)
	KeyRotation         *KeyRotation
	BruteForce          BruteForceStore
	Policy              PasswordPolicy
	AsynqClient         *asynq.Client
	VKClient            *valkey.Client        // Valkey client for per-email rate limiting
	Settings            SettingsReader        // Hot-reloadable system settings for auth token TTLs
	SiteName            string                // Brand name for email templates (e.g. "JapaConnect")
	SiteURL             string                // Public site base URL (JC_SITE_URL) for email links — never derived from request Host
	SupportURL          string                // Support page URL for email footers
	CookieDomain        string                // Cookie domain for cross-subdomain sharing (e.g. ".example.com"), empty = exact host
	SecureCookies       bool                  // When true, session/CSRF cookies are marked Secure (production / TLS)
	ReferralCapturer    ReferralCapturer      // Optional — wired by server after referral.Service is constructed (§60)
	DefaultFolderSetter DefaultFolderProvider // Optional — wired by server after bookmarkfolders.Service is constructed (§50)
	TierManager         TierManager           // Optional — wired by server after tiers.Service is constructed (§unified-tiers)
	SourceManager       SourceManager         // Optional — wired by server for v2 source directory auto-creation
}

// SetReferralCapturer injects the referral capture dependency (§60, ADR-078).
func (h *AuthHandlers) SetReferralCapturer(rc ReferralCapturer) { h.ReferralCapturer = rc }

// SetDefaultFolderProvider injects the default bookmark folder seeder.
func (h *AuthHandlers) SetDefaultFolderProvider(p DefaultFolderProvider) { h.DefaultFolderSetter = p }

// SetTierManager injects the tier manager used to assign the regular tier on
// new registrations.
func (h *AuthHandlers) SetTierManager(m TierManager) { h.TierManager = m }

// SetSourceManager injects the canonical source registry for auto-creating
// user publisher profiles in the v2 source directory.
func (h *AuthHandlers) SetSourceManager(m SourceManager) { h.SourceManager = m }

func (h *AuthHandlers) maybeCreateUserSource(ctx context.Context, userID pgtype.UUID, username string) {
	if h.SourceManager == nil {
		return
	}

	if h.Settings != nil {
		enabled, err := h.Settings.GetSettingBool(ctx, "sources.auto_create_for_users")
		if err != nil || !enabled {
			return
		}
	}

	sourceURL := ""
	if h.SiteURL != "" {
		sourceURL = fmt.Sprintf("%s/u/%s", strings.TrimRight(h.SiteURL, "/"), username)
	}

	source, err := h.SourceManager.CreateSourceFromEntity(ctx, userID, userID, "user", username, username, sourceURL, "news")
	if err != nil {
		slog.Warn("register: failed to create canonical user source", "user_id", userID.String(), "error", err)
		return
	}

	if err := h.Queries.UpdateUserSourceID(ctx, gen.UpdateUserSourceIDParams{
		SourceID: source.ID,
		ID:       userID,
	}); err != nil {
		slog.Warn("register: failed to link user source_id", "user_id", userID.String(), "source_id", source.ID.String(), "error", err)
	}
}

// baseURL returns the public-facing base URL for email links (password reset,
// email verification). It prefers the configured SiteURL (JC_SITE_URL), which
// is a server-side value that cannot be influenced by the client. When SiteURL
// is not configured, it falls back to a trusted-proxy-gated derivation from
// X-Forwarded-Host / X-Forwarded-Proto via clientip.ForwardedBaseURL.
// // This prevents host-header / password-reset link poisoning (CWE-601): the
// previous implementation derived the base URL from r.Host (the client-
// controllable Host header) and an unvalidated X-Forwarded-Proto, allowing an
// attacker to send Host: evil.com and capture the reset token when the victim
// clicked the link in the email.
// // The r.Host fallback is used ONLY when the immediate peer is a trusted
// proxy. r.Host is the client-controllable Host header; behind a trusted
// proxy it is the vhost the proxy forwarded, which is as trustworthy as the
// X-Forwarded-Host value ForwardedBaseURL already accepts from trusted peers.
// When the peer is untrusted (a direct client connection) and no SiteURL is
// configured, there is no safe URL to embed in a security-sensitive link —
// baseURL returns "" so callers can refuse to send the email rather than emit
// a link pointing at an attacker-controlled host.
func (h *AuthHandlers) baseURL(r *http.Request) string {
	if h.SiteURL != "" {
		return h.SiteURL
	}

	if !clientip.IsTrustedPeer(r) {
		return ""
	}

	return clientip.ForwardedBaseURL(r, clientip.ForwardedScheme(r)+"://"+r.Host)
}

// Querier is the interface for database operations needed by auth handlers.
type Querier interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error

	// Password reset
	GetValidResetToken(ctx context.Context, tokenHash string) (*ResetTokenRecord, error)
	MarkResetTokenUsed(ctx context.Context, tokenHash string) error
	UpdateUserPassword(ctx context.Context, userID pgtype.UUID, passwordHash string) error

	// Email verification
	GetValidVerificationToken(ctx context.Context, tokenHash string) (*VerificationTokenRecord, error)
	MarkEmailVerified(ctx context.Context, userID pgtype.UUID) error
	ConsumeVerificationToken(ctx context.Context, tokenHash string) error

	// Password history
	GetRecentPasswordHashes(ctx context.Context, userID pgtype.UUID, limit int) ([]string, error)

	// Group membership
	IsUserInGroup(ctx context.Context, arg gen.IsUserInGroupParams) (bool, error)
}

// User represents a user record from the database.
type User struct {
	ID pgtype.UUID

	Email           string
	PasswordHash    pgtype.Text
	Username        string
	ProfileType     string
	Status          string
	EmailVerifiedAt pgtype.Timestamptz
	IsBot           bool // ADR-068: true for AI persona backing accounts — blocked from login
}

// RefreshToken represents a refresh token record.
type RefreshToken struct {
	ID     pgtype.UUID
	UserID pgtype.UUID

	TokenHash string
	ExpiresAt time.Time
	RevokedAt pgtype.Timestamptz
}

// Register handles POST /api/v1/auth/register.
// Creates a new user with email, username, and password.
func (h *AuthHandlers) Register(w http.ResponseWriter, r *http.Request) {
	// Gate registration on the general.registration_open system setting.
	// Default is true (fail open) — only block when explicitly set to false.
	// Ref: japaconnect-engineering/conventions/system-settings-catalog.md
	if h.Settings != nil {
		if open, err := h.Settings.GetSettingBool(r.Context(), "general.registration_open"); err == nil && !open {
			errpkg.WriteError(w, http.StatusForbidden, "registration_closed",
				"Registration is currently closed. Please check back later.")
			return
		}
	}

	var req struct {
		Email    string `json:"email"`
		Username string `json:"username"`
		Password string `json:"password"`
		RefCode  string `json:"ref_code"` // §60 — referral code captured at signup (?ref=CODE)
		Website  string `json:"website"`  // §41 Layer 2 — honeypot field
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	// §41 Layer 2: honeypot field. Real users never fill the hidden
	// `website` field; bots auto-filling all form fields will. Reject
	// silently with a generic 400 so the bot can't distinguish the
	// honeypot from a real validation error.
	if strings.TrimSpace(req.Website) != "" {
		slog.Info("registration blocked: honeypot triggered")
		errpkg.WriteError(w, http.StatusBadRequest, "honeypot_triggered",
			"Registration failed. Please try again.")

		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)

	// §60 Referral capture — the ref code may arrive in the JSON body
	// (ref_code) or as the ?ref= query param (e.g. when the frontend forwards
	// it from the URL). Fall back to the query param so both paths work.
	if req.RefCode == "" {
		req.RefCode = strings.TrimSpace(r.URL.Query().Get("ref"))
	}

	if req.Email == "" || req.Username == "" || req.Password == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Email, username, and password are required")
		return
	}

	// §50 username contract — enforced server-side on every user-supplied
	// username write (3–30 chars, [a-zA-Z0-9_]).
	if !ValidateUsername(req.Username) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_username",
			"Username must be 3-30 characters of letters, numbers, and underscores")
		return
	}

	if err := ValidatePassword(req.Password, h.Policy); err != nil {
		// Check if this is a breach error — different error code for monitoring.
		var breached *BreachedError
		if errors.As(err, &breached) {
			slog.Warn("registration blocked: breached password", "breach_count", breached.Count)
			errpkg.WriteError(w, http.StatusBadRequest, "password_breached",
				"This password has appeared in a data breach and cannot be used. Please choose a different password.")

			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "weak_password", err.Error())

		return
	}

	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "hash_error", "Failed to secure password")
		return
	}

	userID, err := h.Queries.CreateUser(r.Context(), gen.CreateUserParams{
		Email:        req.Email,
		Username:     req.Username,
		PasswordHash: &passwordHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError

		isDuplicate := errors.Is(err, errpkg.ErrDuplicate) || (errors.As(err, &pgErr) && pgErr.Code == "23505")
		if isDuplicate {
			// The users table has unique constraints on ( email)
			// and ( username). Distinguish them when possible —
			// username_exists is acceptable as an oracle because
			// handles are public identifiers on this platform
			// (/u/{username} profiles, users/{username}/feed.xml).
			// email_exists deliberately says "Email or username"
			// so the private identifier stays fuzzy.
			if pgErr != nil && strings.Contains(pgErr.ConstraintName, "username") {
				errpkg.WriteError(w, http.StatusConflict, "username_exists", "Username already in use")
				return
			}

			errpkg.WriteError(w, http.StatusConflict, "email_exists", "Email or username already in use")

			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "create_failed", "Failed to create user")

		return
	}

	// §unified-tiers: new users start on the regular tier. This adds them to
	// the base members group and syncs users.profile_type.
	if h.TierManager != nil {
		if err := h.TierManager.SetUserTier(r.Context(), userID, "regular"); err != nil {
			slog.Error("register: failed to set regular tier", "error", err, "user_id", userID.String())
			errpkg.WriteError(w, http.StatusInternalServerError, "tier_assignment_failed", "Failed to initialize user tier")

			return
		}
	}

	// Record the initial password in history so it cannot be reused later.
	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		UserID:       userID,
		PasswordHash: passwordHash,
	}); err != nil {
		slog.Warn("register: failed to record password history", "error", err)
	}

	// §60 Referral capture — create a pending referral row linking the
	// referrer to the new user. Non-fatal: a failure logs a warning but does
	// not block registration (the referral is simply lost). Self-referral and
	// duplicate-referee are guarded inside CaptureSignupReferral.
	if req.RefCode != "" && h.ReferralCapturer != nil {
		if err := h.ReferralCapturer.CaptureSignupReferral(r.Context(), userID, req.RefCode); err != nil {
			slog.Warn("register: failed to capture referral", "ref_code", req.RefCode, "error", err)
		}
	}

	// §50 Pre-seed default bookmark folders. Non-fatal: a failure logs a
	// warning but does not block registration.
	if h.DefaultFolderSetter != nil {
		if err := h.DefaultFolderSetter.CreateDefaultFoldersForUser(r.Context(), userID); err != nil {
			slog.Warn("register: failed to create default bookmark folders", "error", err)
		}
	}

	// v2 source directory: auto-create a canonical source for verified publisher
	// users when the platform setting is enabled. Non-fatal.
	h.maybeCreateUserSource(r.Context(), userID, req.Username)

	// Send welcome email (fire-and-forget, Golden Rule 8).
	if h.AsynqClient != nil {
		welcomePayload, _ := json.Marshal(map[string]interface{}{
			"to":            req.Email,
			"template_slug": "welcome",

			"template_data": map[string]string{
				"user_name": req.Username,
			},
			"template_site_name": h.SiteName,
		})

		// TaskOpts restores the registered email:send defaults (Unique dedup,
		// 30s timeout, default-tier queue); the trailing MaxRetry keeps the
		// deliberately lower retry bound for welcome email (options apply
		// in order, so it wins over the registered 10).
		task := asynq.NewTask(worker.TypeEmailSend, welcomePayload)
		opts := append(worker.TaskOpts(worker.TypeEmailSend), asynq.MaxRetry(3))
		if _, err := h.AsynqClient.Enqueue(task, opts...); err != nil {
			slog.Warn("register: failed to enqueue welcome email", "error", err)
		}
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := HashRefreshToken(refreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
		DeviceName: sessionDeviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	data := map[string]interface{}{
		"access_token": accessToken,
		"user": map[string]interface{}{
			"id":       userID.String(),
			"email":    req.Email,
			"username": req.Username,
		},
	}
	writeSessionResponse(w, r, h.CookieDomain, h.SecureCookies, refreshToken, refreshTTL, data, true)
}

// Login handles POST /api/v1/auth/login.
func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Website  string `json:"website"` // §41 Layer 2 — honeypot field
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	// §41 Layer 2: honeypot field. Reject silently if filled.
	if strings.TrimSpace(req.Website) != "" {
		slog.Info("login blocked: honeypot triggered")
		errpkg.WriteError(w, http.StatusBadRequest, "honeypot_triggered",
			"Login failed. Please try again.")

		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Email and password are required")
		return
	}

	// Get client IP from request
	ip := clientIP(r)
	bruteKey := LoginIdentifier(req.Email, ip)

	// Check brute-force lockout — read max attempts from system settings.
	// Fail-CLOSED: a store error denies the attempt (429 + Retry-After),
	// so password guessing cannot proceed while the counter backend is down.
	maxAttempts := DefaultMaxLoginAttempts
	if h.Settings != nil {
		maxAttempts = MaxLoginAttempts(r.Context(), h.Settings)
	}

	allowed, retryAfter := CheckLoginAttempt(r.Context(), h.BruteForce, bruteKey, maxAttempts)
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())))
		errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limited", "Too many failed login attempts. Please try again later.")

		return
	}

	user, err := h.DB.GetUserByEmail(r.Context(), req.Email)
	if err != nil || user == nil {
		// Run a bcrypt compare against a dummy hash so a nonexistent email
		// takes the same wall-clock time as a real user with a wrong
		// password — otherwise the early return leaks account existence
		// via response timing (CWE-208).
		_ = VerifyPassword(dummyPasswordHash, req.Password)

		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		h.logFailedLoginAudit(r.Context(), req.Email, ip)

		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	// ADR-068 §1: AI persona backing accounts (is_bot = true) cannot log in
	// via normal auth flows. They act only through the orchestration engine
	// or the Content Studio as their backing account. This is an explicit
	// security gate — do not rely on the null-password_hash incidental block.
	if user.IsBot {
		slog.Warn("login blocked: is_bot account attempted login", "user_id", user.ID, "email", req.Email)
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	if !user.PasswordHash.Valid {
		// OAuth-only accounts have no password hash — same dummy-compare
		// timing guard as the unknown-email path above.
		_ = VerifyPassword(dummyPasswordHash, req.Password)

		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		h.logFailedLoginAudit(r.Context(), req.Email, ip)

		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	if err := VerifyPassword(user.PasswordHash.String, req.Password); err != nil {
		if err := RecordFailedLogin(r.Context(), h.BruteForce, bruteKey); err != nil {
			slog.Warn("record failed login", "error", err)
		}

		h.logFailedLoginAudit(r.Context(), req.Email, ip)

		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "Invalid email or password")

		return
	}

	// Banned/suspended/restricted users may still authenticate — §22 requires
	// the appeal channel to stay reachable; BanEnforcement confines their
	// session to the appeal/ticket surface. Only 'deleted' (and any unknown
	// status) is a hard stop.
	if !loginAllowedStatus(user.Status) {
		errpkg.WriteError(w, http.StatusForbidden, "account_inactive", "Account is not active")
		return
	}

	// Check email verification requirement from system_settings.
	// Super admins bypass email verification — they are platform operators.
	if EmailVerifyRequired(r.Context(), h.Settings) && !user.EmailVerifiedAt.Valid {
		isSuperAdmin, err := h.DB.IsUserInGroup(r.Context(), gen.IsUserInGroupParams{
			UserID:    user.ID,
			GroupSlug: "super_admin",
		})
		if err != nil {
			slog.Warn("check super admin group", "error", err)
		}

		if !isSuperAdmin {
			errpkg.WriteError(w, http.StatusForbidden, "email_not_verified", "Email verification is required before login")
			return
		}
	}

	// Reset brute-force counter on successful login
	if err := ResetLoginAttempts(r.Context(), h.BruteForce, bruteKey); err != nil {
		slog.Warn("reset login attempts", "error", err)
	}

	// Update last_login_at so analytics active-user counts work (§21).
	if err := h.Queries.UpdateLastLoginAt(r.Context(),

		user.ID,
	); err != nil {
		slog.Warn("update last_login_at", "error", err)
	}

	// Check if 2FA is enabled — if so, require TOTP challenge before
	// issuing a full session (AUDIT-BE-001).
	hasTOTP, err := h.Queries.CheckTOTPEnabled(r.Context(),

		user.ID,
	)
	if err != nil {
		slog.Warn("check TOTP enabled", "error", err)

		hasTOTP = false
	}

	if hasTOTP {
		challengeToken, err := h.KeyRotation.MintTOTPChallengeToken(user.ID)
		if err != nil {
			errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create challenge token")
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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

	accessToken, err := h.KeyRotation.MintAccessToken(user.ID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := HashRefreshToken(refreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		UserID:     user.ID,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
		DeviceName: sessionDeviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	data := map[string]interface{}{
		"access_token": accessToken,
		"user": map[string]interface{}{
			"id":       user.ID.String(),
			"email":    user.Email,
			"username": user.Username,
		},
	}
	writeSessionResponse(w, r, h.CookieDomain, h.SecureCookies, refreshToken, refreshTTL, data, true)
}

// refreshReuseLeeway is the grace window applied to refresh-token reuse
// detection. A revoked token re-presented within this window is treated as
// a cookie-rotation race (parallel page loads, sibling apps sharing the
// jc_refresh cookie) rather than theft; outside it the whole session family
// is revoked. Ref: §02 (Refresh-token rotation reuse detection).
const refreshReuseLeeway = 15 * time.Second

// Refresh handles POST /api/v1/auth/refresh.
// Exchanges a valid refresh token cookie for a new access token and a
// rotated refresh token. The old refresh token is revoked (one-time-use),
// which detects refresh-token theft: if a stolen token is used, the
// legitimate user's next refresh presents the now-revoked token and the
// server rejects it.
// // The user's status and email-verification state are re-checked on every
// refresh so a suspended/banned user cannot keep minting access tokens
// until their refresh token expires. Ref: §02 (Session invalidation on
// ban, Email verify required).
// // Mobile clients (X-Device-Id header) send the refresh token in the JSON
// body instead of the HttpOnly cookie used by the web SPA.
func (h *AuthHandlers) Refresh(w http.ResponseWriter, r *http.Request) {
	var refreshToken string

	var stored *RefreshToken

	if isMobileClient(r) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
			errpkg.WriteError(w, http.StatusUnauthorized, "no_refresh_token", "refresh_token is required")
			return
		}

		refreshToken = req.RefreshToken

		row, err := h.DB.GetRefreshToken(r.Context(), HashRefreshToken(refreshToken))
		if err != nil || row == nil {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token is invalid or expired")
			return
		}

		stored = row
		if stored.RevokedAt.Valid {
			if time.Since(stored.RevokedAt.Time) >= refreshReuseLeeway {
				_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(),
					stored.UserID,
				)

				errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has been revoked")

				return
			}
		}

		if time.Now().After(stored.ExpiresAt) {
			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has expired")
			return
		}
	} else {
		// Multiple jc_refresh cookies can coexist in the same jar: the
		// backend sets Path=/api/v1 for direct-SPA surfaces while SSR
		// proxies (e.g. the main app) re-set it at Path=/ for server-side
		// loads. Both are sent on /api/v1/* requests and cookie ordering
		// can put a rotated-out sibling-app value first, so resolve the
		// token across ALL presented values instead of trusting the first.
		// Ref: §02 (jc_refresh intentionally shared for SSO across apps).
		var cookies []*http.Cookie

		for _, c := range r.Cookies() {
			if c.Name == "jc_refresh" && c.Value != "" {
				cookies = append(cookies, c)
			}
		}

		if len(cookies) == 0 {
			errpkg.WriteError(w, http.StatusUnauthorized, "no_refresh_token", "Refresh token cookie is required")
			return
		}

		var theftCandidate *RefreshToken

		var sawExpired bool

		for _, c := range cookies {
			row, err := h.DB.GetRefreshToken(r.Context(), HashRefreshToken(c.Value))
			if err != nil || row == nil {
				continue
			}

			if row.RevokedAt.Valid {
				// ANY revoked-beyond-leeway reuse revokes the whole family,
				// even when a live sibling cookie is also present. A stale
				// duplicate-path cookie proves an old token left the
				// rotation window — possibly replayed by an attacker — so
				// it must not be masked by a sibling that still validates.
				// Ref: §02 (rotation reuse detection).
				if time.Since(row.RevokedAt.Time) >= refreshReuseLeeway {
					slog.Warn("refresh: revoked token reuse detected, revoking session family",
						"user_id", row.UserID.String())

					_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(),
						row.UserID,
					)

					errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has been revoked")

					return
				}
				// Within leeway: rotation race — usable, stays revoked.
				if stored == nil {
					refreshToken, stored = c.Value, row
				}

				continue
			}

			if time.Now().After(row.ExpiresAt) {
				sawExpired = true
				continue
			}
			// Fully valid token wins immediately. Note: any
			// revoked-beyond-leeway cookie already returned above, so
			// reaching here means no theft signal was present among the
			// presented values.
			refreshToken, stored = c.Value, row

			break
		}

		if stored == nil {
			if theftCandidate != nil {
				// Unreachable in practice: revoked-beyond-leeway cookies
				// return immediately above. Retained as defense-in-depth in
				// case the loop above is ever restructured.
				slog.Warn("refresh: revoked token reuse detected, revoking session family",
					"user_id", theftCandidate.UserID.String())

				_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(),
					theftCandidate.UserID,
				)

				errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has been revoked")

				return
			}

			if sawExpired {
				errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token has expired")
				return
			}

			errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token is invalid or expired")

			return
		}
	}

	tokenHash := HashRefreshToken(refreshToken)

	// Re-check the user's status. A suspended/banned user whose refresh
	// token was issued before the ban must not be able to keep refreshing
	// access tokens until the refresh token expires. Ref: §02.
	userRow, err := h.Queries.GetUserByID(r.Context(),

		stored.UserID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_refresh_token", "Refresh token is invalid or expired")
		return
	}

	// Same restricted-session rule as login (§22): banned/suspended/
	// restricted users keep their session for the appeal channel; only
	// terminal statuses get a full revocation.
	if !loginAllowedStatus(userRow.Status) {
		// Revoke all sessions for the inactive user.
		_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(),
			stored.UserID,
		)

		errpkg.WriteError(w, http.StatusUnauthorized, "account_inactive", "Account is no longer active")

		return
	}

	// Re-check email verification when the platform requires it. A user
	// whose email was un-verified (or whose verification was revoked)
	// cannot keep refreshing. Ref: §02 (auth.email.verify_required).
	if EmailVerifyRequired(r.Context(), h.Settings) {
		verifiedAt, err := h.Queries.GetUserEmailVerifiedStatus(r.Context(),

			stored.UserID,
		)
		if err != nil || !verifiedAt.Valid {
			_ = h.Queries.RevokeAllUserRefreshTokens(r.Context(),
				stored.UserID,
			)

			errpkg.WriteError(w, http.StatusUnauthorized, "email_not_verified", "Email verification is required")

			return
		}
	}

	accessToken, err := h.KeyRotation.MintAccessToken(stored.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	// Rotate the refresh token: issue a new one and revoke the old one.
	// This makes refresh tokens one-time-use, so a stolen refresh token
	// is detected on the next legitimate refresh. Ref: §02 (Token model).
	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	newTokenHash := HashRefreshToken(newRefreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)
	newExpiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		UserID:     stored.UserID,
		TokenHash:  newTokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: newExpiresAt, Valid: true},
		DeviceName: sessionDeviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	// Revoke the old refresh token now that the new one is persisted.
	// Best-effort: a failure here leaves two valid refresh tokens, which
	// is a minor security degradation, not a correctness issue.
	if err := h.Queries.RevokeRefreshTokenByHash(r.Context(), tokenHash); err != nil {
		slog.Warn("refresh: failed to revoke old refresh token", "user_id", stored.UserID.String(), "error", err)
	}

	data := map[string]interface{}{"access_token": accessToken}
	writeSessionResponse(w, r, h.CookieDomain, h.SecureCookies, newRefreshToken, refreshTTL, data, false)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ChallengeToken == "" || req.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "challenge_token and code are required")
		return
	}

	claims, err := h.KeyRotation.ValidateChallengeToken(req.ChallengeToken)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid or expired challenge token")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	var ()

	if err := (error)(nil); err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	// Rate-limit TOTP challenge attempts per user to prevent brute-force
	// guessing of the 6-digit code. 5 attempts per 5 minutes — cataloged
	// as jc:ratelimit:totp_challenge:{user_id}. Fail-open on Valkey
	// errors (log and allow), matching ConsumeBackupCode.
	if h.VKClient != nil {
		rlKey := valkey.MustKey("jc:ratelimit:totp_challenge:%s", userID.String())

		ok, err := rateLimitCheck(r.Context(), h.VKClient.Redis(), rlKey, 5, 5*time.Minute)
		if err != nil {
			slog.Error("totp challenge: rate limit check failed, allowing request", "error", err)
		}

		if !ok {
			errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
				"Too many verification attempts. Please try again later.")
			return
		}
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "Verification code is required")
		return
	}

	totpRecord, err := h.Queries.GetTOTPCredentials(r.Context(),

		userID,
	)
	if err != nil || !totpRecord.EnabledAt.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "totp_error", "2FA not set up")
		return
	}

	plaintext, err := decryptSecret(totpRecord.SecretEncrypted)
	if err != nil {
		slog.Error("decrypt TOTP secret", "user_id", userID)
		errpkg.CaptureException(fmt.Errorf("decrypt TOTP secret for user %s: %w", userID, err))
		errpkg.WriteError(w, http.StatusInternalServerError, "totp_error", "Failed to verify 2FA")

		return
	}

	if !VerifyTOTP(plaintext, req.Code) {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_totp", "Invalid verification code")
		return
	}

	// Replay guard: atomically claim the code before issuing a session so a
	// code observed in transit cannot be reused inside its validity window.
	if h.VKClient != nil && !claimTOTPCode(r.Context(), h.VKClient.Redis(), userID, req.Code) {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_totp", "Invalid verification code")
		return
	}

	// Single-use the challenge token: revoke its JTI so a captured challenge
	// cannot mint additional sessions for the remaining ~5 minutes of its TTL.
	if h.VKClient != nil {
		if err := RevokeToken(r.Context(), claims.JTI, claims.ExpiresAt.Time, h.VKClient.Redis()); err != nil {
			slog.Warn("totp challenge: failed to revoke challenge token", "error", err)
		}
	}

	// Update last_login_at so analytics active-user counts work (§21).
	if err := h.Queries.UpdateLastLoginAt(r.Context(),

		userID,
	); err != nil {
		slog.Warn("update last_login_at", "error", err)
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := HashRefreshToken(refreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
		DeviceName: sessionDeviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	data := map[string]interface{}{"access_token": accessToken}
	writeSessionResponse(w, r, h.CookieDomain, h.SecureCookies, refreshToken, refreshTTL, data, true)
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	if req.ChallengeToken == "" || req.BackupCode == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "challenge_token and backup_code are required")
		return
	}

	claims, err := h.KeyRotation.ValidateChallengeToken(req.ChallengeToken)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid or expired challenge token")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(claims.UserID); err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	var ()

	if err := (error)(nil); err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_challenge", "Invalid challenge token")
		return
	}

	// Rate-limit backup-code attempts per user to prevent brute-force
	// guessing. Backup codes are 8-char alphanumeric (limited entropy), so
	// unthrottled guessing risks account compromise (Bug Hunter §5.2).
	// 5 attempts per 5 minutes — cataloged as jc:ratelimit:totp_backup:{user_id}.
	// Ref: japaconnect-engineering/sections/02-authentication-sessions.md
	if h.VKClient != nil {
		rlKey := valkey.MustKey("jc:ratelimit:totp_backup:%s", userID.String())

		ok, err := rateLimitCheck(r.Context(), h.VKClient.Redis(), rlKey, 5, 5*time.Minute)
		if err != nil {
			slog.Error("totp backup: rate limit check failed, allowing request", "error", err)
		}

		if !ok {
			errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
				"Too many backup code attempts. Please try again later.")
			return
		}
	}

	req.BackupCode = sanitizeCode(req.BackupCode)

	backupRecord, err := h.Queries.GetTOTPCredentials(r.Context(),

		userID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "no_backup_codes", "No backup codes found")
		return
	}

	matched, remaining, err := consumeBackupCode(r.Context(), h.Database, &backupRecord, req.BackupCode, userID)
	if err != nil {
		slog.Error("consume backup code", "error", err)
		errpkg.CaptureException(fmt.Errorf("consume backup code for user %s: %w", userID, err))
		errpkg.WriteError(w, http.StatusInternalServerError, "backup_error", "Failed to verify backup code")

		return
	}

	if !matched {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_backup", "Invalid backup code")
		return
	}

	// Single-use the challenge token on backup-code success too — same
	// replay reasoning as VerifyLoginChallenge.
	if h.VKClient != nil {
		if err := RevokeToken(r.Context(), claims.JTI, claims.ExpiresAt.Time, h.VKClient.Redis()); err != nil {
			slog.Warn("totp backup: failed to revoke challenge token", "error", err)
		}
	}

	// Update last_login_at so analytics active-user counts work (§21).
	if err := h.Queries.UpdateLastLoginAt(r.Context(),

		userID,
	); err != nil {
		slog.Warn("update last_login_at", "error", err)
	}

	accessToken, err := h.KeyRotation.MintAccessToken(userID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	tokenHash := HashRefreshToken(refreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)
	expiresAt := time.Now().Add(refreshTTL)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: expiresAt, Valid: true},
		DeviceName: sessionDeviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")
		return
	}

	data := map[string]interface{}{
		"access_token":    accessToken,
		"codes_remaining": remaining,
	}
	writeSessionResponse(w, r, h.CookieDomain, h.SecureCookies, refreshToken, refreshTTL, data, true)
}

// Logout handles POST /api/v1/auth/logout.
// Revokes the refresh token, revokes the current access token's JTI,
// and clears the cookie.
// // Mobile clients send the refresh token in the JSON body because they do
// not store the HttpOnly cookie used by the web SPA.
func (h *AuthHandlers) Logout(w http.ResponseWriter, r *http.Request) {
	if isMobileClient(r) {
		var req struct {
			RefreshToken string `json:"refresh_token"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err == nil && req.RefreshToken != "" {
			tokenHash := HashRefreshToken(req.RefreshToken)
			if err := h.DB.RevokeRefreshToken(r.Context(), tokenHash); err != nil {
				slog.Error("revoke refresh token", "error", err)
			}
		}
	} else {
		// Revoke ALL presented jc_refresh values: duplicate-path cookies
		// (Path=/ and Path=/api/v1) can coexist in the jar, and leaving
		// any one live would keep the session usable after logout.
		for _, c := range r.Cookies() {
			if c.Name != "jc_refresh" || c.Value == "" {
				continue
			}

			tokenHash := HashRefreshToken(c.Value)
			if err := h.DB.RevokeRefreshToken(r.Context(), tokenHash); err != nil {
				slog.Error("revoke refresh token", "error", err)
			}
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

	// Clear both cookie paths: the backend sets Path=/api/v1 while SSR
	// proxies re-set Path=/, so both must be expired or the browser keeps
	// sending the surviving one.
	for _, path := range []string{"/api/v1", "/"} {
		http.SetCookie(w, &http.Cookie{
			Name:     "jc_refresh",
			Value:    "",
			Path:     path,
			Domain:   h.CookieDomain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.SecureCookies,
			SameSite: http.SameSiteStrictMode,
		}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]string{"status": "logged_out"},
	})
}

// clientIP extracts the client IP from the request. Forwarding headers are
// only honoured when the peer is a trusted proxy — the brute-force lockout is
// keyed on this value, so a spoofable IP means unlimited password guesses.
func clientIP(r *http.Request) string {
	return clientip.FromRequest(r)
}

// logFailedLoginAudit writes an audit_logs entry for a failed login attempt.
// This is consumed by the analytics security scanner (scanFailedLoginSpike)
// to detect brute-force attacks. Ref: sections/40-platform-analytics-reports.md.
func (h *AuthHandlers) logFailedLoginAudit(ctx context.Context, email, ip string) {
	if h.Database == nil {
		return
	}

	meta, _ := json.Marshal(map[string]string{"email": email, "ip": ip})

	_, err := h.Database.Writer.Exec(ctx, `
		INSERT INTO audit_logs (action_type, target_type, metadata, ip_address) VALUES ('auth.login_failed', 'user', $1::jsonb, $2::inet)
	`, meta, ip)
	if err != nil {
		slog.Warn("failed to write login_failed audit log", "error", err)
	}
}

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
		Email   string `json:"email"`
		Website string `json:"website"` // §41 Layer 2 — honeypot field
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	// §41 Layer 2: honeypot field. Reject silently if filled.
	if strings.TrimSpace(req.Website) != "" {
		slog.Info("forgot-password blocked: honeypot triggered")
		errpkg.WriteError(w, http.StatusBadRequest, "honeypot_triggered",
			"Request failed. Please try again.")

		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Email is required")
		return
	}

	// Per-email rate limit: 3 requests per 10 minutes. Prevents a single
	// email address from being used to flood the reset-email queue.
	if h.VKClient != nil {
		emailKey := valkey.MustKey("jc:ratelimit:forgot_password:%s", req.Email)

		ok, err := rateLimitCheck(r.Context(), h.VKClient.Redis(), emailKey, 3, 10*time.Minute)
		if err != nil {
			slog.Error("forgot password: rate limit check failed, allowing request", "error", err)
		}

		if !ok {
			errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many password reset requests. Please try again later.")
			return
		}
	}

	// Always return 200 to prevent email enumeration — whether the user exists or not.
	defer func() {
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "If an account with that email exists, a reset link has been sent.",
			},
		})
	}()

	user, err := h.DB.GetUserByEmail(r.Context(), req.Email)
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
		errpkg.CaptureException(fmt.Errorf("forgot-password: generate token: %w", err))

		return
	}

	expiresAt := time.Now().Add(ResetTokenTTL)
	if err := h.Queries.CreatePasswordResetToken(r.Context(), gen.CreatePasswordResetTokenParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		slog.Error("forgot-password: failed to store token", "error", err)
		errpkg.CaptureException(fmt.Errorf("forgot-password: store token: %w", err))

		return
	}

	// Build the base URL from the server-configured SiteURL (CWE-601 fix).
	// See baseURL doc for the host-header injection rationale.
	baseURL := h.baseURL(r)

	// No safe base URL could be derived: JC_SITE_URL is unset and the
	// immediate peer is not a trusted proxy, so r.Host / X-Forwarded-Host
	// are client-controlled and must not be embedded in a password-reset
	// link (CWE-601). Refuse to send the email rather than emit a link
	// pointing at an attacker-controlled host. The deferred 200 response
	// still fires, so this does not leak that the account exists.
	if baseURL == "" {
		slog.Error("forgot-password: no safe base URL configured (set JC_SITE_URL or run behind a trusted proxy)")
		errpkg.CaptureException(errors.New("forgot-password: no safe base URL configured (set JC_SITE_URL or run behind a trusted proxy)"))

		return
	}

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
//
// The response shape is intentionally unchanged ({data:{valid:bool}},
// always 200 on well-formed requests) so token validity is not
// distinguishable by status code. Tight per-IP rate limiting against
// token-guessing (oracle) is enforced at route level in server.go
// (RateLimit "tight"); the handler only logs validation failures.
func (h *AuthHandlers) HandleResetPasswordPage(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_token", "Token query parameter is required")
		return
	}

	tokenHash := HashRefreshToken(token)

	rec, err := h.DB.GetValidResetToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		slog.Warn("reset-password page: invalid token presented")
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{"valid": false},
		})

		return
	}

	// Check expiry and whether it's been used.
	if time.Now().After(rec.ExpiresAt) || rec.UsedAt.Valid {
		slog.Warn("reset-password page: expired or used token presented")
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{"valid": false},
		})

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"valid": true},
	})
}

// HandleResetPassword handles POST /api/v1/auth/reset-password.
// Validates the token, updates the password, and invalidates all sessions.
func (h *AuthHandlers) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
		Website     string `json:"website"` // §41 Layer 2 — honeypot field
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	// §41 Layer 2: honeypot field. Reject silently if filled.
	if strings.TrimSpace(req.Website) != "" {
		slog.Info("reset-password blocked: honeypot triggered")
		errpkg.WriteError(w, http.StatusBadRequest, "honeypot_triggered",
			"Request failed. Please try again.")

		return
	}

	if req.Token == "" || req.NewPassword == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Token and new_password are required")
		return
	}

	if err := ValidatePassword(req.NewPassword, h.Policy); err != nil {
		// Check if this is a breach error — different error code for monitoring.
		var breached *BreachedError
		if errors.As(err, &breached) {
			slog.Warn("password reset blocked: breached password", "token_prefix", req.Token[:min(8, len(req.Token))], "breach_count", breached.Count)
			errpkg.WriteError(w, http.StatusBadRequest, "password_breached",
				"This password has appeared in a data breach and cannot be used. Please choose a different password.")

			return
		}

		errpkg.WriteError(w, http.StatusBadRequest, "weak_password", err.Error())

		return
	}

	tokenHash := HashRefreshToken(req.Token)

	rec, err := h.DB.GetValidResetToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired reset token")
		return
	}

	if time.Now().After(rec.ExpiresAt) {
		errpkg.WriteError(w, http.StatusUnauthorized, "expired_token", "Reset token has expired")
		return
	}

	if rec.UsedAt.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "used_token", "Reset token has already been used")
		return
	}

	// Check password history before allowing the reset.
	if err := CheckPasswordHistory(r.Context(), h.DB, rec.UserID, req.NewPassword, h.Policy); err != nil {
		var reused *PasswordReusedError
		if errors.As(err, &reused) {
			slog.Warn("password reset blocked: reused password", "token_prefix", req.Token[:min(8, len(req.Token))])
			errpkg.WriteError(w, http.StatusBadRequest, "password_reused",
				"New password cannot be the same as a recent password. Please choose a different password.")

			return
		}
	}

	// Mark the token as used before updating the password.
	if err := h.DB.MarkResetTokenUsed(r.Context(), tokenHash); err != nil {
		slog.Error("reset-password: failed to mark token used", "error", err)
		errpkg.CaptureException(fmt.Errorf("reset-password: mark token used: %w", err))
		errpkg.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to process reset token")

		return
	}

	passwordHash, err := HashPassword(req.NewPassword)
	if err != nil {
		errpkg.CaptureException(fmt.Errorf("reset-password: hash password: %w", err))
		errpkg.WriteError(w, http.StatusInternalServerError, "hash_error", "Failed to secure password")

		return
	}

	if err := h.DB.UpdateUserPassword(r.Context(), rec.UserID, passwordHash); err != nil {
		slog.Error("reset-password: failed to update password", "error", err)
		errpkg.CaptureException(fmt.Errorf("reset-password: update password: %w", err))
		errpkg.WriteError(w, http.StatusInternalServerError, "update_failed", "Failed to update password")

		return
	}

	// Record the new password in history to prevent reuse.
	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		UserID:       rec.UserID,
		PasswordHash: passwordHash,
	}); err != nil {
		slog.Warn("reset-password: failed to record password history", "error", err)
	}

	// Invalidate all sessions for this user.
	if err := InvalidateAllUserSessions(r.Context(), h.Queries, rec.UserID); err != nil {
		slog.Error("reset-password: failed to invalidate sessions", "user_id", rec.UserID.String(), "error", err)
	}

	// Audit log: record the password reset action.
	// Ref: §02 "Audit log: 'user.password_reset'"
	if h.Database != nil {
		meta, _ := json.Marshal(map[string]string{"user_id": rec.UserID.String()})

		_, err := h.Database.Writer.Exec(r.Context(), `
			INSERT INTO audit_logs (action_type, target_type, metadata) VALUES ('user.password_reset', 'user', $1::jsonb)
		`, meta)
		if err != nil {
			slog.Warn("reset-password: failed to write audit log", "error", err)
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusBadRequest, "missing_token", "Token query parameter is required")
		return
	}

	tokenHash := HashRefreshToken(token)

	rec, err := h.DB.GetValidVerificationToken(r.Context(), tokenHash)
	if err != nil || rec == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "invalid_token", "Invalid or expired verification token")
		return
	}

	if time.Now().After(rec.ExpiresAt) {
		errpkg.WriteError(w, http.StatusUnauthorized, "expired_token", "Verification token has expired")
		return
	}

	if rec.VerifiedAt.Valid {
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "Email has already been verified.",
			},
		})

		return
	}

	// Consume the token BEFORE marking verified so a replayed link finds
	// an already-consumed row (single-use). The guarded UPDATE is a no-op
	// on consumed rows; the pre-check above is what rejects replays.
	if err := h.DB.ConsumeVerificationToken(r.Context(), tokenHash); err != nil {
		slog.Error("verify-email: failed to consume token", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify email")

		return
	}

	if err := h.DB.MarkEmailVerified(r.Context(), rec.UserID); err != nil {
		slog.Error("verify-email: failed to mark verified", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "verify_failed", "Failed to verify email")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "Email is required")
		return
	}

	// Per-email rate limit: 1 request per 5 minutes. Prevents a single
	// email address from being used to flood the verification-email queue.
	if h.VKClient != nil {
		emailKey := valkey.MustKey("jc:ratelimit:resend_verification:%s", req.Email)

		ok, err := rateLimitCheck(r.Context(), h.VKClient.Redis(), emailKey, 1, 5*time.Minute)
		if err != nil {
			slog.Error("resend verification: rate limit check failed, allowing request", "error", err)
		}

		if !ok {
			errpkg.WriteError(w, http.StatusTooManyRequests, "rate_limit_exceeded", "Too many verification email requests. Please try again later.")
			return
		}
	}

	// Always return 200 to prevent email enumeration.
	defer func() {
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]string{
				"message": "If an account with that email exists and is unverified, a new verification link has been sent.",
			},
		})
	}()

	user, err := h.DB.GetUserByEmail(r.Context(), req.Email)
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

	// Invalidate previous unverified tokens before issuing a new one.
	// Ref: §02 "POST /auth/resend-verification → invalidates previous unverified token"
	if err := h.Queries.InvalidateUserEmailVerifications(r.Context(),
		user.ID,
	); err != nil {
		slog.Error("resend-verification: failed to invalidate previous tokens", "error", err)
	}

	expiresAt := time.Now().Add(VerificationTokenTTL)
	if err := h.Queries.CreateEmailVerification(r.Context(), gen.CreateEmailVerificationParams{
		UserID:    user.ID,
		Email:     user.Email,
		TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		slog.Error("resend-verification: failed to store token", "error", err)
		return
	}

	// Build the base URL from the server-configured SiteURL (CWE-601 fix).
	baseURL := h.baseURL(r)

	// No safe base URL could be derived: JC_SITE_URL is unset and the
	// immediate peer is not a trusted proxy, so r.Host / X-Forwarded-Host
	// are client-controlled and must not be embedded in an email-verification
	// link (CWE-601). Refuse to send the email rather than emit a link
	// pointing at an attacker-controlled host. The deferred 200 response
	// still fires, so this does not leak that the account exists.
	if baseURL == "" {
		slog.Error("resend-verification: no safe base URL configured (set JC_SITE_URL or run behind a trusted proxy)")
		errpkg.CaptureException(errors.New("resend-verification: no safe base URL configured (set JC_SITE_URL or run behind a trusted proxy)"))

		return
	}

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

// loginAllowedStatus is the §22 restricted-session rule: users under a
// ban/suspension/operation restriction may authenticate so the appeal
// channel (POST /appeals, /me/appeals, /me/tickets) stays reachable —
// BanEnforcement confines them to exempt paths. 'deleted', 'pending', and
// any unknown status remain hard stops.
func loginAllowedStatus(status string) bool {
	switch status {
	case "active", "banned", "suspended", "restricted":
		return true
	default:
		return false
	}
}
