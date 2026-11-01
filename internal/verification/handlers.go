package verification

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers holds the verification HTTP handlers.
type Handlers struct {
	service *Service
}

// NewHandlers creates Handlers bound to the given service.
func NewHandlers(s *Service) *Handlers {
	return &Handlers{service: s}
}

func requireAuth(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, err
	}
	return u, nil
}

func parseInt32(s string) int32 {
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	// #nosec G109 G115 -- bounded by MaxPageSize checks at call site
	return int32(n)
}

type submitRequest struct {
	EntityType       string  `json:"entity_type"`
	OrganizationID   *string `json:"organization_id"`
	RequestedType    *string `json:"requested_type"`
	FormSubmissionID string  `json:"form_submission_id"`
}

// SubmitRequest handles POST /verification-requests.
// Ref: japaconnect-engineering/sections/07-organization-profiles-claim-ownership.md (Verification Requests).
func (h *Handlers) SubmitRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	var req submitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}
	if req.FormSubmissionID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "form_submission_required", "form_submission_id is required")
		return
	}

	result, err := h.service.SubmitRequest(r.Context(), user.UserID, SubmitInput{
		EntityType:       req.EntityType,
		OrganizationID:   req.OrganizationID,
		RequestedType:    req.RequestedType,
		FormSubmissionID: req.FormSubmissionID,
	})
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// ListMyRequests handles GET /me/verification-requests.
func (h *Handlers) ListMyRequests(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	result, err := h.service.ListMyRequests(r.Context(), user.UserID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}
	if result == nil {
		result = []gen.ListVerificationRequestsByRequesterRow{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// GetRequest handles GET /verification-requests/{id} (requester-scoped).
func (h *Handlers) GetRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid request ID")
		return
	}

	result, err := h.service.GetRequest(r.Context(), id, user.UserID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// WithdrawRequest handles DELETE /verification-requests/{id}.
func (h *Handlers) WithdrawRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid request ID")
		return
	}

	if err := h.service.WithdrawRequest(r.Context(), id, user.UserID); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id.String(), "status": StatusWithdrawn}})
}

// GetOrgPendingRequest handles GET /organizations/{id}/verification-request —
// returns the org's pending request for owner/admin pre-checks (§07).
func (h *Handlers) GetOrgPendingRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	orgID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid organization ID")
		return
	}

	row, err := h.service.GetPendingForOrgScoped(r.Context(), orgID, user.UserID)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": row.String()}})
}

// ListTypes handles GET /verification-requests/types — the active badge-type
// vocabulary from verification_types (001013). ?scope=organization|user|
// service_listing filters; absent returns all scopes' active rows. Response:
// {data:{types:[{id,scope,value,label,description?,badge_media_id?,
// badge_url?,position,is_active,is_protected}]}}.
func (h *Handlers) ListTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.ListTypes(r.Context(), r.URL.Query().Get("scope"))
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to load verification types")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"types": types}})
}

// ---- /admin/verification-types (verification.types.manage, 001013) ----

// AdminListTypes handles GET /admin/verification-types?scope= — all rows,
// active and inactive, for the taxonomy manager.
func (h *Handlers) AdminListTypes(w http.ResponseWriter, r *http.Request) {
	types, err := h.service.AdminListTypes(r.Context(), r.URL.Query().Get("scope"))
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to load verification types")
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// AdminCreateType handles POST /admin/verification-types.
func (h *Handlers) AdminCreateType(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Scope        string  `json:"scope"`
		Value        string  `json:"value"`
		Label        string  `json:"label"`
		Description  *string `json:"description"`
		BadgeMediaID *string `json:"badge_media_id"`
		Position     *int32  `json:"position"`
		IsActive     *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	dto, err := h.service.AdminCreateType(r.Context(), TypeInput{
		Scope: body.Scope, Value: body.Value, Label: body.Label,
		Description: body.Description, BadgeMediaID: body.BadgeMediaID,
		Position: body.Position, IsActive: body.IsActive,
	})
	if err != nil {
		writeTypeError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": dto})
}

// AdminUpdateType handles PATCH /admin/verification-types/{id}. scope+value
// are immutable — entity rows store the value verbatim.
func (h *Handlers) AdminUpdateType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid verification type ID")
		return
	}
	var body struct {
		Label        string  `json:"label"`
		Description  *string `json:"description"`
		BadgeMediaID *string `json:"badge_media_id"`
		Position     *int32  `json:"position"`
		IsActive     *bool   `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid request body")
		return
	}
	dto, err := h.service.AdminUpdateType(r.Context(), id, TypeInput{
		Label: body.Label, Description: body.Description,
		BadgeMediaID: body.BadgeMediaID, Position: body.Position, IsActive: body.IsActive,
	})
	if err != nil {
		writeTypeError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": dto})
}

// AdminDeleteType handles DELETE /admin/verification-types/{id}.
func (h *Handlers) AdminDeleteType(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid verification type ID")
		return
	}
	if err := h.service.AdminDeleteType(r.Context(), id); err != nil {
		writeTypeError(w, err)
		return
	}
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// writeTypeError maps taxonomy errors to the standard error contract.
func writeTypeError(w http.ResponseWriter, err error) {
	var inputErr *TypeInputError
	switch {
	case errors.As(err, &inputErr):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_input", inputErr.Msg)
	case errors.Is(err, ErrTypeExists):
		errpkg.WriteError(w, http.StatusConflict, "verification_type_exists", "A type with this value already exists for the scope")
	case errors.Is(err, ErrTypeProtected):
		errpkg.WriteError(w, http.StatusConflict, "verification_type_protected", "Seeded types cannot be deleted — deactivate instead")
	case errors.Is(err, ErrTypeNotFound):
		errpkg.WriteError(w, http.StatusNotFound, "verification_type_not_found", "Verification type not found")
	case errors.Is(err, ErrInvalidTypeScope):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_scope", "scope must be one of organization|user|service_listing")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Verification type operation failed")
	}
}

// AdminListRequests handles GET /admin/verification-requests.
func (h *Handlers) AdminListRequests(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r); !ok {
		return
	}

	q := r.URL.Query()
	page := parseInt32(q.Get("page"))
	pageSize := parseInt32(q.Get("page_size"))

	rows, total, err := h.service.AdminList(r.Context(), q.Get("status"), q.Get("entity_type"), q.Get("q"), q.Get("sort"), q.Get("sort_dir"), page, pageSize)
	if err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}
	if rows == nil {
		rows = []gen.ListVerificationRequestsRow{}
	}
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if page <= 0 {
		page = 1
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": rows,
		"meta": map[string]any{
			"total":      total,
			"page":       page,
			"page_size":  pageSize,
			"page_count": (total + int64(pageSize) - 1) / int64(pageSize),
		},
	})
}

// ReviewRequest handles PATCH /admin/verification-requests/{id}.
func (h *Handlers) ReviewRequest(w http.ResponseWriter, r *http.Request) {
	user, ok := requireAuth(w, r)
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid request ID")
		return
	}

	var req struct {
		Status         string `json:"status"`
		ResolutionNote string `json:"resolution_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	if err := h.service.ReviewRequest(r.Context(), id, user.UserID, ReviewInput{
		Status:         req.Status,
		ResolutionNote: req.ResolutionNote,
	}); err != nil {
		errpkg.HandleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"id": id.String(), "status": req.Status}})
}
