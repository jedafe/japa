package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	"github.com/jedafe/japa/internal/httpx"
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input RestrictUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.Operations) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "At least one operation is required")
		return
	}

	seconds, ok := durationMap[input.Duration]
	if !ok {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_duration", "Duration must be one of: 1h, 2h, 6h, 12h, 1d, 7d, 30d, permanent")
		return
	}

	var actorID pgtype.UUID
	if err := actorID.Scan(actor.UserID.String()); err != nil {
		slog.Error("restrict: invalid actor id", "error", err, "actor_id", actor.UserID.String())
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

		return
	}

	ctx := r.Context()
	for _, op := range input.Operations {
		if !validOperations[op] {
			httpx.WriteError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
			return
		}
		// Soft-delete any existing active restriction for this user+operation first.
		if err := h.Queries.SoftDeleteActiveOperationBan(ctx, gen.SoftDeleteActiveOperationBanParams{
			UserID:    uid,
			Operation: gen.BanOperation(op),
		}); err != nil {
			slog.Error("restrict: failed to soft-delete existing operation ban",
				"error", err, "user_id", uid.String(), "operation", op)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

			return
		}
		if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
			UserID:    uid,
			TenantID:  tenantID,
			Operation: gen.BanOperation(op),
			Reason:    &input.Reason,
			Column5:   int32(seconds),
			IssuedBy:  actorID,
		}); err != nil {
			slog.Error("restrict: failed to create operation ban", "error", err, "user_id", uid.String(), "operation", op)
			httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")
			return
		}
	}

	// Set user status to 'restricted'.
	if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
		Status: "restricted",
		ID:     uid,
	}); err != nil {
		slog.Error("restrict: failed to update user status", "error", err, "user_id", uid.String())
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

		return
	}

	// Invalidate caches so the new restrictions take effect immediately.
	h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid), banCacheKey(uid))

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	op := chi.URLParam(r, "operation")
	if !validOperations[op] {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
		return
	}

	if err := h.Queries.LiftOperationBan(r.Context(), gen.LiftOperationBanParams{
		UserID:    uid,
		Operation: gen.BanOperation(op),
	}); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restriction")
		return
	}

	// If no active restrictions remain, restore user status to 'active'. On a
	// count failure the status is left as-is rather than optimistically
	// restoring access the user may not be entitled to.
	remaining, err := h.Queries.CountActiveOperationBans(r.Context(), uid)
	if err != nil {
		slog.Error("lift restriction: failed to count active operation bans",
			"error", err, "user_id", uid.String())
	} else if remaining == 0 {
		if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
			Status: "active",
			ID:     uid,
		}); err != nil {
			slog.Error("lift restriction: failed to restore user status", "error", err, "user_id", uid.String())
		}
	}

	h.invalidateUserAccessCaches(r.Context(), uid, restrictionsCacheKey(uid))

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "operation": op, "lifted": true})
}

// LiftAllRestrictions handles DELETE /admin/users/{id}/restrict.
// Lifts all operation bans for a user.
func (h *Handlers) LiftAllRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Queries.LiftAllOperationBans(r.Context(), uid); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restrictions")
		return
	}

	// Restore user status to 'active' when all restrictions are lifted.
	if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
		Status: "active",
		ID:     uid,
	}); err != nil {
		slog.Error("lift all restrictions: failed to restore user status", "error", err, "user_id", uid.String())
	}

	h.invalidateUserAccessCaches(r.Context(), uid, restrictionsCacheKey(uid))

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "lifted_all": true})
}

// ListRestrictions handles GET /admin/users/{id}/restrictions.
// Lists active operation bans for a user.
func (h *Handlers) ListRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	rows, err := h.Queries.ListActiveOperationBans(r.Context(), uid)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list restrictions")
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

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input SuspendUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to suspend user")
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")
		return
	}

	// 4. Invalidate all caches.
	h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid), banCacheKey(uid))

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
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
	"spam":           {"post", "comment"},
	"harassment":     {"message", "comment"},
	"misinformation": {"post", "blog"},
	"hate_speech":    {"post", "comment", "discussion"},
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input WarnUserTypedRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.ViolationType == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "violation_type is required")
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
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to issue warning")
		return
	}

	// Check warning count for auto-escalation.
	count, err := h.Queries.CountUserWarningsByType(ctx, gen.CountUserWarningsByTypeParams{
		UserID:        uid,
		ViolationType: &input.ViolationType,
	})
	if err != nil {
		slog.Error("warn: failed to count warnings for auto-escalation",
			"error", err, "user_id", uid.String(), "violation_type", input.ViolationType)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to issue warning")

		return
	}

	autoRestricted := false
	if count >= 3 {
		// Auto-escalate: restrict from related operations for 7 days.
		ops := violationToOperations[input.ViolationType]
		for _, op := range ops {
			if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
				UserID:    uid,
				TenantID:  tenantID,
				Operation: gen.BanOperation(op),
				Reason:    strPtr("Auto-restricted after 3 " + input.ViolationType + " warnings"),
				Column5:   604800, // 7 days in seconds
			}); err != nil {
				slog.Error("warn: failed to auto-restrict operation",
					"error", err, "user_id", uid.String(), "operation", op)
			}
		}

		autoRestricted = len(ops) > 0

		h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid))
		slog.Warn("warn: auto-escalation triggered",
			"user_id", uid.String(),
			"violation_type", input.ViolationType,
			"count", count,
			"operations", ops)
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"user_id":         uid.String(),
		"violation_type":  input.ViolationType,
		"warning_count":   count,
		"auto_restricted": autoRestricted,
	})
}

func strPtr(s string) *string { return &s }

// banCacheKey returns the Valkey key holding a user's cached ban state.
func banCacheKey(uid pgtype.UUID) string { return "jc:ban:" + uid.String() }

// restrictionsCacheKey returns the Valkey key holding a user's cached
// per-operation restrictions.
func restrictionsCacheKey(uid pgtype.UUID) string { return "jc:restrictions:" + uid.String() }

// invalidateUserAccessCaches drops the cached RBAC permission set plus the
// given Valkey keys for a user after a moderation action. Failures are logged
// rather than returned — the write is already committed — but they are never
// dropped silently, since a stale cache lets a restricted user keep acting.
func (h *Handlers) invalidateUserAccessCaches(ctx context.Context, uid pgtype.UUID, keys ...string) {
	if h.Valkey == nil {
		return
	}

	if err := rbac.InvalidatePermCache(ctx, h.Valkey, uid); err != nil {
		slog.Error("failed to invalidate permission cache", "error", err, "user_id", uid.String())
	}

	if len(keys) == 0 {
		return
	}

	if err := h.Valkey.Del(ctx, keys...).Err(); err != nil {
		slog.Error("failed to invalidate moderation cache", "error", err, "user_id", uid.String(), "keys", keys)
	}
}
