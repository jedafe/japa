package gates

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// SuperAdminChecker is satisfied by *rbac.RBAC (§123: gate access requires
// super_admin group membership, not a delegable permission node).
type SuperAdminChecker interface {
	IsSuperAdmin(ctx context.Context, userID pgtype.UUID) bool
}

// Handlers serves the /admin/access-gates routes.
type Handlers struct {
	svc  *Service // nil = feature disabled
	rbac SuperAdminChecker
}

// NewHandlers builds handlers. svc may be nil when JC_COOLIFY_*/JC_ACCESS_GATES
// are unset — requests then answer 503 feature_disabled (still super-admin
// gated, so the disabled state is not disclosed to non-admins).
func NewHandlers(svc *Service, r SuperAdminChecker) *Handlers {
	return &Handlers{svc: svc, rbac: r}
}

// HandleList is GET /admin/access-gates.
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r) {
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"gates": h.svc.List(r.Context())})
}

// HandleToggle is POST /admin/access-gates/{key}/toggle.
func (h *Handlers) HandleToggle(w http.ResponseWriter, r *http.Request) {
	if !h.allow(w, r) {
		return
	}
	st, err := h.svc.Toggle(r.Context(), chi.URLParam(r, "key"))
	if err != nil {
		if IsUnknownKey(err) {
			errpkg.WriteError(w, http.StatusNotFound, "gate_not_found", "Unknown access gate")
			return
		}
		errpkg.WriteError(w, http.StatusBadGateway, "gate_backend_error", "The infrastructure provider rejected the operation")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, st)
}

// allow enforces auth + super_admin group membership (§123) and the
// feature-enabled check. Returns false after writing the error response.
func (h *Handlers) allow(w http.ResponseWriter, r *http.Request) bool {
	user := auth.FromContext(r.Context())
	if user == nil || !user.UserID.Valid {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Authentication required")
		return false
	}
	if h.rbac == nil || !h.rbac.IsSuperAdmin(r.Context(), user.UserID) {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Super-admin access required")
		return false
	}
	if h.svc == nil {
		errpkg.WriteError(w, http.StatusServiceUnavailable, "feature_disabled", "Access gates are not configured on this deployment")
		return false
	}
	return true
}
