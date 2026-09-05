// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
package devices

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the device token HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates device token handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleRegisterDevice registers or updates a device token.
// POST /api/v1/me/devices.
func (h *Handlers) HandleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var input RegisterDeviceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	info, err := h.svc.RegisterDevice(r.Context(), user.TenantID, user.UserID, input)
	if err != nil {
		writeDeviceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]interface{}{"data": info})
}

// HandleDeregisterDevice removes a device token.
// DELETE /api/v1/me/devices/{device_id}.
func (h *Handlers) HandleDeregisterDevice(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	deviceID := chi.URLParam(r, "device_id")
	if deviceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_param", "device_id is required")
		return
	}

	if err := h.svc.DeregisterDevice(r.Context(), user.TenantID, user.UserID, deviceID); err != nil {
		writeDeviceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": map[string]bool{"deregistered": true}})
}

// HandleListDevices returns the user's registered devices.
// GET /api/v1/me/devices.
func (h *Handlers) HandleListDevices(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	devices, err := h.svc.ListDevices(r.Context(), user.TenantID, user.UserID)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	// Always emit a JSON array, never null, for an empty list (§6.2).
	if devices == nil {
		devices = []DeviceInfo{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": devices})
}

// HandleAdminTestPush sends a test push to a device.
// POST /api/v1/admin/push/test.
func (h *Handlers) HandleAdminTestPush(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var body struct {
		DeviceID string `json:"device_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if body.DeviceID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "device_id is required")
		return
	}

	if err := h.svc.SendTestPush(r.Context(), user.TenantID, body.DeviceID); err != nil {
		writeDeviceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]interface{}{"data": map[string]bool{"sent": true}})
}

// --- shared helpers ---

// requireAuth resolves the authenticated user from the request context using
// the auth middleware's UserContextKey (BE-001: the previous private
// tenantCtxKey/userCtxKey types were never populated by any middleware, so
// every endpoint always returned 401).
func requireAuth(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid || !user.TenantID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// writeDeviceError maps a devices service error to the standard error envelope
// (Rule 5). Sentinel errors get stable snake_case codes; unknown errors get a
// generic 400 with the wrapped message (BE-004: the previous errors.Is against
// errors.New(...) was dead code).
func writeDeviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMissingFields):
		errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "device_id and token are required")
	case errors.Is(err, ErrInvalidPlatform):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_platform", err.Error())
	case errors.Is(err, ErrDeviceNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "device_not_found", "Device not found")
	case errors.Is(err, ErrPushDisabled):
		errpkg.WriteError(w, http.StatusForbidden, "push_disabled", "Push notifications are disabled")
	case errors.Is(err, ErrNoDispatcher):
		errpkg.WriteError(w, http.StatusServiceUnavailable, "push_unavailable", "Push delivery is not configured")
	case errors.Is(err, ErrNoActiveTokens):
		errpkg.WriteError(w, http.StatusNotFound, "no_active_tokens", "No active device tokens for user")
	default:
		errpkg.WriteError(w, http.StatusBadRequest, "device_error", err.Error())
	}
}
