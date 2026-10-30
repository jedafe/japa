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
	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/email"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
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
	Clients             OAuthClientStore      // Optional — Control-managed provider credentials (jc:oauth_clients)
	AsynqClient         *asynq.Client         // Optional — welcome/verification emails on OAuth registration
	SiteName            string                // Brand name for email templates
	FrontendBaseURL     string                // Public frontend base URL (JC_SITE_URL) for post-OAuth redirects
	CookieDomain        string                // Cookie domain for cross-subdomain sharing (e.g. ".example.com"), empty = exact host
	SecureCookies       bool                  // When true, session cookies are marked Secure (production / TLS)
	DefaultFolderSetter DefaultFolderProvider // Optional — wired by server after bookmarkfolders.Service is constructed (§50)
	TierManager         TierManager           // Optional — wired by server after tiers.Service is constructed (§unified-tiers)
	SourceManager       SourceManager         // Optional — wired by server for v2 source directory auto-creation
	OwnerEmail          string                // JC_OWNER_EMAIL — designates the platform owner account (§03 owner amendment)
	OwnerPromoter       SuperAdminPromoter    // Optional — promotes the owner account into the super_admin group at registration
	Activity            ActivityEmitter       // Optional — wired by server for user.registered / daily.login point events (ADR-129)
}

// SetActivityEmitter injects the wallet activity emitter (ADR-129).
func (h *OAuthHandlers) SetActivityEmitter(e ActivityEmitter) { h.Activity = e }

// SetTierManager injects the tier manager used to assign the regular tier on
// first-time OAuth registrations.
func (h *OAuthHandlers) SetTierManager(m TierManager) { h.TierManager = m }

// SetSourceManager injects the canonical source registry for auto-creating
// user publisher profiles in the v2 source directory.
func (h *OAuthHandlers) SetSourceManager(m SourceManager) { h.SourceManager = m }

func (h *OAuthHandlers) maybeCreateUserSource(ctx context.Context, userID pgtype.UUID, username string) {
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
	if h.FrontendBaseURL != "" {
		sourceURL = fmt.Sprintf("%s/u/%s", strings.TrimRight(h.FrontendBaseURL, "/"), username)
	}

	source, err := h.SourceManager.CreateSourceFromEntity(ctx, userID, userID, "user", username, username, sourceURL, "news")
	if err != nil {
		slog.Warn("oauth register: failed to create canonical user source", "user_id", userID.String(), "error", err)
		return
	}

	if err := h.Queries.UpdateUserSourceID(ctx, gen.UpdateUserSourceIDParams{
		SourceID: source.ID,
		ID:       userID,
	}); err != nil {
		slog.Warn("oauth register: failed to link user source_id", "user_id", userID.String(), "source_id", source.ID.String(), "error", err)
	}
}

// Short-lived cookies carrying OAuth flow state between initiate and callback:
//   - jc_oauth_state: "{provider}.{token}" CSRF token — the provider prefix
//     binds the state to one provider so a state minted for google cannot be
//     replayed against the github callback.
//   - jc_oauth_redirect: URL-escaped post-login frontend path (escaped because
//     raw paths can contain bytes that are invalid in cookie values).
//   - jc_oauth_verifier: PKCE code verifier for providers that support it.
const (
	oauthStateCookie    = "jc_oauth_state"
	oauthRedirectCookie = "jc_oauth_redirect"
	oauthVerifierCookie = "jc_oauth_verifier"
)

// socialLoginAllowed reports whether the users.allow_social_login system
// setting permits social login right now. It is evaluated on both initiate
// AND callback — a provider flow in flight when the admin disables the
// setting must not complete. Fails CLOSED: a settings-read error disables
// social login rather than silently bypassing an admin lockout.
func (h *OAuthHandlers) socialLoginAllowed(ctx context.Context) bool {
	if h.Settings == nil {
		return true
	}

	allowed, err := h.Settings.GetSettingBool(ctx, "users.allow_social_login")

	return err == nil && allowed
}

// resolveProvider returns the OAuthProvider for a normalized provider name.
// Env-configured credentials win; otherwise a Control-managed client entry
// (jc:oauth_clients) is consulted so operators can configure providers
// without a redeploy.
func (h *OAuthHandlers) resolveProvider(ctx context.Context, providerName string) OAuthProvider {
	if p := h.Config.NewProvider(providerName); p != nil {
		return p
	}

	if h.Clients != nil {
		cfg, err := h.Clients.GetClientConfig(ctx, providerName)
		if err != nil {
			slog.Warn("oauth: dynamic client lookup failed", "provider", providerName, "error", err)
			return nil
		}

		if cfg != nil {
			return cfg.NewProvider(h.Config.RedirectBase)
		}
	}

	return nil
}

// SetDefaultFolderProvider injects the default bookmark folder seeder.
func (h *OAuthHandlers) SetDefaultFolderProvider(p DefaultFolderProvider) { h.DefaultFolderSetter = p }

// flowSameSite returns the SameSite mode for OAuth flow cookies. Sign in
// with Apple completes via a cross-site POST (response_mode=form_post) and
// SameSite=Lax cookies are withheld on cross-site POST navigations — Apple
// flows need SameSite=None, which browsers only accept together with Secure.
// When SecureCookies is off (local dev) Apple falls back to Lax and its
// form_post flow cannot complete — acceptable, Apple needs HTTPS anyway.
func (h *OAuthHandlers) flowSameSite(providerName string) http.SameSite {
	if providerName == "apple" && h.SecureCookies {
		return http.SameSiteNoneMode
	}

	return http.SameSiteLaxMode
}

// setOAuthFlowCookie writes a short-lived OAuth flow cookie.
func (h *OAuthHandlers) setOAuthFlowCookie(w http.ResponseWriter, name, value string, sameSite http.SameSite) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		Domain:   h.CookieDomain,
		MaxAge:   600, // 10 minutes
		HttpOnly: true,
		Secure:   h.SecureCookies,
		SameSite: sameSite,
	}) // #nosec G124 -- Secure is set via h.SecureCookies (true in production); gosec can't verify the struct field statically
}

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
	if !h.socialLoginAllowed(r.Context()) {
		errpkg.WriteError(w, http.StatusForbidden, "social_login_disabled",
			"Social login is disabled")
		return
	}

	// Normalize the route param: provider matching, storage keys and
	// usernames all assume lowercase ("google", not "Google").
	providerName := strings.ToLower(chi.URLParam(r, "provider"))

	provider := h.resolveProvider(r.Context(), providerName)
	if provider == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "unsupported_provider",
			"OAuth provider is not configured or not supported")
		return
	}

	token, err := GenerateOAuthState()
	if err != nil {
		slog.Error("generate OAuth state", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "state_error", "Failed to initiate OAuth")

		return
	}

	// The state value embeds the provider name so a state minted for one
	// provider cannot be replayed at another provider's callback.
	state := providerName + "." + token

	sameSite := h.flowSameSite(providerName)

	// Store state in a short-lived cookie for CSRF validation on callback.
	h.setOAuthFlowCookie(w, oauthStateCookie, state, sameSite)

	// PKCE: generate a verifier and pass the S256 challenge to the provider.
	// Providers that don't support PKCE ignore the challenge, but the
	// verifier cookie is cheap and harmless in that case.
	var codeChallenge string
	if verifier, verr := GeneratePKCEVerifier(); verr == nil {
		codeChallenge = pkceChallenge(verifier)
		h.setOAuthFlowCookie(w, oauthVerifierCookie, verifier, sameSite)
	}

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

	redirect = safeRedirectTarget(redirect, h.Config.RedirectBase)

	// Escape the value: raw paths can contain bytes that are invalid inside
	// a cookie value; the callback unescapes before use. PathEscape (not
	// QueryEscape) is used deliberately — it encodes spaces as %20 rather
	// than '+', so the value round-trips identically through the frontend
	// proxy's decodeURIComponent/re-encode without a '+' vs space ambiguity.
	h.setOAuthFlowCookie(w, oauthRedirectCookie, url.PathEscape(redirect), sameSite)

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"auth_url": provider.AuthURL(state, codeChallenge)},
	})
}

// oauthCallbackParams extracts the provider's callback parameters. Standard
// OAuth callbacks arrive as GET with query parameters; Sign in with Apple
// uses response_mode=form_post and POSTs the same fields (plus an optional
// "user" JSON object carrying the name on first authorization).
func oauthCallbackParams(r *http.Request) (state, code, errParam, errDesc, userJSON string) {
	if r.Method == http.MethodPost {
		_ = r.ParseForm()

		// r.Form merges URL query and body params — covers proxies that
		// leave params in the query string even on form_post.
		return r.Form.Get("state"), r.Form.Get("code"),
			r.Form.Get("error"), r.Form.Get("error_description"),
			r.Form.Get("user")
	}

	q := r.URL.Query()

	return q.Get("state"), q.Get("code"), q.Get("error"), q.Get("error_description"), ""
}

// oauthLoginAllowed enforces the same account gates as password login
// (§02, §22, ADR-068): is_bot persona accounts are refused, 'deleted' and
// other terminal statuses are hard-stopped, and the platform email-
// verification requirement applies (with the same super_admin bypass).
// When the provider attests the email the account already has, the link
// doubles as proof of ownership and the account is marked verified.
func (h *OAuthHandlers) oauthLoginAllowed(w http.ResponseWriter, r *http.Request, user *User, info *OAuthUserInfo) bool {
	if user.IsBot {
		slog.Warn("oauth login blocked: is_bot account", "user_id", user.ID, "provider", info.Provider)
		h.redirectOAuthError(w, r, "invalid_account")
		return false
	}

	if !loginAllowedStatus(user.Status) {
		h.redirectOAuthError(w, r, "account_inactive")
		return false
	}

	// Opportunistic verification: the provider attesting the very email on
	// the account is at least as strong a proof as our own link — mark it.
	if h.Queries != nil && !user.EmailVerifiedAt.Valid && info.EmailVerified && strings.EqualFold(info.Email, user.Email) {
		if err := h.Queries.MarkEmailVerifiedByUserID(r.Context(), user.ID); err != nil {
			slog.Warn("oauth: failed to mark provider-verified email", "user_id", user.ID, "error", err)
		} else {
			user.EmailVerifiedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
		}
	}

	if EmailVerifyRequired(r.Context(), h.Settings) && !user.EmailVerifiedAt.Valid {
		isSuperAdmin := false
		if h.Queries != nil {
			if ok, err := h.Queries.IsUserInGroup(r.Context(), gen.IsUserInGroupParams{
				UserID:    user.ID,
				GroupSlug: "super_admin",
			}); err == nil {
				isSuperAdmin = ok
			}
		}

		if !isSuperAdmin {
			h.redirectOAuthError(w, r, "email_not_verified")
			return false
		}
	}

	return true
}

// resolveLinkOwner re-reads oauth_accounts after a link attempt and reports
// whether the provider identity now resolves to the expected user. When it
// resolves to a DIFFERENT user (the provider account was already linked
// elsewhere), the merge must not issue a session — otherwise one provider
// identity could sign into two accounts depending on lookup order.
func (h *OAuthHandlers) resolveLinkOwner(ctx context.Context, provider, providerUserID string, userID pgtype.UUID) bool {
	linked, err := h.Store.GetOAuthAccount(ctx, provider, providerUserID)
	if err != nil || linked == nil {
		return false
	}

	return linked.UserID == userID
}

// HandleCallback handles GET|POST /api/v1/auth/oauth/{provider}/callback.
// Validates the CSRF state, exchanges the code for user info, creates or
// links the account per Section 02, sets session cookies, and redirects
// to the frontend. New users redirect to /onboarding; returning users to
// the stored redirect path (default /). Errors redirect to /login with
// an oauth_error query parameter. POST is required because Sign in with
// Apple responds with response_mode=form_post.
func (h *OAuthHandlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	providerName := strings.ToLower(chi.URLParam(r, "provider"))

	if !h.socialLoginAllowed(r.Context()) {
		h.redirectOAuthError(w, r, "social_login_disabled")
		return
	}

	provider := h.resolveProvider(r.Context(), providerName)
	if provider == nil {
		h.redirectOAuthError(w, r, "unsupported_provider")
		return
	}

	callbackState, code, errParam, errDesc, userJSON := oauthCallbackParams(r)

	// Validate CSRF state — the cookie carries "{provider}.{token}" so a
	// state minted for a different provider is rejected outright.
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" {
		h.redirectOAuthError(w, r, "missing_state")
		return
	}

	if !strings.HasPrefix(stateCookie.Value, providerName+".") ||
		!ValidateOAuthState(stateCookie.Value, callbackState) {
		h.redirectOAuthError(w, r, "invalid_state")
		return
	}

	// Clear the flow cookies (read redirect + verifier before clearing).
	// Re-validate the path even though it was validated at initiation —
	// the cookie could have been tampered with between the two requests.
	redirectPath := "/"
	if rc, err := r.Cookie(oauthRedirectCookie); err == nil && rc.Value != "" {
		if unescaped, uerr := url.PathUnescape(rc.Value); uerr == nil {
			redirectPath = safeRedirectTarget(unescaped, h.Config.RedirectBase)
		}
	}

	var codeVerifier string
	if vc, err := r.Cookie(oauthVerifierCookie); err == nil {
		codeVerifier = vc.Value
	}

	h.clearOAuthCookies(w)

	// Check for provider error
	if errParam != "" {
		slog.Warn("OAuth provider error", "provider", providerName, "error", errParam, "description", errDesc)
		h.redirectOAuthError(w, r, "oauth_error")

		return
	}

	if code == "" {
		h.redirectOAuthError(w, r, "missing_code")
		return
	}

	// Exchange code for user info. The state cookie value is passed as the
	// expected nonce — providers that echo a nonce (Apple id_token) must
	// return this exact value, binding the token to this browser's flow.
	userInfo, err := provider.Exchange(r.Context(), code, codeVerifier, stateCookie.Value)
	if err != nil {
		slog.Error("OAuth exchange failed", "provider", providerName, "error", err)
		h.redirectOAuthError(w, r, "oauth_exchange_failed")

		return
	}

	if userInfo.ProviderUserID == "" {
		slog.Error("OAuth provider returned empty subject", "provider", providerName)
		h.redirectOAuthError(w, r, "oauth_exchange_failed")

		return
	}

	// Apple sends the user's name once, in the form_post "user" field, on
	// first authorization — it is absent from every later response.
	if providerName == "apple" && userInfo.Name == "" && userJSON != "" {
		var u struct {
			Name *struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
			} `json:"name"`
		}
		if json.Unmarshal([]byte(userJSON), &u) == nil && u.Name != nil {
			userInfo.Name = strings.TrimSpace(u.Name.FirstName + " " + u.Name.LastName)
		}
	}

	// Normalize the provider email once: users.email is stored lowercase
	// (registration normalizes), so lookups and inserts must agree.
	userInfo.Email = strings.ToLower(strings.TrimSpace(userInfo.Email))

	// Try to find existing linked account — returning user.
	existing, err := h.Store.GetOAuthAccount(r.Context(), providerName, userInfo.ProviderUserID)
	if err == nil && existing != nil {
		user, uerr := h.Store.GetUserByID(r.Context(), existing.UserID)
		if uerr != nil || user == nil {
			slog.Error("oauth linked account has no user row", "user_id", existing.UserID, "error", uerr)
			h.redirectOAuthError(w, r, "account_unavailable")

			return
		}

		if !h.oauthLoginAllowed(w, r, user, userInfo) {
			return
		}

		h.issueSessionAndRedirect(w, r, user.ID, redirectPath)

		return
	}

	// Try to find user by email — if found, merge the OAuth identity ONLY
	// when the provider verified the email address. Merging on an
	// unverified address would let an attacker claim someone else's email
	// at a lax provider and take over the account. Unverified matches get
	// a pending-link redirect (no session, no silent merge).
	if userInfo.Email != "" {
		user, err := h.Store.GetUserByEmail(r.Context(), userInfo.Email)
		if err == nil && user != nil {
			if !userInfo.EmailVerified {
				// Provider did not verify the email: do NOT merge, no
				// session issued. Return the generic oauth_error — a
				// distinct code would let anyone probe "does an account
				// exist for this email" by attaching that address
				// unverified at a lax provider (e.g. GitHub).
				slog.Warn("oauth merge refused: unverified provider email",
					"provider", providerName, "user_id", user.ID)
				h.redirectOAuthError(w, r, "oauth_error")

				return
			}

			if !h.oauthLoginAllowed(w, r, user, userInfo) {
				return
			}

			if linkErr := h.Store.LinkOAuthAccount(r.Context(), user.ID, providerName, userInfo.ProviderUserID); linkErr != nil {
				slog.Error("link OAuth account", "error", linkErr)
			}

			if !h.resolveLinkOwner(r.Context(), providerName, userInfo.ProviderUserID, user.ID) {
				// The provider identity is already linked to a different
				// account (or the link failed) — refuse rather than sign
				// this user in on a contested identity.
				slog.Warn("oauth link conflict: provider identity owned by another user",
					"provider", providerName, "user_id", user.ID)
				h.redirectOAuthError(w, r, "link_conflict")

				return
			}

			h.issueSessionAndRedirect(w, r, user.ID, redirectPath)

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

	userID, err := h.createOAuthUser(r.Context(), userInfo, providerName)
	if err != nil {
		slog.Error("create OAuth user", "provider", providerName, "error", err)
		h.redirectOAuthError(w, r, "create_failed")

		return
	}

	// New user → onboarding wizard, carrying the validated redirect so
	// completion returns to it (§120 MCP consent round-trip). The path was
	// sanitized by safeRedirectTarget — it is a relative site path or an
	// allowlisted API-origin absolute URL, never an open-redirect vector.
	target := "/onboarding"
	if redirectPath != "/" {
		target += "?redirect=" + url.QueryEscape(redirectPath)
	}

	h.issueSessionAndRedirect(w, r, userID, target)
}

// createOAuthUser creates a new OAuth-only user (password_hash = NULL) with a
// unique username derived from the email local-part and provider, then links
// the OAuth account. Retries on username collision with a random suffix.
func (h *OAuthHandlers) createOAuthUser(ctx context.Context, info *OAuthUserInfo, providerName string) (pgtype.UUID, error) {
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

		// §03 alias amendment: skip candidates that collide with an existing
		// public_username alias — the DB unique index only covers username.
		if taken, terr := h.Queries.PublicHandleTaken(ctx, gen.PublicHandleTakenParams{
			Handle: candidate,
		}); terr == nil && taken > 0 {
			continue
		}

		userID, err := h.Queries.CreateUser(ctx, gen.CreateUserParams{
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
			if tierErr := h.TierManager.SetUserTier(ctx, userID, "regular"); tierErr != nil {
				slog.Warn("oauth register: failed to set regular tier", "error", tierErr, "user_id", userID.String())
			}
		}

		// §03 owner amendment: the JC_OWNER_EMAIL account is the platform
		// owner — grant it super_admin membership on first registration.
		PromoteOwnerIfMatch(ctx, h.OwnerPromoter, h.OwnerEmail, info.Email, userID)

		// A provider-verified email is proof of ownership — mark the account
		// verified so auth.email.verify_required doesn't revoke the session
		// at the first refresh (§02).
		if info.EmailVerified {
			if verr := h.Queries.MarkEmailVerifiedByUserID(ctx, userID); verr != nil {
				slog.Warn("oauth register: failed to mark provider-verified email", "error", verr)
			}
		}

		if linkErr := h.Store.LinkOAuthAccount(ctx, userID, providerName, info.ProviderUserID); linkErr != nil {
			return pgtype.UUID{}, fmt.Errorf("link OAuth account: %w", linkErr)
		}

		if !h.resolveLinkOwner(ctx, providerName, info.ProviderUserID, userID) {
			return pgtype.UUID{}, errors.New("oauth link did not resolve to the new user")
		}

		// §50 Pre-seed default bookmark folders. Non-fatal: logged on failure.
		if h.DefaultFolderSetter != nil {
			if err := h.DefaultFolderSetter.CreateDefaultFoldersForUser(ctx, userID); err != nil {
				slog.Warn("oauth register: failed to create default bookmark folders", "error", err)
			}
		}

		// v2 source directory: auto-create a canonical source for publisher users.
		h.maybeCreateUserSource(ctx, userID, candidate)

		// ADR-129: signup earns points (one_time rule) — fire-and-forget.
		emitDailyLogin0(ctx, h.Activity, "user.registered", "user:"+userID.String(), userID)

		// Transactional emails (fire-and-forget, Golden Rule 8): welcome for
		// every signup — parity with password Register — and a verification
		// link only when the provider could not attest the address.
		h.sendWelcomeEmail(info.Email, candidate)

		if !info.EmailVerified {
			h.sendVerificationEmail(ctx, userID, info.Email, candidate)
		}

		return userID, nil
	}

	return pgtype.UUID{}, fmt.Errorf("username collision after retries: %s", attempt)
}

// emitDailyLogin0 emits an arbitrary activity event — the OAuth counterpart
// to AuthHandlers.emitActivity (package-private helper, fire-and-forget).
func emitDailyLogin0(ctx context.Context, e ActivityEmitter, key, reference string, userID pgtype.UUID) {
	if e == nil || !userID.Valid {
		return
	}
	event, err := e.EmitActivityEvent(ctx, userID, AuthActivityInput{ActivityKey: key, Reference: reference})
	if err != nil {
		return
	}
	_ = e.EnqueueProcessActivityEvent(ctx, event.ID)
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
// redirectPath is the final target — callers pass /onboarding for new users
// and the stored redirect path for returning users.
func (h *OAuthHandlers) issueSessionAndRedirect(w http.ResponseWriter, r *http.Request, userID pgtype.UUID, redirectPath string) {
	if h.Queries != nil {
		if err := h.Queries.UpdateLastLoginAt(r.Context(),

			userID,
		); err != nil {
			slog.Warn("update last_login_at", "error", err)
		}
	}
	emitDailyLogin(r.Context(), h.Activity, userID)

	accessToken, err := h.KeyRot.MintAccessTokenWithContext(r.Context(), userID)
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
	refreshTTL := GetRefreshTokenTTL(r.Context(), h.Settings)

	if err := h.Queries.CreateRefreshToken(r.Context(), gen.CreateRefreshTokenParams{
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
	accessTTL := GetAccessTokenTTL(r.Context(), h.Settings)
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

// redirectToFrontend sends a 302 to the frontend base + path — or to the
// path verbatim when it is an allowlisted absolute URL (§120 MCP consent
// round-trip targets the API host). The path is validated by
// safeRedirectTarget at both initiation (when stored in the cookie) and
// callback (when read back), ensuring it is a relative site path or a URL on
// the API issuer origin — never an open-redirect vector.
func (h *OAuthHandlers) redirectToFrontend(w http.ResponseWriter, r *http.Request, path string) {
	if u, err := url.Parse(path); err == nil && u.IsAbs() {
		http.Redirect(w, r, path, http.StatusFound) // #nosec G710 -- absolute target was allowlisted to the API issuer origin by safeRedirectTarget
		return
	}

	base := strings.TrimRight(h.FrontendBaseURL, "/")
	if base == "" {
		base = "/"
	}

	target := base + path
	http.Redirect(w, r, target, http.StatusFound) // #nosec G710 -- path is validated by safeRedirectTarget at both initiation and callback
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

// clearOAuthCookies expires the OAuth flow cookies.
func (h *OAuthHandlers) clearOAuthCookies(w http.ResponseWriter) {
	for _, name := range []string{oauthStateCookie, oauthRedirectCookie, oauthVerifierCookie} {
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
	return safeRedirectTarget(path, "")
}

// safeRedirectTarget is safeRedirectPath plus one narrow exception: absolute
// http(s) URLs whose scheme+host match allowedOrigin (the API host base,
// JC_OAUTH_REDIRECT_BASE). §120 MCP consent: the login round-trip must be
// able to bounce the user back to {api-host}/oauth/authorize. Any other
// absolute URL still reduces to "/".
func safeRedirectTarget(path, allowedOrigin string) string {
	if path == "" {
		return "/"
	}

	// Relative site path: single "/", not "//" (protocol-relative). A
	// backslash is rejected too — browsers normalize "/\evil.com" to
	// "//evil.com" when following a Location header.
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		if strings.Contains(path, "\\") || strings.Contains(path, "://") {
			return "/"
		}

		return path
	}

	// Absolute URL — allowed only on the API issuer origin.
	if allowedOrigin != "" {
		u, err := url.Parse(path)
		ao, aerr := url.Parse(allowedOrigin)

		if err == nil && aerr == nil && u.IsAbs() &&
			(u.Scheme == "https" || u.Scheme == "http") &&
			strings.EqualFold(u.Scheme, ao.Scheme) &&
			strings.EqualFold(u.Host, ao.Host) {
			return path
		}
	}

	return "/"
}

// HandleProviders handles GET /api/v1/auth/oauth/providers — the public list
// of configured social-login providers so the frontend renders buttons only
// for providers that can actually complete a flow (env-configured plus
// enabled Control-managed client entries).
func (h *OAuthHandlers) HandleProviders(w http.ResponseWriter, r *http.Request) {
	// Respect the social-login gate — a disabled feature should render no
	// buttons, matching what the initiate endpoint would reject anyway.
	if !h.socialLoginAllowed(r.Context()) {
		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": []string{}})
		return
	}

	seen := map[string]bool{}
	providers := []string{}

	for _, p := range h.Config.SupportedProviders() {
		if !seen[p] {
			seen[p] = true
			providers = append(providers, p)
		}
	}

	if h.Clients != nil {
		if configs, err := h.Clients.ListClientConfigs(r.Context()); err == nil {
			for _, c := range configs {
				p := strings.ToLower(c.Provider)
				if !seen[p] && c.NewProvider(h.Config.RedirectBase) != nil {
					seen[p] = true
					providers = append(providers, p)
				}
			}
		}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": providers})
}

// sendWelcomeEmail enqueues the welcome email for a new OAuth registration —
// parity with the password Register path. Fire-and-forget (Golden Rule 8).
func (h *OAuthHandlers) sendWelcomeEmail(to, username string) {
	if h.AsynqClient == nil {
		return
	}

	welcomePayload, _ := json.Marshal(map[string]interface{}{
		"to":            to,
		"template_slug": "welcome",
		"template_data": map[string]string{
			"user_name": username,
		},
		"template_site_name": h.SiteName,
	})

	task := asynq.NewTask(worker.TypeEmailSend, welcomePayload)
	opts := append(worker.TaskOpts(worker.TypeEmailSend), asynq.MaxRetry(3))
	if _, err := h.AsynqClient.Enqueue(task, opts...); err != nil {
		slog.Warn("oauth register: failed to enqueue welcome email", "error", err)
	}
}

// sendVerificationEmail issues a verification token and enqueues the
// verify-your-email message for OAuth signups whose provider could not
// attest the address — without it they can never satisfy
// auth.email.verify_required. Fire-and-forget; failures only log.
func (h *OAuthHandlers) sendVerificationEmail(ctx context.Context, userID pgtype.UUID, to, username string) {
	if h.AsynqClient == nil || h.Queries == nil {
		return
	}

	base := strings.TrimRight(h.FrontendBaseURL, "/")
	if base == "" {
		slog.Error("oauth register: cannot send verification email without a site URL")
		return
	}

	token, hash, err := GenerateVerificationToken()
	if err != nil {
		slog.Error("oauth register: failed to generate verification token", "error", err)
		return
	}

	expiresAt := time.Now().Add(VerificationTokenTTL)
	if err := h.Queries.CreateEmailVerification(ctx, gen.CreateEmailVerificationParams{
		UserID:    userID,
		Email:     to,
		TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	}); err != nil {
		slog.Error("oauth register: failed to store verification token", "error", err)
		return
	}

	siteName := h.SiteName
	if siteName == "" {
		siteName = "App"
	}

	htmlBody, err := email.RenderEmailVerification(email.TemplateData{
		SiteName:   siteName,
		SiteURL:    base,
		UserName:   username,
		ActionURL:  fmt.Sprintf("%s/auth/verify-email?token=%s", base, token),
		Expiry:     "24 hours",
		SupportURL: base + "/support",
	})
	if err != nil {
		slog.Error("oauth register: failed to render verification email", "error", err)
		return
	}

	enqueueEmailTask(h.AsynqClient, to, "Verify Your Email", htmlBody, true)
}
