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

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("json encode error", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
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
	return int32(n)
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return int32(n)
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Programs
// ---------------------------------------------------------------------------

// ListPrograms returns all mentorship programs for the tenant.
// GET /admin/mentorship/programs
func (h *Handlers) ListPrograms(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	programs, err := h.Service.ListPrograms(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing mentorship programs", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list programs")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": programs})
}

// GetProgram returns a single mentorship program by ID.
// GET /admin/mentorship/programs/{id}
func (h *Handlers) GetProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	program, err := h.Service.GetProgram(r.Context(), tenantID, id)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "Program not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": program})
}

// CreateProgram creates a new mentorship program.
// POST /admin/mentorship/programs
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
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	program, err := h.Service.CreateProgram(r.Context(), tenantID, input.Name, input.Description)
	if err != nil {
		slog.Error("creating mentorship program", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to create program")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{"data": program})
}

// UpdateProgram updates an existing mentorship program.
// PATCH /admin/mentorship/programs/{id}
func (h *Handlers) UpdateProgram(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	var input struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	program, err := h.Service.UpdateProgram(r.Context(), tenantID, id, input.Name, input.Description, input.IsActive)
	if err != nil {
		slog.Error("updating mentorship program", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update program")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": program})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Matches
// ---------------------------------------------------------------------------

// ListMatches returns all mentor-mentee matches for the tenant.
// GET /admin/mentorship/matches
func (h *Handlers) ListMatches(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parseOffset(r)

	matches, err := h.Service.ListMatches(r.Context(), tenantID, limit, offset)
	if err != nil {
		slog.Error("listing mentorship matches", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to list matches")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": matches})
}

// UpdateMatch updates a mentor-mentee match status.
// PATCH /admin/mentorship/matches/{id}
func (h *Handlers) UpdateMatch(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid match ID")
		return
	}

	var input struct {
		Status *string `json:"status,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Status == nil {
		writeError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	match, err := h.Service.UpdateMatch(r.Context(), tenantID, id, *input.Status)
	if err != nil {
		slog.Error("updating mentorship match", "error", err)
		writeError(w, http.StatusInternalServerError, "internal_error", "Failed to update match")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"data": match})
}
