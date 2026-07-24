package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/httpx"
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
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	input.Email = strings.TrimSpace(strings.ToLower(input.Email))
	input.Username = strings.TrimSpace(input.Username)

	if input.Email == "" || input.Username == "" || input.Password == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation", "email, username, and password are required")
		return
	}

	if err := auth.ValidatePassword(input.Password, auth.DefaultPasswordPolicy()); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "password_policy", err.Error())
		return
	}

	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "password_error", "Failed to hash password")
		return
	}

	var userID pgtype.UUID
	userID, err = h.Queries.CreateUser(r.Context(), gen.CreateUserParams{
		TenantID:     tenantID,
		Email:        input.Email,
		Username:     input.Username,
		PasswordHash: &passwordHash,
	})
	if err != nil {
		if errors.Is(err, errpkg.ErrDuplicate) {
			httpx.WriteError(w, http.StatusConflict, "email_exists", "A user with this email already exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "create_error", "Failed to create user")
		return
	}

	if err := h.Queries.InsertPasswordHistory(r.Context(), gen.InsertPasswordHistoryParams{
		UserID:       userID,
		PasswordHash: passwordHash,
	}); err != nil {
		// Non-fatal: password history is a security enhancement, not a blocker.
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
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
		httpx.WriteError(w, http.StatusInternalServerError, "not_configured", "Hard delete is not configured")
		return
	}

	var userID pgtype.UUID
	if err := userID.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid user ID")
		return
	}

	// Verify the user is soft-deleted before allowing hard delete.
	user, err := h.Queries.GetUserByID(r.Context(), gen.GetUserByIDParams{
		TenantID: tenantID,
		UserID:   userID,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "User not found")
		return
	}
	if user.Status != "deleted" {
		httpx.WriteError(w, http.StatusBadRequest, "not_deleted", "User must be soft-deleted before hard delete")
		return
	}

	if err := h.GDPR.HardDeleteUser(r.Context(), userID, tenantID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "hard_delete_error", "Failed to hard delete user")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"id":      userID.String(),
		"status":  "hard_deleted",
		"message": "User permanently erased",
	})
}
