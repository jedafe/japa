// handlers.go — HTTP handlers for the user-facing appeal routes (§22).
//
//	POST /api/v1/appeals    — submit an appeal (auth)
//	GET  /api/v1/me/appeals — appellant's own appeals (auth)
//
// Admin resolution lives on PATCH /admin/appeals/{id} (internal/admin),
// which delegates to Service.Resolve.
package appeals

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
)

// Handlers exposes the appeal submission routes.
type Handlers struct {
	service *Service
}

// NewHandlers creates appeal handlers bound to the service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// maxBodyBytes bounds the appeal payload — the reason field caps at 4000
// runes, so 64 KiB is generous headroom for the JSON envelope.
const maxBodyBytes = 64 << 10

func parsePageParams(r *http.Request) (limit, offset int32) {
	limit = 20

	// Convention params (page_size/page_offset) win; legacy page/limit
	// spelling is still accepted for older clients.
	if v := r.URL.Query().Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n) // #nosec
		}
	}

	if v := r.URL.Query().Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n) // #nosec
		}
	} else if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			offset = (int32(n) - 1) * limit // #nosec
		}
	}

	return limit, offset
}

// writeAppealError maps service sentinels to the standard error envelope.
func writeAppealError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrReasonRequired):
		errpkg.WriteError(w, http.StatusBadRequest, "reason_required", "An appeal reason is required")
	case errors.Is(err, ErrUnknownSubject):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_entity_type", "This entity type cannot be appealed")
	case errors.Is(err, ErrNotEligible):
		errpkg.WriteError(w, http.StatusForbidden, "not_eligible", "This decision is not eligible for appeal, or you are not the affected party")
	case errors.Is(err, ErrDuplicate):
		errpkg.WriteError(w, http.StatusConflict, "duplicate_appeal", "You already have a pending appeal for this decision")
	case errors.Is(err, ErrInvalidStatus):
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_status", "Status must be upheld, overturn, or dismissed")
	case errors.Is(err, ErrAlreadyResolved):
		errpkg.WriteError(w, http.StatusConflict, "already_resolved", "This appeal has already been resolved")
	case errors.Is(err, pgx.ErrNoRows):
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Not found")
	default:
		errpkg.WriteError(w, http.StatusInternalServerError, "appeal_failed", "Failed to process appeal")
	}
}

// HandleSubmit handles POST /appeals — body: { entity_type, entity_id, reason }.
func (h *Handlers) HandleSubmit(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	var input struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
		Reason     string `json:"reason"`
	}

	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	appeal, err := h.service.Submit(r.Context(), user.UserID, input.EntityType, input.EntityID, input.Reason)
	if err != nil {
		writeAppealError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": appeal})
}

// HandleListMine handles GET /me/appeals — the appellant's own appeals.
func (h *Handlers) HandleListMine(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	limit, offset := parsePageParams(r)

	appeals, err := h.service.ListMine(r.Context(), user.UserID, limit, offset)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list appeals")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": appeals,
		"meta": map[string]any{"page_size": limit, "page_offset": offset},
	})
}

// HandleListEligible handles GET /me/appeals/eligible — every decision the
// caller can currently appeal, across all registered subjects (§22).
func (h *Handlers) HandleListEligible(w http.ResponseWriter, r *http.Request) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return
	}

	items, err := h.service.Eligible(r.Context(), user.UserID)
	if err != nil {
		errpkg.WriteError(w, http.StatusInternalServerError, "list_failed", "Failed to list appealable decisions")
		return
	}

	if items == nil {
		items = []EligibleItem{}
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}
