package admin

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/rbac"
)

// validOperations is the set of operations that can be restricted.
var validOperations = map[string]bool{
	"post": true, "comment": true, "discussion": true, "blog": true, "message": true,
}

// RestrictUserRequest is the body for POST /admin/users/{id}/restrict.
type RestrictUserRequest struct {
	Operations []string `json:"operations"`
	Reason     string   `json:"reason"`
	Duration   string   `json:"duration"` // "1h", "2h", "6h", "12h", "1d", "7d", "30d", "permanent"
}

// durationMap maps human-readable durations to seconds.
var durationMap = map[string]int{
	"1h":        3600,
	"2h":        7200,
	"6h":        21600,
	"12h":       43200,
	"1d":        86400,
	"7d":        604800,
	"30d":       2592000,
	"permanent": 0,
}

// RestrictUser handles POST /admin/users/{id}/restrict.
// Creates per-operation bans for the specified operations and duration.
func (h *Handlers) RestrictUser(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input RestrictUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.Operations) == 0 {
		writeError(w, http.StatusBadRequest, "validation", "At least one operation is required")
		return
	}

	seconds, ok := durationMap[input.Duration]
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_duration", "Duration must be one of: 1h, 2h, 6h, 12h, 1d, 7d, 30d, permanent")
		return
	}

	var actorID pgtype.UUID
	actorID.Scan(actor.UserID.String())

	ctx := r.Context()
	for _, op := range input.Operations {
		if !validOperations[op] {
			writeError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
			return
		}
		// Soft-delete any existing active restriction for this user+operation first.
		_ = h.Queries.SoftDeleteActiveOperationBan(ctx, gen.SoftDeleteActiveOperationBanParams{
			UserID:    uid,
			Operation: gen.BanOperation(op),
		})
		if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
			UserID:    uid,
			TenantID:  tenantID,
			Operation: gen.BanOperation(op),
			Reason:    &input.Reason,
			Column5:   int32(seconds),
			IssuedBy:  actorID,
		}); err != nil {
			slog.Error("restrict: failed to create operation ban", "error", err, "user_id", uid.String(), "operation", op)
			writeError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")
			return
		}
	}

	// Set user status to 'restricted'.
	_ = h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "restricted",
		ID:     uid,
	})

	// Invalidate caches.
	if h.Valkey != nil {
		_ = rbac.InvalidatePermCache(ctx, h.Valkey, uid)
		_ = h.Valkey.Del(ctx, "jc:ban:"+uid.String())
		_ = h.Valkey.Del(ctx, "jc:restrictions:"+uid.String())
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id":    uid.String(),
		"operations": input.Operations,
		"duration":   input.Duration,
		"reason":     input.Reason,
	})
}

// LiftRestriction handles DELETE /admin/users/{id}/restrict/{operation}.
// Lifts a single operation ban.
func (h *Handlers) LiftRestriction(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	op := chi.URLParam(r, "operation")
	if !validOperations[op] {
		writeError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
		return
	}

	if err := h.Queries.LiftOperationBan(r.Context(), gen.LiftOperationBanParams{
		UserID:    uid,
		Operation: gen.BanOperation(op),
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restriction")
		return
	}

	// If no active restrictions remain, restore user status to 'active'.
	remaining, _ := h.Queries.CountActiveOperationBans(r.Context(), uid)
	if remaining == 0 {
		_ = h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
			Status: "active",
			ID:     uid,
		})
	}

	if h.Valkey != nil {
		_ = rbac.InvalidatePermCache(r.Context(), h.Valkey, uid)
		_ = h.Valkey.Del(r.Context(), "jc:restrictions:"+uid.String())
	}

	writeJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "operation": op, "lifted": true})
}

// LiftAllRestrictions handles DELETE /admin/users/{id}/restrict.
// Lifts all operation bans for a user.
func (h *Handlers) LiftAllRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Queries.LiftAllOperationBans(r.Context(), uid); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restrictions")
		return
	}

	// Restore user status to 'active' when all restrictions are lifted.
	_ = h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: "active",
		ID:     uid,
	})

	if h.Valkey != nil {
		_ = rbac.InvalidatePermCache(r.Context(), h.Valkey, uid)
		_ = h.Valkey.Del(r.Context(), "jc:restrictions:"+uid.String())
	}

	writeJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "lifted_all": true})
}

// ListRestrictions handles GET /admin/users/{id}/restrictions.
// Lists active operation bans for a user.
func (h *Handlers) ListRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	rows, err := h.Queries.ListActiveOperationBans(r.Context(), uid)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list restrictions")
		return
	}

	type Restriction struct {
		ID        string     `json:"id"`
		Operation string     `json:"operation"`
		Reason    *string    `json:"reason"`
		ExpiresAt *time.Time `json:"expires_at"`
		CreatedAt time.Time  `json:"created_at"`
	}
	items := make([]Restriction, len(rows))
	for i, row := range rows {
		items[i] = Restriction{
			ID:        row.ID.String(),
			Operation: row.Operation,
			Reason:    row.Reason,
			CreatedAt: row.CreatedAt.Time,
		}
		if row.ExpiresAt.Valid {
			items[i].ExpiresAt = &row.ExpiresAt.Time
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

// SuspendUserRequest is the body for POST /admin/users/{id}/suspend.
type SuspendUserRequest struct {
	Reason string `json:"reason"`
}

// SuspendUser handles POST /admin/users/{id}/suspend.
// Creates a permanent full suspension: bans from all operations + sets status.
func (h *Handlers) SuspendUser(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input SuspendUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	ctx := r.Context()

	// 1. Create suspension record.
	if err := h.Queries.CreateUserSuspension(ctx, gen.CreateUserSuspensionParams{
		UserID:   uid,
		TenantID: tenantID,
		Reason:   &input.Reason,
	}); err != nil {
		slog.Error("suspend: failed to create suspension record", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to suspend user")
		return
	}

	// 2. Ban from all operations permanently.
	for _, op := range middleware.AllOperations() {
		if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
			UserID:    uid,
			TenantID:  tenantID,
			Operation: gen.BanOperation(op),
			Reason:    &input.Reason,
			Column5:   0, // permanent
		}); err != nil {
			slog.Error("suspend: failed to create operation ban", "error", err, "operation", op)
		}
	}

	// 3. Update user status.
	if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "suspended",
		ID:     uid,
	}); err != nil {
		slog.Error("suspend: failed to update user status", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}

	// 4. Invalidate all caches.
	if h.Valkey != nil {
		_ = rbac.InvalidatePermCache(ctx, h.Valkey, uid)
		_ = h.Valkey.Del(ctx, "jc:ban:"+uid.String())
		_ = h.Valkey.Del(ctx, "jc:restrictions:"+uid.String())
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id": uid.String(),
		"status":  "suspended",
		"reason":  input.Reason,
	})
}

// WarnUserTypedRequest is the body for POST /admin/users/{id}/warn (typed).
type WarnUserTypedRequest struct {
	Reason        string `json:"reason"`
	ViolationType string `json:"violation_type"` // "spam", "harassment", "misinformation", "hate_speech", "other"
}

// violationToOperations maps violation types to operations to auto-restrict.
var violationToOperations = map[string][]string{
	"spam":            {"post", "comment"},
	"harassment":      {"message", "comment"},
	"misinformation":  {"post", "blog"},
	"hate_speech":     {"post", "comment", "discussion"},
}

// WarnUserTyped handles POST /admin/users/{id}/warn with violation_type.
// After 3 warnings of the same type, auto-restricts the related operations for 7 days.
func (h *Handlers) WarnUserTyped(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input WarnUserTypedRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.ViolationType == "" {
		writeError(w, http.StatusBadRequest, "validation", "violation_type is required")
		return
	}

	ctx := r.Context()

	// Create typed warning.
	if err := h.Queries.CreateUserTypedWarning(ctx, gen.CreateUserTypedWarningParams{
		UserID:        uid,
		TenantID:      tenantID,
		Reason:        &input.Reason,
		ViolationType: &input.ViolationType,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to issue warning")
		return
	}

	// Check warning count for auto-escalation.
	count, _ := h.Queries.CountUserWarningsByType(ctx, gen.CountUserWarningsByTypeParams{
		UserID:        uid,
		ViolationType: &input.ViolationType,
	})

	autoRestricted := false
	if count >= 3 {
		// Auto-escalate: restrict from related operations for 7 days.
		ops := violationToOperations[input.ViolationType]
		for _, op := range ops {
			_ = h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
				UserID:    uid,
				TenantID:  tenantID,
				Operation: gen.BanOperation(op),
				Reason:    strPtr("Auto-restricted after 3 " + input.ViolationType + " warnings"),
				Column5:   604800, // 7 days in seconds
			})
		}
		autoRestricted = len(ops) > 0
		if h.Valkey != nil {
			_ = rbac.InvalidatePermCache(ctx, h.Valkey, uid)
			_ = h.Valkey.Del(ctx, "jc:restrictions:"+uid.String())
		}
		slog.Warn("warn: auto-escalation triggered",
			"user_id", uid.String(),
			"violation_type", input.ViolationType,
			"count", count,
			"operations", ops)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"user_id":         uid.String(),
		"violation_type":  input.ViolationType,
		"warning_count":   count,
		"auto_restricted": autoRestricted,
	})
}

func strPtr(s string) *string { return &s }
