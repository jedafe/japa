package menus

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/rbac"
	"github.com/jedafe/japa/internal/tenant"
)

type Handlers struct {
	Service     *Service
	RBACService *rbac.RBAC
}

// NewHandlers constructs menus Handlers without RBAC permission resolution.
// Prefer NewHandlersWithPerms for the public resolver path so authenticated
// users are gated by permission_node per ADR-054.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// NewHandlersWithPerms constructs menus Handlers wired to the RBAC resolver.
// The public GET /menus/{slug} endpoint uses this to prune items whose
// permission_node is not held by the requesting user (ADR-054).
func NewHandlersWithPerms(service *Service, rbacSvc *rbac.RBAC) *Handlers {
	h := NewHandlers(service)
	h.RBACService = rbacSvc
	return h
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "Tenant context not available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func parseUUIDParam(r *http.Request, name string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}

func parsePageSize(r *http.Request) int32 {
	v := r.URL.Query().Get("page_size")
	if v == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > int(MaxPageSize) {
		return MaxPageSize
	}
	return int32(n)
}

func parsePageOffset(r *http.Request) int32 {
	v := r.URL.Query().Get("page_offset")
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrMenuNotFound), errors.Is(err, ErrMenuItemNotFound):
		writeError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidPlacement), errors.Is(err, ErrInvalidVisibility),
		errors.Is(err, ErrInvalidStatus), errors.Is(err, ErrInvalidTarget):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, ErrDuplicateSlug):
		writeError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ── Admin Menu CRUD ─────────────────────────────────────────────────────

func (h *Handlers) ListMenus(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	q := r.URL.Query().Get("q")
	placement := r.URL.Query().Get("placement")
	status := r.URL.Query().Get("status")
	sort := r.URL.Query().Get("sort")
	sortDir := r.URL.Query().Get("sort_dir")
	if sortDir != "desc" {
		sortDir = "asc"
	}
	result, err := h.Service.ListMenus(r.Context(), tenantID, q, placement, status, sort, sortDir, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) GetMenu(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu ID")
		return
	}
	menu, err := h.Service.GetMenu(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": menu})
}

func (h *Handlers) CreateMenu(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input struct {
		Slug      string `json:"slug"`
		Name      string `json:"name"`
		Placement string `json:"placement"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Slug == "" || input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "slug and name are required")
		return
	}
	menu, err := h.Service.CreateMenu(r.Context(), tenantID, input.Slug, input.Name, input.Placement, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": menu})
}

func (h *Handlers) UpdateMenu(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu ID")
		return
	}
	var input struct {
		Name      string `json:"name"`
		Placement string `json:"placement"`
		Status    string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}
	menu, err := h.Service.UpdateMenu(r.Context(), id, tenantID, input.Name, input.Placement, input.Status)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": menu})
}

func (h *Handlers) DeleteMenu(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu ID")
		return
	}
	if err := h.Service.DeleteMenu(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ── Admin Menu Item CRUD ────────────────────────────────────────────────

func (h *Handlers) CreateMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	menuID, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu ID")
		return
	}
	var input CreateMenuItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	// menu_id comes from the URL path (/admin/menus/{id}/items), not the body.
	input.MenuID = menuID.String()
	if input.Label == "" || input.URL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "label and url are required")
		return
	}
	item, err := h.Service.CreateMenuItem(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": item})
}

func (h *Handlers) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "itemId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu item ID")
		return
	}
	var input UpdateMenuItemInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if input.Label == "" || input.URL == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "label and url are required")
		return
	}
	item, err := h.Service.UpdateMenuItem(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": item})
}

func (h *Handlers) DeleteMenuItem(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "itemId")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid menu item ID")
		return
	}
	if err := h.Service.DeleteMenuItem(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (h *Handlers) ReorderMenuItems(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	var input ReorderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}
	if len(input.Items) == 0 {
		writeError(w, http.StatusBadRequest, "validation_error", "items array is required")
		return
	}
	if err := h.Service.ReorderMenuItems(r.Context(), tenantID, input.Items); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ── Public Resolution ───────────────────────────────────────────────────

func (h *Handlers) GetMenuBySlug(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	if slug == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "slug is required")
		return
	}

	isAuthenticated := false
	userGroupIDs := map[string]bool{}
	var userID pgtype.UUID

	user := auth.FromContext(r.Context())
	if user != nil {
		isAuthenticated = true
		userID = user.UserID
		// Resolve the user's group memberships for group-based menu item gating.
		groupIDs, err := h.Service.GetUserGroupIDs(r.Context(), tenantID, user.UserID.String())
		if err == nil {
			for _, gid := range groupIDs {
				userGroupIDs[gid] = true
			}
		}
	}

	menu, err := h.Service.GetMenuBySlug(r.Context(), tenantID, slug, userGroupIDs, isAuthenticated, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": menu})
}
