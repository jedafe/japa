package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/middleware"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/valkey"
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

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input RestrictUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if len(input.Operations) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation", "At least one operation is required")
		return
	}

	seconds, ok := durationMap[input.Duration]
	if !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_duration", "Duration must be one of: 1h, 2h, 6h, 12h, 1d, 7d, 30d, permanent")
		return
	}
	if seconds < 0 || seconds > math.MaxInt32 {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_duration", "Duration out of range")
		return
	}

	var actorID pgtype.UUID
	if err := actorID.Scan(actor.UserID.String()); err != nil {
		slog.Error("restrict: invalid actor id", "error", err, "actor_id", actor.UserID.String())
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

		return
	}

	ctx := r.Context()

	for _, op := range input.Operations {
		if !validOperations[op] {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
			return
		}
		// Soft-delete any existing active restriction for this user+operation first.
		if err := h.Queries.SoftDeleteActiveOperationBan(ctx, gen.SoftDeleteActiveOperationBanParams{
			UserID:    uid,
			Operation: gen.BanOperation(op),
		}); err != nil {
			slog.Error("restrict: failed to soft-delete existing operation ban",
				"error", err, "user_id", uid.String(), "operation", op)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

			return
		}

		if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
			UserID:    uid,
			Operation: gen.BanOperation(op),
			Reason:    &input.Reason,
			Column4:   int32(seconds), //nolint:gosec // G115: seconds is validated against durationMap and math.MaxInt32.
			IssuedBy:  actorID,
		}); err != nil {
			slog.Error("restrict: failed to create operation ban", "error", err, "user_id", uid.String(), "operation", op)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

			return
		}
	}

	// Set user status to 'restricted' — only when the user is currently
	// 'active' or already 'restricted'. A 'banned', 'suspended', or
	// 'deleted' user keeps their status; the operation bans still apply
	// on top, and lifting them must not resurrect the account.
	status, serr := h.Queries.GetUserStatus(ctx, uid)
	if serr != nil {
		// pgx.ErrNoRows (user doesn't exist) maps to 404 via
		// HandleServiceError.
		errpkg.HandleServiceError(w, serr)
		return
	}
	if status == "active" || status == "restricted" {
		if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
			Status: "restricted",
			ID:     uid,
		}); err != nil {
			slog.Error("restrict: failed to update user status", "error", err, "user_id", uid.String())
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to restrict user")

			return
		}
	}

	// Invalidate caches so the new restrictions take effect immediately.
	h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid), banCacheKey(uid))

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	op := chi.URLParam(r, "operation")
	if !validOperations[op] {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_operation", "Invalid operation: "+op)
		return
	}

	if err := h.Queries.LiftOperationBan(r.Context(), gen.LiftOperationBanParams{
		UserID:    uid,
		Operation: gen.BanOperation(op),
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restriction")
		return
	}

	// If no active restrictions remain, restore user status to 'active' —
	// but only when the current status was set by a restriction/suspension.
	// A 'banned' or 'deleted' user must not be reactivated by lifting an
	// operation ban. On a count/status failure the status is left as-is
	// rather than optimistically restoring access the user may not be
	// entitled to.
	remaining, err := h.Queries.CountActiveOperationBans(r.Context(), uid)
	if err != nil {
		slog.Error("lift restriction: failed to count active operation bans",
			"error", err, "user_id", uid.String())
	} else if remaining == 0 {
		status, serr := h.Queries.GetUserStatus(r.Context(), uid)
		if serr != nil {
			slog.Error("lift restriction: failed to read user status", "error", serr, "user_id", uid.String())
		} else if status == "restricted" || status == "suspended" {
			// Lifting the last op ban of a suspended user is effectively an
			// unsuspend — clear the 'suspension' moderation-action row too or
			// ban enforcement keeps blocking the reactivated account.
			if status == "suspended" {
				if err := h.Queries.SoftDeleteUserSuspension(r.Context(), uid); err != nil {
					slog.Error("lift restriction: failed to clear suspension record", "error", err, "user_id", uid.String())
				}
			}
			if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
				Status: "active",
				ID:     uid,
			}); err != nil {
				slog.Error("lift restriction: failed to restore user status", "error", err, "user_id", uid.String())
			}
		}
	}

	h.invalidateUserAccessCaches(r.Context(), uid, restrictionsCacheKey(uid))

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "operation": op, "lifted": true})
}

// LiftAllRestrictions handles DELETE /admin/users/{id}/restrict.
// Lifts all operation bans for a user.
func (h *Handlers) LiftAllRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	if err := h.Queries.LiftAllOperationBans(r.Context(), uid); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to lift restrictions")
		return
	}

	// Restore user status to 'active' when all restrictions are lifted —
	// only for 'restricted'/'suspended' users. Lifting restrictions must
	// not reactivate a 'banned' or 'deleted' account. A suspended user
	// also carries a 'suspension' user_moderation_actions row that the
	// ban-enforcement middleware reads — it must be removed or the
	// "unsuspended" user stays blocked.
	status, serr := h.Queries.GetUserStatus(r.Context(), uid)
	if serr != nil {
		slog.Error("lift all restrictions: failed to read user status", "error", serr, "user_id", uid.String())
	} else if status == "restricted" || status == "suspended" {
		if status == "suspended" {
			if err := h.Queries.SoftDeleteUserSuspension(r.Context(), uid); err != nil {
				slog.Error("lift all restrictions: failed to clear suspension record", "error", err, "user_id", uid.String())
			}
		}
		if err := h.Queries.UpdateUserStatus(r.Context(), gen.UpdateUserStatusParams{
			Status: "active",
			ID:     uid,
		}); err != nil {
			slog.Error("lift all restrictions: failed to restore user status", "error", err, "user_id", uid.String())
		}
	}

	h.invalidateUserAccessCaches(r.Context(), uid, restrictionsCacheKey(uid))

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"user_id": uid.String(), "lifted_all": true})
}

// ListRestrictions handles GET /admin/users/{id}/restrictions.
// Lists active operation bans for a user.
func (h *Handlers) ListRestrictions(w http.ResponseWriter, r *http.Request) {
	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	rows, err := h.Queries.ListActiveOperationBans(r.Context(), uid)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list restrictions")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
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

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input SuspendUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	ctx := r.Context()

	// Refuse to suspend a banned or deleted account — a later unsuspend
	// would restore status 'active' and silently bypass the ban/deletion.
	// GetUserStatus doubles as a 404 check for a non-existent user.
	curStatus, serr := h.Queries.GetUserStatus(ctx, uid)
	if serr != nil {
		errpkg.HandleServiceError(w, serr)
		return
	}
	if curStatus == "banned" || curStatus == "deleted" {
		errpkg.WriteError(w, http.StatusConflict, "invalid_status_transition",
			"Cannot suspend a "+curStatus+" user")
		return
	}

	// 1. Create suspension record.
	if err := h.Queries.CreateUserSuspension(ctx, gen.CreateUserSuspensionParams{
		UserID: uid,

		Reason: &input.Reason,
	}); err != nil {
		slog.Error("suspend: failed to create suspension record", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to suspend user")

		return
	}

	// 2. Ban from all operations permanently.
	for _, op := range middleware.AllOperations() {
		if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
			UserID: uid,

			Operation: gen.BanOperation(op),
			Reason:    &input.Reason, Column4: 0, // permanent
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update user status")

		return
	}

	// 4. Invalidate all caches.
	h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid), banCacheKey(uid))

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
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

// validViolationTypes mirrors the violation types served by
// GET /admin/users/config (including 'other', which maps to no
// auto-restriction operations).
var validViolationTypes = map[string]bool{
	"spam":           true,
	"harassment":     true,
	"misinformation": true,
	"hate_speech":    true,
	"other":          true,
}

// WarnUserTyped handles POST /admin/users/{id}/warn with violation_type.
// After 3 warnings of the same type, auto-restricts the related operations for 7 days.
func (h *Handlers) WarnUserTyped(w http.ResponseWriter, r *http.Request) {
	_, ok := requireUser(w, r)
	if !ok {
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	var input WarnUserTypedRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.ViolationType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation", "violation_type is required")
		return
	}

	// Whitelist the canonical violation types served by GET
	// /admin/users/config so arbitrary strings can't be written into the
	// moderation record.
	if _, ok := validViolationTypes[input.ViolationType]; !ok {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_violation_type", "Invalid violation_type")
		return
	}

	ctx := r.Context()

	// 404 (not a FK-violation 500) when the target user does not exist.
	if _, err := h.Queries.GetUserStatus(ctx, uid); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	// Create typed warning.
	if err := h.Queries.CreateUserTypedWarning(ctx, gen.CreateUserTypedWarningParams{
		UserID: uid,

		Reason:        &input.Reason,
		ViolationType: &input.ViolationType,
	}); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to issue warning")
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
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to issue warning")

		return
	}

	autoRestricted := false

	if count >= 3 {
		// Auto-escalate: restrict from related operations for 7 days.
		ops := violationToOperations[input.ViolationType]
		for _, op := range ops {
			if err := h.Queries.CreateOperationBan(ctx, gen.CreateOperationBanParams{
				UserID: uid,

				Operation: gen.BanOperation(op),
				Reason:    strPtr("Auto-restricted after 3 " + input.ViolationType + " warnings"), Column4: 604800, // 7 days in seconds
			}); err != nil {
				slog.Error("warn: failed to auto-restrict operation",
					"error", err, "user_id", uid.String(), "operation", op)
			}
		}

		autoRestricted = len(ops) > 0

		// Mirror RestrictUser: flip status to 'restricted' so the account
		// state reflects the new operation bans — but never downgrade a
		// 'banned', 'suspended', or 'deleted' user.
		status, serr := h.Queries.GetUserStatus(ctx, uid)
		if serr != nil {
			slog.Error("warn: failed to read user status", "error", serr, "user_id", uid.String())
		} else if status == "active" || status == "restricted" {
			if err := h.Queries.UpdateUserStatus(ctx, gen.UpdateUserStatusParams{
				Status: "restricted",
				ID:     uid,
			}); err != nil {
				slog.Error("warn: failed to mark user restricted", "error", err, "user_id", uid.String())
			}
		}

		h.invalidateUserAccessCaches(ctx, uid, restrictionsCacheKey(uid))
		slog.Warn("warn: auto-escalation triggered",
			"user_id", uid.String(),
			"violation_type", input.ViolationType,
			"count", count,
			"operations", ops)
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"user_id":         uid.String(),
			"violation_type":  input.ViolationType,
			"warning_count":   count,
			"auto_restricted": autoRestricted,
		},
	})
}

func strPtr(s string) *string { return &s }

// banCacheKey returns the Valkey key holding a user's cached ban state.
func banCacheKey(uid pgtype.UUID) string { return valkey.MustKey("ban:%s", uid.String()) }

// restrictionsCacheKey returns the Valkey key holding a user's cached
// per-operation restrictions.
func restrictionsCacheKey(uid pgtype.UUID) string {
	return valkey.MustKey("restrictions:%s", uid.String())
}

// invalidateUserAccessCaches drops the cached RBAC permission set plus the
// given Valkey keys for a user after a moderation action. Failures are logged
// rather than returned — the write is already committed — but they are never
// dropped silently, since a stale cache lets a restricted user keep acting.
// Both the L1 in-memory resolver cache (via h.RBAC) and the L2 Valkey entry
// are evicted so the ban/suspension takes effect immediately in-process.
func (h *Handlers) invalidateUserAccessCaches(ctx context.Context, uid pgtype.UUID, keys ...string) {
	// L1: in-process resolver cache. Always evicted when the resolver is
	// wired, even when Valkey is unavailable.
	if h.RBAC != nil {
		h.RBAC.InvalidateUserCache(ctx, uid)
	}

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
