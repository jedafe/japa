package sidebars

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the sidebar admin CRUD and public resolution endpoints.
type Handlers struct {
	Service *Service
}

// NewHandlers is an exported function.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return pgtype.UUID{}, true
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

	return int32(n) // #nosec
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

	return int32(n) // #nosec
}

func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSidebarNotFound), errors.Is(err, ErrSidebarWidgetNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, ErrInvalidApp), errors.Is(err, ErrInvalidPosition),
		errors.Is(err, ErrInvalidVisibility), errors.Is(err, ErrInvalidStatus),
		errors.Is(err, ErrInvalidWidgetType), errors.Is(err, ErrInvalidPagePattern),
		errors.Is(err, ErrWidgetTypeNotAllowedForApp):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	case errors.Is(err, ErrDuplicateSlug):
		errpkg.WriteError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, errpkg.ErrRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		errpkg.HandleServiceError(w, err)
	}
}

// ── Widget Types ────────────────────────────────────────────────────────

// ListWidgetTypes handles GET /api/v1/sidebars/widget-types.
// Returns the sidebar widget type registry. Public (no auth required) so the
// admin UI can load it without a separate auth check.
func (h *Handlers) ListWidgetTypes(w http.ResponseWriter, r *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"widget_types": GetWidgetTypes()}})
}

// SidebarOption is a single value/label pair used by the admin UI selectors.
type SidebarOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// GetSidebarOptions handles GET /api/v1/admin/sidebars/options.
// Returns the canonical app, position, visibility and status values (Rule 16)
// so the admin UI can populate selects and filters without hardcoding them.
func (h *Handlers) GetSidebarOptions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.requireUser(w, r); !ok {
		return
	}

	apps := []SidebarOption{
		{Value: string(AppMain), Label: "Main"},
		{Value: string(AppDocs), Label: "Docs"},
		{Value: string(AppLearn), Label: "Learn"},
	}
	positions := []SidebarOption{
		{Value: string(PositionLeft), Label: "Left"},
		{Value: string(PositionRight), Label: "Right"},
	}
	visibilities := []SidebarOption{
		{Value: string(VisibilityAll), Label: "All"},
		{Value: string(VisibilityAuthenticated), Label: "Authenticated"},
		{Value: string(VisibilityGuest), Label: "Guest"},
	}
	statuses := []SidebarOption{
		{Value: string(StatusActive), Label: "Active"},
		{Value: string(StatusInactive), Label: "Inactive"},
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"apps":         apps,
		"positions":    positions,
		"visibilities": visibilities,
		"statuses":     statuses,
	}})
}

// ── Admin Sidebar CRUD ──────────────────────────────────────────────────

// ListSidebars handles GET /api/v1/admin/sidebars.
func (h *Handlers) ListSidebars(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageOffset(r)
	app := r.URL.Query().Get("app")
	position := r.URL.Query().Get("position")
	status := r.URL.Query().Get("status")
	q := r.URL.Query().Get("q")
	sortParam := r.URL.Query().Get("sort")
	dir := r.URL.Query().Get("dir")
	// Compose sort into "key:dir" format for the SQL CASE WHEN matcher.
	// Only forward the dir suffix when both sort and dir are present and valid.
	sortKey := ""
	if sortParam != "" {
		sortKey = sortParam
		if dir == "asc" || dir == "desc" {
			sortKey = sortParam + ":" + dir
		}
	}

	result, err := h.Service.ListSidebars(r.Context(), app, position, status, q, sortKey, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetSidebar handles GET /api/v1/admin/sidebars/{id}.
func (h *Handlers) GetSidebar(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	sidebar, err := h.Service.GetSidebar(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": sidebar})
}

// CreateSidebar handles POST /api/v1/admin/sidebars.
func (h *Handlers) CreateSidebar(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input CreateSidebarInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.Slug == "" || input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "slug and name are required")
		return
	}

	if input.Visibility == "" {
		input.Visibility = "all"
	}

	if input.Status == "" {
		input.Status = "active"
	}

	sidebar, err := h.Service.CreateSidebar(r.Context(), tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": genSidebarToJSON(sidebar)})
}

// UpdateSidebar handles PATCH /api/v1/admin/sidebars/{id}.
func (h *Handlers) UpdateSidebar(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	var input UpdateSidebarInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	sidebar, err := h.Service.UpdateSidebar(r.Context(), id, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": genSidebarToJSON(sidebar)})
}

// DeleteSidebar handles DELETE /api/v1/admin/sidebars/{id}.
func (h *Handlers) DeleteSidebar(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	if err := h.Service.DeleteSidebar(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ── Admin Sidebar Widget CRUD ───────────────────────────────────────────

// CreateSidebarWidget handles POST /api/v1/admin/sidebars/{id}/widgets.
func (h *Handlers) CreateSidebarWidget(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	sidebarID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	var input CreateSidebarWidgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if input.WidgetType == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "widget_type is required")
		return
	}

	if input.Visibility == "" {
		input.Visibility = "all"
	}

	if input.Status == "" {
		input.Status = "active"
	}

	widget, err := h.Service.CreateSidebarWidget(r.Context(), tenantID, sidebarID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": genSidebarWidgetToJSON(widget)})
}

// UpdateSidebarWidget handles PATCH /api/v1/admin/sidebars/{id}/widgets/{widgetId}.
func (h *Handlers) UpdateSidebarWidget(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	sidebarID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	id, err := parseUUIDParam(r, "widgetId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid widget ID")
		return
	}

	var input UpdateSidebarWidgetInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	widget, err := h.Service.UpdateSidebarWidget(r.Context(), id, sidebarID, tenantID, input)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": genSidebarWidgetToJSON(widget)})
}

// DeleteSidebarWidget handles DELETE /api/v1/admin/sidebars/{id}/widgets/{widgetId}.
func (h *Handlers) DeleteSidebarWidget(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	sidebarID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	id, err := parseUUIDParam(r, "widgetId")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid widget ID")
		return
	}

	if err := h.Service.DeleteSidebarWidget(r.Context(), id, sidebarID, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}

// ReorderSidebarWidgets handles POST /api/v1/admin/sidebars/{id}/widgets/reorder.
func (h *Handlers) ReorderSidebarWidgets(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	sidebarID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid sidebar ID")
		return
	}

	var input ReorderInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	if len(input.Items) == 0 {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "items array is required")
		return
	}

	if err := h.Service.ReorderSidebarWidgets(r.Context(), sidebarID, tenantID, input.Items); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "reordered"}})
}

// ── Public Resolution ───────────────────────────────────────────────────

// Resolve handles GET /api/v1/sidebars/resolve?app=&position=&path=.
func (h *Handlers) Resolve(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	app := r.URL.Query().Get("app")
	position := r.URL.Query().Get("position")
	requestPath := r.URL.Query().Get("path")

	if app == "" || position == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "app and position query params are required")
		return
	}

	if requestPath == "" {
		requestPath = "/"
	}

	isAuthenticated := false
	userGroupIDs := map[string]bool{}

	var userID pgtype.UUID

	user := auth.FromContext(r.Context())
	if user != nil {
		isAuthenticated = true
		userID = user.UserID

		groupIDs, err := h.Service.GetUserGroupIDs(r.Context(), user.UserID.String())
		if err == nil {
			for _, gid := range groupIDs {
				userGroupIDs[gid] = true
			}
		}
	}

	result, err := h.Service.Resolve(r.Context(), tenantID, app, position, requestPath, userGroupIDs, isAuthenticated, userID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}
