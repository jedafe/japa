// Package mentorship — handlers.go provides HTTP handlers for mentorship admin CRUD.
//
// Routes:
//
//	GET    /admin/mentorship/programs          — list programs
//	GET    /admin/mentorship/programs/{id}     — get program
//	POST   /admin/mentorship/programs          — create program
//	PATCH  /admin/mentorship/programs/{id}     — update program
//	GET    /admin/mentorship/matches           — list mentor-mentee matches
//	PATCH  /admin/mentorship/matches/{id}      — update match status
package mentorship

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/httpx"
	"github.com/jedafe/japa/internal/tenant"
)

const (
	defaultPageSize int32 = 20
	maxPageSize     int32 = 100
)

// Handlers holds dependencies for mentorship admin HTTP handlers.
type Handlers struct {
	Service *Service
}

// NewHandlers creates Handlers with the given Service.
func NewHandlers(service *Service) *Handlers {
	return &Handlers{Service: service}
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		httpx.WriteError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}

	return t.ID, true
}

func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > int(maxPageSize) {
		return maxPageSize
	}

	return int32(n) //nolint:gosec // G109: bounded by MaxPageSize check above
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Programs
// ---------------------------------------------------------------------------

// ListPrograms returns all mentorship programs for the tenant.
// GET /admin/mentorship/programs.
func (h *Handlers) ListPrograms(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := httpx.ParseOffset(r)

	programs, err := h.Service.ListPrograms(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing mentorship programs", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list programs")

		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": programs})
}

// GetProgram returns a single mentorship program by ID.
// GET /admin/mentorship/programs/{id}.
func (h *Handlers) GetProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	program, err := h.Service.GetProgram(r.Context(), tenantID, id)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "not_found", "Program not found")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// CreateProgram creates a new mentorship program.
// POST /admin/mentorship/programs.
func (h *Handlers) CreateProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	program, err := h.Service.CreateProgram(r.Context(), tenantID, input.Name, input.Description)
	if err != nil {
		slog.Error("creating mentorship program", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create program")

		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"data": program})
}

// UpdateProgram updates an existing mentorship program.
// PATCH /admin/mentorship/programs/{id}.
func (h *Handlers) UpdateProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	var input struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	program, err := h.Service.UpdateProgram(r.Context(), tenantID, id, input.Name, input.Description, input.IsActive)
	if err != nil {
		slog.Error("updating mentorship program", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update program")

		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Matches
// ---------------------------------------------------------------------------

// ListMatches returns all mentor-mentee matches for the tenant.
// GET /admin/mentorship/matches.
func (h *Handlers) ListMatches(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := httpx.ParseOffset(r)

	matches, err := h.Service.ListMatches(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing mentorship matches", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list matches")

		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": matches})
}

// UpdateMatch updates a mentor-mentee match status.
// PATCH /admin/mentorship/matches/{id}.
func (h *Handlers) UpdateMatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid match ID")
		return
	}

	var input struct {
		Status *string `json:"status,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Status == nil {
		httpx.WriteError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	match, err := h.Service.UpdateMatch(r.Context(), tenantID, id, *input.Status)
	if err != nil {
		slog.Error("updating mentorship match", "error", err)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update match")

		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"data": match})
}
