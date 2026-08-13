// Package personas — handlers.go provides the admin HTTP handlers for the
// AI Persona System management routes.
//
// Routes (Section 68, ADR-068):
//
//	GET   /admin/personas           — paginated list (?status=&q=&sort=&dir=&page_size=&page_offset=)
//	POST  /admin/personas           — mass provisioning (JSON array body)
//	GET   /admin/personas/{id}      — detail with activity_policy
//	PATCH /admin/personas/{id}      — update activity_policy, modes, status, voice_prompt
//	GET   /admin/personas/{id}/activity — paginated action log
//
// Ref: japaconnect-engineering/sections/68-ai-personas.md
//
//	japaconnect-engineering/adr/ADR-068-ai-persona-system.md
package personas

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers exposes the admin persona management endpoints.
type Handlers struct {
	service *Service
}

// NewHandlers creates persona handlers bound to the provided service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers (same pattern as university/handlers.go, rag/handlers.go)
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("personas: encode response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

func requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return u, true
}

func scanUUIDParam(w http.ResponseWriter, r *http.Request, name string) (pgtype.UUID, bool) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, name)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid "+name)
		return pgtype.UUID{}, false
	}
	return id, true
}

// parsePagination extracts page_size and page_offset from the query string
// per api-response-format.md (BUG-002: convention drift — previously used
// limit/offset/page). The legacy limit/offset/page params are still accepted
// as deprecated aliases so existing callers don't break, but page_size /
// page_offset take precedence.
func parsePagination(r *http.Request) (int32, int32) {
	limit, offset := int32(20), int32(0)
	q := r.URL.Query()

	// page_size is the canonical param; limit is a deprecated alias.
	if v := q.Get("page_size"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	} else if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = int32(n)
		}
	}

	// page_offset is the canonical param; offset is a deprecated alias.
	if v := q.Get("page_offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	} else if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = int32(n)
		}
	} else if v := q.Get("page"); v != "" {
		// page (1-based) is a deprecated alias; convert to offset.
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			offset = (int32(n) - 1) * limit
		}
	}
	return limit, offset
}

func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// HandleList handles GET /admin/personas — paginated list with optional
// status filter, search query, and sort. Gated by persona.view permission
// (checked by RBAC middleware at the route registration site).
//
// Query params (BUG-002/BUG-003):
//   - status: filter by persona status (active|paused|retired); empty = all
//   - q: search by profile name or backing username (ILIKE)
//   - sort: sort key (name|created_at|total_actions); empty = created_at
//   - dir: sort direction (asc|desc); empty = desc
//   - page_size / page_offset: canonical pagination (api-response-format.md)
func (h *Handlers) HandleList(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	q := r.URL.Query()
	status := q.Get("status")
	search := q.Get("q")
	sort := q.Get("sort")
	dir := q.Get("dir")
	// Default sort to created_at DESC when not specified.
	if sort == "" {
		sort = "created_at"
	}
	if dir == "" {
		dir = "desc"
	}
	// Whitelist sort keys to prevent SQL injection via the CASE expressions.
	switch sort {
	case "name", "created_at", "total_actions":
	default:
		sort = "created_at"
	}
	switch dir {
	case "asc", "desc":
	default:
		dir = "desc"
	}

	limit, offset := parsePagination(r)

	items, total, err := h.service.ListPersonas(r.Context(), tenantID, status, search, sort, dir, limit, offset)
	if err != nil {
		slog.Error("personas: list failed", "error", err)
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": items,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}

// HandleCreate handles POST /admin/personas — mass provisioning. Accepts a
// JSON array of persona inputs. The entire batch is provisioned in ONE
// transaction; if any persona fails, nothing is committed (ADR-068 §7).
// Gated by persona.manage permission.
func (h *Handlers) HandleCreate(w http.ResponseWriter, r *http.Request) {
	user, ok := requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	var inputs []PersonaInput
	if err := json.NewDecoder(r.Body).Decode(&inputs); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body — expected a JSON array")
		return
	}
	if len(inputs) == 0 {
		writeError(w, http.StatusBadRequest, "empty_batch", "At least one persona is required")
		return
	}

	results, err := h.service.ProvisionPersonas(r.Context(), tenantID, user.UserID, inputs)
	if err != nil {
		// Distinguish validation errors (400 with specific message) from
		// provisioning failures (422 generic). Validation errors are wrapped
		// with errpkg.ErrInvalid in the service layer (BUG-013).
		if errors.Is(err, errpkg.ErrInvalid) {
			writeError(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		slog.Error("personas: provisioning failed", "error", err, "admin_user_id", user.UserID.String())
		// Standard error envelope per api-response-format.md (BUG-002).
		// The transaction is all-or-nothing, so no personas were committed.
		writeError(w, http.StatusUnprocessableEntity, "provisioning_failed",
			"Mass provisioning failed — no personas were committed (transaction rolled back)")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data":    results,
		"meta":    map[string]int{"count": len(results)},
	})
}

// HandleGet handles GET /admin/personas/{id} — persona detail with
// activity_policy. Gated by persona.view permission.
func (h *Handlers) HandleGet(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	detail, err := h.service.GetPersona(r.Context(), tenantID, id)
	if err != nil {
		slog.Error("personas: get failed", "error", err, "id", id.String())
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": detail})
}

// HandleUpdate handles PATCH /admin/personas/{id} — update activity_policy,
// modes, status, voice_prompt. Gated by persona.manage permission.
func (h *Handlers) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var body struct {
		DisclosureMode *string         `json:"disclosure_mode"`
		AutonomyMode   *string         `json:"autonomy_mode"`
		VoicePrompt    *string         `json:"voice_prompt"`
		ActivityPolicy *ActivityPolicy `json:"activity_policy"`
		Status         *string         `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid request body")
		return
	}

	in := UpdatePersonaInput{
		DisclosureMode: body.DisclosureMode,
		AutonomyMode:   body.AutonomyMode,
		VoicePrompt:    body.VoicePrompt,
		ActivityPolicy: body.ActivityPolicy,
		Status:         body.Status,
	}

	persona, err := h.service.UpdatePersona(r.Context(), tenantID, id, in)
	if err != nil {
		slog.Error("personas: update failed", "error", err, "id", id.String())
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": persona})
}

// HandleActivity handles GET /admin/personas/{id}/activity — paginated
// action log (platform_events for the persona's backing user).
// Gated by persona.view permission.
func (h *Handlers) HandleActivity(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireUser(w, r); !ok {
		return
	}
	tenantID, ok := requireTenant(w, r)
	if !ok {
		return
	}

	if h.service == nil {
		writeError(w, http.StatusServiceUnavailable, "personas_unavailable", "Persona service not configured")
		return
	}

	id, ok := scanUUIDParam(w, r, "id")
	if !ok {
		return
	}

	limit, offset := parsePagination(r)

	entries, total, err := h.service.GetPersonaActivity(r.Context(), tenantID, id, limit, offset)
	if err != nil {
		slog.Error("personas: activity failed", "error", err, "id", id.String())
		handleServiceError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": entries,
		"meta": map[string]int64{"total": total, "limit": int64(limit), "offset": int64(offset)},
	})
}
