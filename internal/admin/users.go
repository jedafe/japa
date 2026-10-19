package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
)

// CreateUserRequest is the input for creating a user from the admin panel.
// MakeSuperAdmin grants the new account super_admin membership; only the
// platform owner (JC_OWNER_EMAIL) may create super_admin accounts.
type CreateUserRequest struct {
	Email          string `json:"email"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	DisplayName    string `json:"display_name,omitempty"`
	MakeSuperAdmin bool   `json:"make_super_admin,omitempty"`
}

// CreateUser handles POST /admin/users — creates a new user account directly.
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	var input CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Username = strings.TrimSpace(input.Username)

	// §03 owner amendment: creating a super_admin account is owner-only.
	if input.MakeSuperAdmin && !h.callerIsOwner(r) {
		errpkg.WriteError(w, http.StatusForbidden, "owner_required", "Only the owner can create super_admin accounts")
		return
	}

	if input.Email == "" || input.Username == "" || input.Password == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation", "email, username, and password are required")
		return
	}

	// §50 username contract — same charset rule as public registration.
	if !auth.ValidateUsername(input.Username) {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_username",
			"Username must be 3-30 characters of letters, numbers, and underscores")
		return
	}

	if err := auth.ValidatePassword(input.Password, auth.DefaultPasswordPolicy()); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "password_policy", err.Error())
		return
	}

	// §03 alias amendment: a canonical username must not shadow another
	// user's public_username alias — the DB unique index only covers the
	// username column itself.
	if taken, terr := h.Queries.PublicHandleTaken(r.Context(), gen.PublicHandleTakenParams{
		Handle: input.Username,
	}); terr == nil && taken > 0 {
		errpkg.WriteError(w, http.StatusConflict, "username_exists", "A user with this username already exists")
		return
	}

	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "password_error", "Failed to hash password")
		return
	}

	var userID pgtype.UUID

	userID, err = h.Queries.CreateUser(r.Context(), gen.CreateUserParams{
		Email:        input.Email,
		Username:     input.Username,
		PasswordHash: &passwordHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError

		isDuplicate := errors.Is(err, errpkg.ErrDuplicate) || (errors.As(err, &pgErr) && pgErr.Code == "23505")
		if isDuplicate {
			// users has unique indexes on both email and username —
			// distinguish them via the violated constraint name (same
			// convention as the public register handler).
			if pgErr != nil && strings.Contains(pgErr.ConstraintName, "username") {
				errpkg.WriteError(w, http.StatusConflict, "username_exists", "A user with this username already exists")
				return
			}

			errpkg.WriteError(w, http.StatusConflict, "email_exists", "A user with this email already exists")

			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "create_error", "Failed to create user")

		return
	}

	// Persist the optional display_name — the field lives on user_profiles,
	// not users. Non-fatal: the account is usable without it.
	if input.DisplayName != "" {
		if err := h.Queries.SetUserDisplayName(r.Context(), gen.SetUserDisplayNameParams{
			UserID:      userID,
			DisplayName: &input.DisplayName,
		}); err != nil {
			slog.Warn("admin create user: failed to set display_name", "error", err, "user_id", userID.String())
		}
	}

	// §unified-tiers: admin-created users start on the regular tier.
	if h.TierManager != nil {
		if err := h.TierManager.SetUserTier(r.Context(), userID, "regular"); err != nil {
			slog.Error("admin create user: failed to set regular tier", "error", err, "user_id", userID.String())
			errpkg.WriteError(w, http.StatusInternalServerError, "tier_assignment_failed", "Failed to initialize user tier")

			return
		}
	}

	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		UserID:       userID,
		PasswordHash: passwordHash,
	}); err != nil {
		// Non-fatal: password history is a security enhancement, not a blocker.
	}

	// §50 Pre-seed default bookmark folders. Non-fatal: logged on failure.
	if h.DefaultFolderSetter != nil {
		if err := h.DefaultFolderSetter.CreateDefaultFoldersForUser(r.Context(), userID); err != nil {
			slog.Warn("admin create user: failed to create default bookmark folders", "error", err)
		}
	}

	// Owner-gated super_admin grant (§03 owner amendment).
	if input.MakeSuperAdmin {
		if err := h.promoteToSuperAdmin(r.Context(), userID); err != nil {
			slog.Error("admin create user: failed to grant super_admin", "error", err, "user_id", userID.String())
			errpkg.WriteError(w, http.StatusInternalServerError, "super_admin_grant_failed", "User created but super_admin grant failed")
			return
		}
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{
		"data": map[string]any{
			"id":       userID.String(),
			"email":    input.Email,
			"username": input.Username,
		},
	})
}

// promoteToSuperAdmin adds a user to the super_admin group and invalidates
// their permission cache (§03 owner amendment).
func (h *Handlers) promoteToSuperAdmin(ctx context.Context, userID pgtype.UUID) error {
	group, err := h.Queries.GetUserGroupBySlug(ctx, rbac.SystemGroupSuperAdmin)
	if err != nil {
		return err
	}

	if err := h.Queries.AddUserToGroup(ctx, gen.AddUserToGroupParams{
		UserID:  userID,
		GroupID: group.ID,
	}); err != nil {
		return err
	}

	if h.RBAC != nil {
		h.RBAC.InvalidateUserCache(ctx, userID)
	}

	return nil
}

// HardDeleteUser handles DELETE /admin/users/{id}/hard — permanently erases
// a soft-deleted user and all cascading data. Only works on users with status=deleted.
func (h *Handlers) HardDeleteUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	if h.GDPR == nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "not_configured", "Hard delete is not configured")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	// §03 owner amendment: owner immutable; super_admin targets are owner-only.
	if !h.guardSuperAdminTarget(w, r, userID) {
		return
	}

	// Verify the user is soft-deleted before allowing hard delete.
	user, err := h.Queries.GetUserByID(r.Context(),

		userID,
	)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}

	if user.Status != "deleted" {
		errpkg.WriteError(w, http.StatusBadRequest, "not_deleted", "User must be soft-deleted before hard delete")
		return
	}

	if err := h.GDPR.HardDeleteUser(r.Context(), userID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "hard_delete_error", "Failed to hard delete user")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      userID.String(),
		"status":  "hard_deleted",
		"message": "User permanently erased",
	})
}
