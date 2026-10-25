package verification

import (
	"encoding/json"
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

// ListTypes handles GET /verification-requests/types — the allowed badge-type
// vocabulary (public; Rule 16 so frontends never hardcode the enum).
func (h *Handlers) ListTypes(w http.ResponseWriter, _ *http.Request) {
	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"types": VerificationTypes}})
}

// AdminListRequests handles GET /admin/verification-requests.
func (h *Handlers) AdminListRequests(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r); !ok {
		return
	}

	q := r.URL.Query()
	page := parseInt32(q.Get("page"))
	pageSize := parseInt32(q.Get("page_size"))

	rows, total, err := h.service.AdminList(r.Context(), q.Get("status"), q.Get("entity_type"), page, pageSize)
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
