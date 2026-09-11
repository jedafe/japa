package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (OAuth social login)
// On first OAuth login, create account if no matching email exists; if a matching
// email exists, merge the OAuth identity onto the existing account. The callback
// redirects to the frontend with cookies set — it does not return JSON, because
// the browser lands here after the provider redirect.

// OAuthHandlers holds dependencies for OAuth HTTP handlers.
type OAuthHandlers struct {
	Config              OAuthConfig
	Store               OAuthStore
	Queries             *gen.Queries
	KeyRot              *KeyRotation
	Settings            SettingsReader        // Hot-reloadable system settings for refresh token TTL
	FrontendBaseURL     string                // Public frontend base URL (JC_SITE_URL) for post-OAuth redirects
	CookieDomain        string                // Cookie domain for cross-subdomain sharing (e.g. ".example.com"), empty = exact host
	SecureCookies       bool                  // When true, session cookies are marked Secure (production / TLS)
	DefaultFolderSetter DefaultFolderProvider // Optional — wired by server after bookmarkfolders.Service is constructed (§50)
	TierManager         TierManager           // Optional — wired by server after tiers.Service is constructed (§unified-tiers)
	SourceManager       SourceManager         // Optional — wired by server for v2 source directory auto-creation
}

// SetTierManager injects the tier manager used to assign the regular tier on
// first-time OAuth registrations.
func (h *OAuthHandlers) SetTierManager(m TierManager) { h.TierManager = m }

// SetSourceManager injects the canonical source registry for auto-creating
// user publisher profiles in the v2 source directory.
func (h *OAuthHandlers) SetSourceManager(m SourceManager) { h.SourceManager = m }

func (h *OAuthHandlers) maybeCreateUserSource(ctx context.Context, tenantID, userID pgtype.UUID, username string) {
	if h.SourceManager == nil {
		return
	}

	if h.Settings != nil {
		enabled, err := h.Settings.GetSettingBool(ctx, tenantID, "sources.auto_create_for_users")
		if err != nil || !enabled {
			return
		}
	}

	sourceURL := ""
	if h.FrontendBaseURL != "" {
		sourceURL = fmt.Sprintf("%s/u/%s", strings.TrimRight(h.FrontendBaseURL, "/"), username)
	}

	source, err := h.SourceManager.CreateSourceFromEntity(ctx, tenantID, userID, userID, "user", username, username, sourceURL, "news")
	if err != nil {
		slog.Warn("oauth register: failed to create canonical user source", "user_id", userID.String(), "error", err)
		return
	}

	if err := h.Queries.UpdateUserSourceID(ctx, gen.UpdateUserSourceIDParams{
		SourceID: source.ID,
		ID:       userID,
		TenantID: tenantID,
	}); err != nil {
		slog.Warn("oauth register: failed to link user source_id", "user_id", userID.String(), "source_id", source.ID.String(), "error", err)
	}
}

// oauthRedirectCookie is the short-lived cookie holding the frontend path to
// redirect to after a successful OAuth callback. Stored alongside jc_oauth_state.
const oauthRedirectCookie = "jc_oauth_redirect"

// SetDefaultFolderProvider injects the default bookmark folder seeder.
func (h *OAuthHandlers) SetDefaultFolderProvider(p DefaultFolderProvider) { h.DefaultFolderSetter = p }

// HandleInitiate handles POST /api/v1/auth/oauth/{provider}/initiate.
// Validates the provider, generates a CSRF state token, stores it in a
// short-lived cookie, and returns the auth URL as JSON for the frontend
// to redirect the user to the provider's login page. An optional redirect
// path is stored in a separate cookie so the callback knows where to send
// the user after success.
func (h *OAuthHandlers) HandleInitiate(w http.ResponseWriter, r *http.Request) {
	// Gate social login on the users.allow_social_login system setting
	// (admin-configurable in the Settings → Users page). When disabled, all
	// OAuth providers are rejected at initiation so the setting actually
	// affects the site. Ref: conventions/system-settings-catalog.md
	if h.Settings != nil {
		if t := tenant.FromContext(r.Context()); t != nil {
			if allowed, err := h.Settings.GetSettingBool(r.Context(), t.ID, "users.allow_social_login"); err == nil && !allowed {
				errpkg.WriteError(w, http.StatusForbidden, "social_login_disabled",
					"Social login is disabled for this tenant")
				return
			}
		}
	}

	providerName := chi.URLParam(r, "provider")

	provider := h.Config.NewProvider(providerName)
	if provider == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_provider",
			"OAuth provider is not configured or not supported")
		return
	}

	state, err := GenerateOAuthState()
	if err != nil {
		slog.Error("generate OAuth state", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "state_error", "Failed to initiate OAuth")

		return
	}

	// Store state in a short-lived cookie for CSRF validation on callback.
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_oauth_state",
		Value:    state,
		Path:     "/",
		Domain:   h.CookieDomain,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically

	// Store the requested redirect path so the callback can forward the user
	// to the right frontend page after success. Defaults to "/".
	// The path is validated by safeRedirectPath to prevent open-redirect
	// attacks (CWE-601): an attacker could set redirect=//evil.com and
	// send the victim to an external host after OAuth login.
	redirect := r.URL.Query().Get("redirect")
	if redirect == "" {
		var body struct {
			Redirect string `json:"redirect"`
		}
		// Best-effort decode; the frontend may send redirect as JSON or query.
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.Redirect != "" {
			redirect = body.Redirect
		}
	}

	redirect = safeRedirectPath(redirect)

	http.SetCookie(w, &http.Cookie{
		Name:     oauthRedirectCookie,
		Value:    redirect,
		Path:     "/",
		Domain:   h.CookieDomain,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"auth_url": provider.AuthURL(state)},
	})
}

// HandleCallback handles GET /api/v1/auth/oauth/{provider}/callback.
// Validates the CSRF state, exchanges the code for user info, creates or
// links the account per Section 02, sets session cookies, and redirects
// to the frontend. New users redirect to /onboarding; returning users to
// the stored redirect path (default /). Errors redirect to /login with
// an oauth_error query parameter.
func (h *OAuthHandlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")

	provider := h.Config.NewProvider(providerName)
	if provider == nil {
		h.redirectOAuthError(w, r, "unsupported_provider")
		return
	}

	// Validate CSRF state
	stateCookie, err := r.Cookie("jc_oauth_state")
	if err != nil || stateCookie.Value == "" {
		h.redirectOAuthError(w, r, "missing_state")
		return
	}

	callbackState := r.URL.Query().Get("state")
	if !ValidateOAuthState(stateCookie.Value, callbackState) {
		h.redirectOAuthError(w, r, "invalid_state")
		return
	}

	// Clear the state + redirect cookies (read redirect before clearing).
	// Re-validate the path even though it was validated at initiation —
	// the cookie could have been tampered with between the two requests.
	redirectPath := "/"
	if rc, err := r.Cookie(oauthRedirectCookie); err == nil && rc.Value != "" {
		redirectPath = safeRedirectPath(rc.Value)
	}

	h.clearOAuthCookies(w)

	// Check for provider error
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		slog.Warn("OAuth provider error", "provider", providerName, "error", errParam, "description", desc)
		h.redirectOAuthError(w, r, "oauth_error")

		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		h.redirectOAuthError(w, r, "missing_code")
		return
	}

	// Exchange code for user info
	userInfo, err := provider.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("OAuth exchange failed", "provider", providerName, "error", err)
		h.redirectOAuthError(w, r, "oauth_exchange_failed")

		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		h.redirectOAuthError(w, r, "tenant_unavailable")
		return
	}

	// Try to find existing linked account — returning user.
	existing, err := h.Store.GetOAuthAccount(r.Context(), t.ID, providerName, userInfo.ProviderUserID)
	if err == nil && existing != nil {
		h.issueSessionAndRedirect(w, r, existing.UserID, t.ID, redirectPath, false)
		return
	}

	// Try to find user by email — if found, merge the OAuth identity.
	if userInfo.Email != "" {
		user, err := h.Store.GetUserByEmail(r.Context(), t.ID, userInfo.Email)
		if err == nil && user != nil {
			// ADR-068 §1: AI persona backing accounts cannot log in via any
			// auth flow, including OAuth. Block is_bot accounts explicitly.
			if user.IsBot {
				slog.Warn("oauth login blocked: is_bot account",
					"user_id", user.ID, "provider", providerName)
				h.redirectOAuthError(w, r, "invalid_account")

				return
			}

			if linkErr := h.Store.LinkOAuthAccount(r.Context(), t.ID, user.ID, providerName, userInfo.ProviderUserID); linkErr != nil {
				slog.Error("link OAuth account", "error", linkErr)
			}

			h.issueSessionAndRedirect(w, r, user.ID, t.ID, redirectPath, false)

			return
		}
	}

	// No existing account or email match — create a new OAuth-only account
	// per Section 02 line 36. password_hash is NULL for OAuth-only accounts.
	if userInfo.Email == "" {
		// Without an email we cannot create an account — the provider did not
		// return one. Redirect to login with an explanatory error.
		h.redirectOAuthError(w, r, "missing_email")
		return
	}

	userID, err := h.createOAuthUser(r.Context(), t.ID, userInfo, providerName)
	if err != nil {
		slog.Error("create OAuth user", "provider", providerName, "error", err)
		h.redirectOAuthError(w, r, "create_failed")

		return
	}

	// New user → onboarding wizard, regardless of the stored redirect path.
	h.issueSessionAndRedirect(w, r, userID, t.ID, "/onboarding", true)
}

// createOAuthUser creates a new OAuth-only user (password_hash = NULL) with a
// unique username derived from the email local-part and provider, then links
// the OAuth account. Retries on username collision with a random suffix.
func (h *OAuthHandlers) createOAuthUser(ctx context.Context, tenantID pgtype.UUID, info *OAuthUserInfo, providerName string) (pgtype.UUID, error) {
	base := sanitizeUsernameBase(info.Email, info.Name)
	attempt := base + "_" + providerName

	for i := range 5 {
		candidate := attempt

		if i > 0 {
			suffix, err := randomSuffix(6)
			if err != nil {
				return pgtype.UUID{}, fmt.Errorf("generate username suffix: %w", err)
			}

			candidate = attempt + "_" + suffix
		}

		userID, err := h.Queries.CreateUser(ctx, gen.CreateUserParams{
			TenantID:     tenantID,
			Email:        strings.ToLower(strings.TrimSpace(info.Email)),
			Username:     candidate,
			PasswordHash: nil, // OAuth-only account
		})
		if err != nil {
			if errors.Is(err, errpkg.ErrDuplicate) {
				continue // username or email collision — retry with new suffix
			}

			return pgtype.UUID{}, fmt.Errorf("create user: %w", err)
		}

		// §unified-tiers: new users start on the regular tier.
		if h.TierManager != nil {
			if tierErr := h.TierManager.SetUserTier(ctx, tenantID, userID, "regular"); tierErr != nil {
				slog.Warn("oauth register: failed to set regular tier", "error", tierErr, "user_id", userID.String())
			}
		}

		if linkErr := h.Store.LinkOAuthAccount(ctx, tenantID, userID, providerName, info.ProviderUserID); linkErr != nil {
			return pgtype.UUID{}, fmt.Errorf("link OAuth account: %w", linkErr)
		}

		// §50 Pre-seed default bookmark folders. Non-fatal: logged on failure.
		if h.DefaultFolderSetter != nil {
			if err := h.DefaultFolderSetter.CreateDefaultFoldersForUser(ctx, tenantID, userID); err != nil {
				slog.Warn("oauth register: failed to create default bookmark folders", "error", err)
			}
		}

		// v2 source directory: auto-create a canonical source for publisher users.
		h.maybeCreateUserSource(ctx, tenantID, userID, candidate)

		return userID, nil
	}

	return pgtype.UUID{}, fmt.Errorf("username collision after retries: %s", attempt)
}

// sanitizeUsernameBase derives a username base from the email local-part or
// the provider-supplied name. Lowercased, alphanumeric + underscore only,
// truncated to 20 characters.
func sanitizeUsernameBase(email, name string) string {
	source := email
	if at := strings.IndexByte(email, '@'); at > 0 {
		source = email[:at]
	}

	if source == "" {
		source = name
	}

	source = strings.ToLower(strings.TrimSpace(source))

	var b strings.Builder

	for _, c := range source {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' {
			b.WriteRune(c)
		} else if c == '.' || c == '-' || c == ' ' || c == '+' {
			b.WriteRune('_')
		}
	}

	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = "user"
	}

	if len(out) > 20 {
		out = out[:20]
	}

	return out
}

// randomSuffix returns a hex-encoded random string of the given byte length.
func randomSuffix(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}

	return hex.EncodeToString(b), nil
}

// issueSessionAndRedirect mints an access token, sets jc_token + jc_refresh
// cookies, updates last_login_at, and redirects the browser to the frontend.
// isNew controls the redirect target: new users go to /onboarding (passed as
// redirectPath by the caller); returning users go to redirectPath.
func (h *OAuthHandlers) issueSessionAndRedirect(w http.ResponseWriter, r *http.Request, userID, tenantID pgtype.UUID, redirectPath string, _ bool) {
	if h.Queries != nil {
		if err := h.Queries.UpdateLastLoginAt(r.Context(), gen.UpdateLastLoginAtParams{
			TenantID: tenantID,
			ID:       userID,
		}); err != nil {
			slog.Warn("update last_login_at", "error", err)
		}
	}

	accessToken, err := h.KeyRot.MintAccessTokenWithContext(r.Context(), userID, tenantID)
	if err != nil {
		slog.Error("mint access token", "error", err)
		h.redirectOAuthError(w, r, "token_error")

		return
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		slog.Error("generate refresh token", "error", err)
		h.redirectOAuthError(w, r, "token_error")

		return
	}

	tokenHash := HashRefreshToken(refreshToken)
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings, tenantID)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
		TenantID:   tenantID,
		UserID:     userID,
		TokenHash:  tokenHash,
		ExpiresAt:  pgtype.Timestamptz{Time: time.Now().Add(refreshTTL), Valid: true},
		DeviceName: deviceName(r),
		IpAddress:  ipPtr(r),
	}); err != nil {
		slog.Error("create refresh token", "error", err)
		h.redirectOAuthError(w, r, "token_error")

		return
	}

	// jc_token: access token, readable by the frontend hooks via cookie.
	// The cookie MaxAge must match the actual JWT expiry, which is configurable
	// via system_settings (auth.session.access_token_ttl_minutes). Using a
	// hardcoded 15 minutes would cause the cookie to expire before or after
	// the actual token if an admin changes the setting.
	// Ref: sections/02-authentication-sessions.md (Token model)
	accessTTL := GetAccessTokenTTL(r.Context(), h.Settings, tenantID)
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_token",
		Value:    accessToken,
		Path:     "/",
		Domain:   h.CookieDomain,
		MaxAge:   int(accessTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteLaxMode,
	}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically

	// jc_refresh: refresh token, scoped to /api/v1 so /me/sessions can identify
	// the current session. HttpOnly + Secure + SameSite=Strict mitigates CSRF/XSS.
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_refresh",
		Value:    refreshToken,
		Path:     "/api/v1",
		Domain:   h.CookieDomain,
		MaxAge:   int(refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: http.SameSiteStrictMode,
	}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically

	h.redirectToFrontend(w, r, redirectPath)
}

// redirectToFrontend sends a 302 to the frontend base + path. The path
// is validated by safeRedirectPath at both initiation (when stored in the
// cookie) and callback (when read back), ensuring it is a relative site
// path and not an open-redirect vector.
func (h *OAuthHandlers) redirectToFrontend(w http.ResponseWriter, r *http.Request, path string) {
	base := strings.TrimRight(h.FrontendBaseURL, "/")
	if base == "" {
		base = "/"
	}

	target := base + path
	http.Redirect(w, r, target, http.StatusFound) // #nosec G710 -- path is validated by safeRedirectPath at both initiation and callback
}

// redirectOAuthError redirects to the frontend login page with an error code.
func (h *OAuthHandlers) redirectOAuthError(w http.ResponseWriter, r *http.Request, code string) {
	base := strings.TrimRight(h.FrontendBaseURL, "/")
	if base == "" {
		base = ""
	}

	target := base + "/login?oauth_error=" + url.QueryEscape(code)
	http.Redirect(w, r, target, http.StatusFound)
}

// clearOAuthCookies expires the state and redirect cookies.
func (h *OAuthHandlers) clearOAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{"jc_oauth_state", oauthRedirectCookie} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Domain:   h.CookieDomain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   h.SecureCookies,
			SameSite: http.SameSiteLaxMode,
		}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically
	}
}

// RequireAuthRoute wraps RequireAuth for convenience in route registration.
func RequireAuthRoute(kr *KeyRotation, vkClient valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return RequireAuth(kr, vkClient)
}

// safeRedirectPath validates that a user-supplied redirect path is safe to
// use in a post-OAuth redirect. It must be a relative site path: start with
// a single "/", not "//" (protocol-relative URL), and not be an absolute URL
// (scheme://). This prevents open-redirect attacks (CWE-601) where an
// attacker sets redirect=//evil.com and the victim is sent to evil.com after
// a successful OAuth login.
func safeRedirectPath(path string) string {
	if path == "" {
		return "/"
	}

	// Must start with "/" and not "//" (protocol-relative).
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return "/"
	}

	// Reject absolute URLs (scheme://...) — these would be redirected to
	// an external host.
	if strings.Contains(path, "://") {
		return "/"
	}

	return path
}
