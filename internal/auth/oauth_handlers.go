package auth

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
	"github.com/valkey-io/valkey-go/valkeycompat"
)

// OAuthHandlers holds dependencies for OAuth HTTP handlers.
type OAuthHandlers struct {
	Config       OAuthConfig
	Store        OAuthStore
	Queries      *gen.Queries
	KeyRot       *KeyRotation
	CookieDomain string // Cookie domain for cross-subdomain sharing (e.g. ".example.com"), empty = exact host
}

// HandleInitiate handles POST /api/v1/auth/oauth/{provider}/initiate.
// Validates the provider, generates a CSRF state token, stores it in a
// short-lived cookie, and returns the auth URL as JSON for the frontend
// to redirect the user to the provider's login page.
func (h *OAuthHandlers) HandleInitiate(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")

	provider := h.Config.NewProvider(providerName)
	if provider == nil {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported_provider",
			"OAuth provider is not configured or not supported")
		return
	}

	state, err := GenerateOAuthState()
	if err != nil {
		slog.Error("generate OAuth state", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "state_error", "Failed to initiate OAuth")

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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]string{"auth_url": provider.AuthURL(state)},
	})
}

// HandleCallback handles GET /api/v1/auth/oauth/{provider}/callback.
// Validates the CSRF state, exchanges the code for user info, creates or
// links the account, and returns a JWT.
func (h *OAuthHandlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	providerName := chi.URLParam(r, "provider")

	provider := h.Config.NewProvider(providerName)
	if provider == nil {
		httpx.WriteError(w, http.StatusBadRequest, "unsupported_provider",
			"OAuth provider is not configured or not supported")
		return
	}

	// Validate CSRF state
	stateCookie, err := r.Cookie("jc_oauth_state")
	if err != nil || stateCookie.Value == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_state", "OAuth state cookie is missing")
		return
	}

	callbackState := r.URL.Query().Get("state")
	if !ValidateOAuthState(stateCookie.Value, callbackState) {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_state", "OAuth state mismatch — possible CSRF attack")
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "jc_oauth_state",
		Value:    "",
		Path:     "/",
		Domain:   h.CookieDomain,
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	// Check for provider error
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		desc := r.URL.Query().Get("error_description")
		slog.Warn("OAuth provider error", "provider", providerName, "error", errParam, "description", desc)
		httpx.WriteError(w, http.StatusUnauthorized, "oauth_error", "Authorization was denied or failed")

		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		httpx.WriteError(w, http.StatusBadRequest, "missing_code", "Authorization code is required")
		return
	}

	// Exchange code for user info
	userInfo, err := provider.Exchange(r.Context(), code)
	if err != nil {
		slog.Error("OAuth exchange failed", "provider", providerName, "error", err)
		httpx.WriteError(w, http.StatusUnauthorized, "oauth_exchange_failed", "Failed to verify with OAuth provider")

		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Try to find existing linked account
	existing, err := h.Store.GetOAuthAccount(r.Context(), t.ID, providerName, userInfo.ProviderUserID)
	if err == nil && existing != nil {
		// Account already linked — issue JWT
		h.issueToken(w, r, existing.UserID, t.ID)
		return
	}

	// Try to find user by email — if found, link the account
	if userInfo.Email != "" {
		user, err := h.Store.GetUserByEmail(r.Context(), t.ID, userInfo.Email)
		if err == nil && user != nil {
			if linkErr := h.Store.LinkOAuthAccount(r.Context(), t.ID, user.ID, providerName, userInfo.ProviderUserID); linkErr != nil {
				slog.Error("link OAuth account", "error", linkErr)
			}

			h.issueToken(w, r, user.ID, t.ID)

			return
		}
	}

	// No existing account or email match — new user signup would go here.
	// For now, return an error directing users to register first.
	httpx.WriteError(w, http.StatusForbidden, "account_not_found",
		"No account found. Please register with email/password first, then link your OAuth account from settings.")
}

// issueToken mints a JWT and returns it as a JSON response.
func (h *OAuthHandlers) issueToken(w http.ResponseWriter, r *http.Request, userID, tenantID pgtype.UUID) {
	if h.Queries != nil {
		if err := h.Queries.UpdateLastLoginAt(r.Context(), gen.UpdateLastLoginAtParams{
			TenantID: tenantID,
			ID:       userID,
		}); err != nil {
			slog.Warn("update last_login_at", "error", err)
		}
	}

	token, err := h.KeyRot.MintAccessTokenWithContext(r.Context(), userID, tenantID)
	if err != nil {
		slog.Error("mint access token", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "token_error", "Failed to create session")

		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"access_token": token,
			"user": map[string]interface{}{
				"id": userID.String(),
			},
		},
	})
}

// RequireAuthRoute wraps RequireAuth for convenience in route registration.
func RequireAuthRoute(kr *KeyRotation, vkClient valkeycompat.Cmdable) func(http.Handler) http.Handler {
	return RequireAuth(kr, vkClient)
}
