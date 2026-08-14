package auth

import (
	"encoding/json"
	"log/slog"
	"net/http"

	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/tenant"
)

// TOTPHandlers holds dependencies for TOTP HTTP handlers.
type TOTPHandlers struct {
	Store    TOTPStore
	SiteName string
}

// HandleSetup handles POST /api/v1/me/totp/enable.
// Generates a new TOTP secret and returns the secret + otpauth URL for the
// user to scan with their authenticator app. The secret is stored encrypted
// but 2FA is not yet enabled — the user must verify a code via /totp/confirm.
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (2FA TOTP)
func (h *TOTPHandlers) HandleSetup(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	t := tenant.FromContext(r.Context())
	if t == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return
	}

	// Check if 2FA is already enabled — refuse to re-setup
	existing, err := h.Store.GetTOTPSecret(r.Context(), user.UserID)
	if err == nil && existing != nil && existing.Enabled {
		errpkg.WriteError(w, http.StatusConflict, "totp_enabled", "2FA is already enabled. Disable it first.")
		return
	}

	// Fetch the user's email for the QR code label so authenticator apps
	// display the correct identity instead of a placeholder.
	email, err := h.Store.GetUserEmail(r.Context(), user.UserID)
	if err != nil {
		slog.Warn("fetch user email for TOTP, using fallback", "user_id", user.UserID.String(), "error", err)
		email = "user@" + user.UserID.String() + ".local"
	}

	secret, otpauthURL, err := GenerateTOTPSecret(user.UserID.String(), email, h.SiteName)
	if err != nil {
		slog.Error("generate TOTP secret", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "totp_error", "Failed to generate 2FA secret")
		return
	}

	// Store the encrypted secret (not enabled yet)
	encrypted, err := encryptSecret(secret)
	if err != nil {
		slog.Error("encrypt TOTP secret", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "totp_error", "Failed to encrypt 2FA secret")
		return
	}

	if err := h.Store.UpsertTOTPSecret(r.Context(), user.UserID, encrypted); err != nil {
		slog.Error("store TOTP secret", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "totp_error", "Failed to save 2FA secret")
		return
	}

	// Return the plaintext secret and otpauth URL. The secret is needed for
	// manual entry when the user can't scan the QR code. It is only returned
	// this one time during setup — the stored copy is encrypted.
	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"secret":      secret,
			"otpauth_url": otpauthURL,
		},
	})
}

// HandleVerify handles POST /api/v1/me/totp/confirm.
// Verifies the TOTP code and enables 2FA for the user. This is the
// second step after /totp/enable — the user scans the QR code, enters
// the 6-digit code, and this endpoint confirms it, enables 2FA, and
// generates one-time backup codes for account recovery.
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (2FA TOTP)
func (h *TOTPHandlers) HandleVerify(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "Verification code is required")
		return
	}

	if err := EnableTOTP(r.Context(), h.Store, user.UserID, req.Code); err != nil {
		slog.Warn("TOTP verify failed", "user_id", user.UserID.String(), "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "totp_verify_failed", err.Error())
		return
	}

	// Generate one-time backup codes for account recovery.
	// Stored as SHA-256 hashes; plaintext returned once for the user to save.
	backupCodes, err := GenerateBackupCodes()
	if err != nil {
		slog.Error("generate backup codes", "user_id", user.UserID.String(), "error", err)
		// 2FA is already enabled — don't fail the whole request, just
		// return without backup codes. The user can regenerate later.
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"enabled":      true,
				"backup_codes": []string{},
				"message":      "2FA has been enabled. Backup code generation failed — please contact support.",
			},
		})
		return
	}

	hashed := make([]string, len(backupCodes))
	for i, c := range backupCodes {
		hashed[i] = hashBackupCode(c)
	}
	if err := h.Store.SaveBackupCodes(r.Context(), user.UserID, hashed); err != nil {
		slog.Error("save backup codes", "user_id", user.UserID.String(), "error", err)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"enabled":      true,
			"backup_codes": backupCodes,
			"message":      "2FA has been enabled",
		},
	})
}

// HandleDisable handles DELETE /api/v1/me/totp.
// Requires a valid TOTP code to confirm the disable action, preventing
// an attacker with session access from silently turning off 2FA.
// Ref: japaconnect-engineering/sections/02-authentication-sessions.md (2FA TOTP)
func (h *TOTPHandlers) HandleDisable(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	var req struct {
		Code string `json:"code"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Request body is not valid JSON")
		return
	}

	req.Code = sanitizeCode(req.Code)
	if req.Code == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_code", "Verification code is required to disable 2FA")
		return
	}

	if err := DisableTOTP(r.Context(), h.Store, user.UserID, req.Code); err != nil {
		slog.Warn("TOTP disable failed", "user_id", user.UserID.String(), "error", err)
		errpkg.WriteError(w, http.StatusBadRequest, "totp_disable_failed", err.Error())

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{
			"enabled": false,
			"message": "2FA has been disabled",
		},
	})
}

// HandleStatus handles GET /api/v1/me/totp/status — whether 2FA is enabled
// for the caller.
func (h *TOTPHandlers) HandleStatus(w http.ResponseWriter, r *http.Request) {
	user := authUserFromContext(w, r)
	if user == nil {
		return
	}

	record, err := h.Store.GetTOTPSecret(r.Context(), user.UserID)
	if err != nil || record == nil {
		errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{"enabled": false},
		})

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{
		"data": map[string]interface{}{"enabled": record.Enabled},
	})
}

// authUserFromContext extracts the authenticated user from the request context.
// Writes a 401 error and returns nil if no user is present.
func authUserFromContext(w http.ResponseWriter, r *http.Request) *AuthenticatedUser {
	user := FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
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

	for i := range len(s) {
		found := false

		for j := range len(chars) {
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
