package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// SetOwnerEmail wires the JC_OWNER_EMAIL designation (§03 owner amendment).
func (h *Handlers) SetOwnerEmail(email string) {
	h.ownerEmail = strings.ToLower(strings.TrimSpace(email))
}

// isOwnerEmail reports whether the given account email is the configured
// platform owner.
func (h *Handlers) isOwnerEmail(email string) bool {
	return auth.IsOwnerEmail(email, h.ownerEmail)
}

// callerIsOwner resolves the authenticated caller's email and reports whether
// it is the configured owner. Fails closed (false) on lookup error.
func (h *Handlers) callerIsOwner(r *http.Request) bool {
	u := auth.FromContext(r.Context())
	if u == nil || !u.UserID.Valid {
		return false
	}

	email, err := h.Queries.GetUserEmailByID(r.Context(), u.UserID)
	if err != nil {
		return false
	}

	return h.isOwnerEmail(email)
}

// guardSuperAdminTarget enforces the §03 owner amendment on lifecycle
// mutations of a super_admin-grade account:
//   - the owner account is immutable — always 403 owner_protected
//   - a non-owner super_admin target may only be mutated by the owner —
//     otherwise 403 owner_required
//
// A target-email lookup failure aborts the mutation — the owner check must
// never be skipped silently. Returns true when the mutation may proceed.
func (h *Handlers) guardSuperAdminTarget(w http.ResponseWriter, r *http.Request, target pgtype.UUID) bool {
	email, err := h.Queries.GetUserEmailByID(r.Context(), target)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errpkg.WriteError(w, http.StatusNotFound, "user_not_found", "User not found")
		} else {
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to verify target account")
		}

		return false
	}

	if h.isOwnerEmail(email) {
		errpkg.WriteError(w, http.StatusForbidden, "owner_protected", "The owner account cannot be modified")
		return false
	}

	if h.RBAC != nil && h.RBAC.IsSuperAdmin(r.Context(), target) && !h.callerIsOwner(r) {
		errpkg.WriteError(w, http.StatusForbidden, "owner_required",
			"Only the owner can manage super_admin accounts")
		return false
	}

	return true
}
