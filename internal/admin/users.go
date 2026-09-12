package admin

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// CreateUserRequest is the input for creating a user from the admin panel.
type CreateUserRequest struct {
	Email       string `json:"email"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name,omitempty"`
}

// CreateUser handles POST /admin/users — creates a new user account directly.
func (h *Handlers) CreateUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Username = strings.TrimSpace(input.Username)

	if input.Email == "" || input.Username == "" || input.Password == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation", "email, username, and password are required")
		return
	}

	if err := auth.ValidatePassword(input.Password, auth.DefaultPasswordPolicy()); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "password_policy", err.Error())
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
		if errors.Is(err, errpkg.ErrDuplicate) {
			errpkg.WriteError(w, http.StatusConflict, "email_exists", "A user with this email already exists")
			return
		}

		errpkg.WriteError(w, http.StatusInternalServerError, "create_error", "Failed to create user")

		return
	}

	// §unified-tiers: admin-created users start on the regular tier.
	if h.TierManager != nil {
		if err := h.TierManager.SetUserTier(r.Context(), tenantID, userID, "regular"); err != nil {
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
		if err := h.DefaultFolderSetter.CreateDefaultFoldersForUser(r.Context(), tenantID, userID); err != nil {
			slog.Warn("admin create user: failed to create default bookmark folders", "error", err)
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

// HardDeleteUser handles DELETE /admin/users/{id}/hard — permanently erases
// a soft-deleted user and all cascading data. Only works on users with status=deleted.
func (h *Handlers) HardDeleteUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}

	tenantID, ok := requireTenant(w, r)
	if !ok {
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

	if err := h.GDPR.HardDeleteUser(r.Context(), userID, tenantID); err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "hard_delete_error", "Failed to hard delete user")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      userID.String(),
		"status":  "hard_deleted",
		"message": "User permanently erased",
	})
}
