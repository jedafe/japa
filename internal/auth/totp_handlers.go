package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jedafe/japa/internal/tenant"
)

// TOTPHandlers holds dependencies for TOTP HTTP handlers.
type TOTPHandlers struct {
	Store TOTPStore
}

// HandleSetup handles POST /api/v1/auth/totp/setup.
// Generates a new TOTP secret and returns the QR code URL for the user
// to scan with their authenticator app. The secret is stored but 2FA
// is not yet enabled — the user must verify a code via /totp/verify first.
func (h *TOTPHandlers) HandleSetup(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Check if 2FA is already enabled — refuse to re-setup
	existing, err := h.Store.GetTOTPSecret(r.Context(), user.UserID)
	if err == nil && existing != nil && existing.Enabled {
		writeError(w, http.StatusConflict, "totp_enabled", "2FA is already enabled. Disable it first.")
		return
	}

	// We need the email for the QR code label. Fetch from existing record
	// or use a placeholder — the user object from JWT doesn't carry email.
	secret, qrURL, err := GenerateTOTPSecret(user.UserID.String(), "user@"+user.UserID.String()+".local")
	if err != nil {
		slog.Error("generate TOTP secret", "error", err)
		writeError(w, http.StatusInternalServerError, "totp_error", "Failed to generate 2FA secret")
		return
	}

	// Store the encrypted secret (not enabled yet)
	encrypted, err := encryptSecret(secret)
	if err != nil {
		slog.Error("encrypt TOTP secret", "error", err)
		writeError(w, http.StatusInternalServerError, "totp_error", "Failed to encrypt 2FA secret")
		return
	}
	if err := h.Store.UpsertTOTPSecret(r.Context(), user.UserID, encrypted); err != nil {
		slog.Error("store TOTP secret", "error", err)
		writeError(w, http.StatusInternalServerError, "totp_error", "Failed to save 2FA secret")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"secret": secret,
			"qr_url": qrURL,
		},
	})
}

// HandleVerify handles POST /api/v1/auth/totp/verify.
// Verifies the TOTP code and enables 2FA for the user. This is the
// second step after /totp/setup — the user scans the QR code, enters
// the 6-digit code, and this endpoint confirms it and flips the switch.
func (h *TOTPHandlers) HandleVerify(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "Verification code is required")
		return
	}

	if err := EnableTOTP(r.Context(), h.Store, user.UserID, req.Code); err != nil {
		slog.Warn("TOTP verify failed", "user_id", user.UserID.String(), "error", err)
		writeError(w, http.StatusBadRequest, "totp_verify_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"enabled": true,
			"message": "2FA has been enabled",
		},
	})
}

// HandleDisable handles POST /api/v1/auth/totp/disable.
// Requires a valid TOTP code to confirm the disable action, preventing
// an attacker with session access from silently turning off 2FA.
func (h *TOTPHandlers) HandleDisable(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		writeError(w, http.StatusBadRequest, "missing_code", "Verification code is required to disable 2FA")
		return
	}

	if err := DisableTOTP(r.Context(), h.Store, user.UserID, req.Code); err != nil {
		slog.Warn("TOTP disable failed", "user_id", user.UserID.String(), "error", err)
		writeError(w, http.StatusBadRequest, "totp_disable_failed", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"enabled": false,
			"message": "2FA has been disabled",
		},
	})
}

// authUserFromContext extracts the authenticated user from the request context.
// Writes a 401 error and returns nil if no user is present.
func authUserFromContext(w http.ResponseWriter, r *http.Request) *AuthenticatedUser {
	user := FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return nil
	}
	return user
}

// sanitizeCode strips spaces and dashes from a TOTP code, accepting
// both "123 456" and "123-456" formats from authenticator apps.
func sanitizeCode(code string) string {
	code = removeChars(code, " -")
	return code
}

// removeChars removes all occurrences of chars from s.
func removeChars(s, chars string) string {
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		found := false
		for j := 0; j < len(chars); j++ {
			if s[i] == chars[j] {
				found = true
				break
			}
		}
		if !found {
			result = append(result, s[i])
		}
	}
	return string(result)
}
