// Package devices implements the Mobile Push / Device Token registration
// surface (Section 77, ADR-074).
//
// Ref: japaconnect-engineering/sections/77-flutter-mobile-app.md
package devices

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/go-chi/chi/v5"
)

// Handlers exposes the device token HTTP routes.
type Handlers struct {
	svc *Service
}

// NewHandlers creates device token handlers.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// HandleRegisterDevice registers or updates a device token.
// POST /me/devices
func (h *Handlers) HandleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	var input RegisterDeviceInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	token, err := h.svc.RegisterDevice(r.Context(), tenantID, userID, input)
	if err != nil {
		if errors.Is(err, errors.New("devices: device_id and token are required")) {
			writeError(w, http.StatusBadRequest, "missing_fields", "device_id and token are required")
			return
		}
		writeError(w, http.StatusBadRequest, "register_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         token.ID.String(),
		"device_id":  token.DeviceID,
		"platform":   token.Platform,
		"is_active":  token.IsActive,
	})
}

// HandleDeregisterDevice removes a device token.
// DELETE /me/devices/{device_id}
func (h *Handlers) HandleDeregisterDevice(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	deviceID := chi.URLParam(r, "device_id")
	if deviceID == "" {
		writeError(w, http.StatusBadRequest, "missing_param", "device_id is required")
		return
	}

	if err := h.svc.DeregisterDevice(r.Context(), tenantID, userID, deviceID); err != nil {
		writeError(w, http.StatusBadRequest, "deregister_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"deregistered": true})
}

// HandleListDevices returns the user's registered devices.
// GET /me/devices
func (h *Handlers) HandleListDevices(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	userID, ok := requireUser(w, r)
	if !ok {
		return
	}

	tokens, err := h.svc.ListDevices(r.Context(), tenantID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": tokens})
}

// HandleAdminTestPush sends a test push to a device.
// POST /admin/push/test
func (h *Handlers) HandleAdminTestPush(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var body struct {
		DeviceID string `json:"device_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}
	if body.DeviceID == "" {
		writeError(w, http.StatusBadRequest, "missing_fields", "device_id is required")
		return
	}

	if err := h.svc.SendTestPush(r.Context(), tenantID, body.DeviceID); err != nil {
		writeError(w, http.StatusBadRequest, "test_push_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"sent": true})
}

// --- shared helpers ---

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}

type tenantCtxKey struct{}
type userCtxKey struct{}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	v := r.Context().Value(tenantCtxKey{})
	if v == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Tenant context required")
		return pgtype.UUID{}, false
	}
	id, ok := v.(pgtype.UUID)
	if !ok || !id.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid tenant context")
		return pgtype.UUID{}, false
	}
	return id, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	v := r.Context().Value(userCtxKey{})
	if v == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}
	id, ok := v.(pgtype.UUID)
	if !ok || !id.Valid {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Invalid user context")
		return pgtype.UUID{}, false
	}
	return id, true
}
