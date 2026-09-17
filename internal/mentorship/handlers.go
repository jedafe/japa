// Package mentorship — handlers.go provides HTTP handlers for mentorship
// admin CRUD (legacy programs/matches) and §102 Mentorship Matching routes
// (discovery, profile, connections, comparison, admin override).
//
// Legacy admin routes:
//
//	GET    /admin/mentorship/programs          — list programs
//	GET    /admin/mentorship/programs/{id}     — get program
//	POST   /admin/mentorship/programs          — create program
//	PATCH  /admin/mentorship/programs/{id}     — update program
//	DELETE /admin/mentorship/programs/{id}     — delete program
//	GET    /admin/mentorship/matches           — list mentor-mentee matches
//	PATCH  /admin/mentorship/matches/{id}      — update match status
//
// §102 routes:
//
//	GET    /mentors                            — discover mentors
//	GET    /mentors/{username}                 — mentor public profile
//	GET    /compare/mentors                    — compare up to 4 mentors
//	GET    /me/mentor-profile                  — get own mentor settings
//	PATCH  /me/mentor-profile                  — toggle mentor / set capacity/areas/bio
//	POST   /mentorship/request                 — send mentorship request
//	GET    /me/mentorship/outgoing             — my sent requests
//	DELETE /me/mentorship/outgoing/{id}        — cancel pending request
//	GET    /me/mentorship/incoming             — incoming requests
//	PATCH  /me/mentorship/incoming/{id}/accept — accept request
//	PATCH  /me/mentorship/incoming/{id}/decline— decline request
//	PATCH  /mentorship/{id}/complete           — complete connection
//	PATCH  /mentorship/{id}/cancel             — cancel active connection
//	GET    /admin/mentorship/connections       — all connections (admin)
//	PATCH  /admin/mentorship/connections/{id}  — admin override
package mentorship

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/pgvector/pgvector-go"
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

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

func parseOffset(r *http.Request) int32 {
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			if n > math.MaxInt32 {
				return math.MaxInt32
			}

			// #nosec G109 // value bounded by MaxPageSize check
			return int32(n) //nolint:gosec // G109: bounded by MaxInt32 check above
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Programs
// ---------------------------------------------------------------------------

// ListPrograms returns all mentorship programs for the platform.
// GET /admin/mentorship/programs.
func (h *Handlers) ListPrograms(w http.ResponseWriter, r *http.Request) {

	limit := parsePageSize(r)
	offset := parseOffset(r)

	programs, err := h.Service.ListPrograms(r.Context(), limit, offset)
	if err != nil {
		slog.Error("listing mentorship programs", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list programs")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": programs})
}

// GetProgram returns a single mentorship program by ID.
// GET /admin/mentorship/programs/{id}.
func (h *Handlers) GetProgram(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	program, err := h.Service.GetProgram(r.Context(), id)
	if err != nil {
		errpkg.WriteError(w, http.StatusNotFound, "not_found", "Program not found")
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// CreateProgram creates a new mentorship program.
// POST /admin/mentorship/programs.
func (h *Handlers) CreateProgram(w http.ResponseWriter, r *http.Request) {

	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		IsActive    *bool  `json:"is_active,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "name is required")
		return
	}

	isActive := true
	if input.IsActive != nil {
		isActive = *input.IsActive
	}

	program, err := h.Service.CreateProgram(r.Context(), input.Name, input.Description, isActive)
	if err != nil {
		slog.Error("creating mentorship program", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create program")

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": program})
}

// UpdateProgram updates an existing mentorship program.
// PATCH /admin/mentorship/programs/{id}.
func (h *Handlers) UpdateProgram(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	var input struct {
		Name        *string `json:"name,omitempty"`
		Description *string `json:"description,omitempty"`
		IsActive    *bool   `json:"is_active,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	program, err := h.Service.UpdateProgram(r.Context(), id, input.Name, input.Description, input.IsActive)
	if err != nil {
		slog.Error("updating mentorship program", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update program")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": program})
}

// ---------------------------------------------------------------------------
// Admin Handlers — Mentorship Matches
// ---------------------------------------------------------------------------

// ListMatches returns all mentor-mentee matches for the platform.
// GET /admin/mentorship/matches.
func (h *Handlers) ListMatches(w http.ResponseWriter, r *http.Request) {

	limit := parsePageSize(r)
	offset := parseOffset(r)

	matches, err := h.Service.ListMatches(r.Context(), limit, offset)
	if err != nil {
		slog.Error("listing mentorship matches", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list matches")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": matches})
}

// UpdateMatch updates a mentor-mentee match status.
// PATCH /admin/mentorship/matches/{id}.
func (h *Handlers) UpdateMatch(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid match ID")
		return
	}

	var input struct {
		Status *string `json:"status,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Status == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	match, err := h.Service.UpdateMatch(r.Context(), id, *input.Status)
	if err != nil {
		slog.Error("updating mentorship match", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update match")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": match})
}

// DeleteProgram deletes a mentorship program.
// DELETE /admin/mentorship/programs/{id}.
func (h *Handlers) DeleteProgram(w http.ResponseWriter, r *http.Request) {

	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid program ID")
		return
	}

	if err := h.Service.DeleteProgram(r.Context(), id); err != nil {
		if errors.Is(err, ErrProgramNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Program not found")
			return
		}

		slog.Error("deleting mentorship program", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to delete program")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"deleted": true}})
}

// =====================================================================
// §102 Mentorship Matching handlers (ADR-110)
// =====================================================================

// requireAuthUser extracts the authenticated user ID from the request context.
func (h *Handlers) requireAuthUser(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	u := auth.FromContext(r.Context())
	if u == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return pgtype.UUID{}, false
	}

	return u.UserID, true
}

// parsePage parses page and page_size query params, returning limit and offset.
func parsePage(r *http.Request) (int32, int32) {
	limit := parsePageSize(r)

	page := 1

	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}

	// #nosec G115 // value bounded by caller
	offset := int32((page - 1) * int(limit))
	if offset > math.MaxInt32 {
		offset = math.MaxInt32
	}

	return limit, offset
}

// ListMentors discovers mentors with optional destination and area filters.
// GET /api/v1/mentors?dest=&areas=&sort=match|newest|capacity&page=
// When sort=match and the user is authenticated with a profile embedding,
// uses pgvector cosine similarity for bio-based match scoring (§102 ADR-110).
// When dest is empty and the user is authenticated, injects the user's
// profile destination as the default filter (§102 Geo Awareness).
func (h *Handlers) ListMentors(w http.ResponseWriter, r *http.Request) {

	limit, offset := parsePage(r)

	var destSpaceID pgtype.UUID
	if v := r.URL.Query().Get("dest"); v != "" {
		if err := destSpaceID.Scan(v); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	var areas []string
	if v := r.URL.Query().Get("areas"); v != "" {
		areas = strings.Split(v, ",")
	}

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "newest"
	}

	// §102 Geo Awareness: when dest is empty and the user is authenticated,
	// inject the user's profile destination as the default filter.
	if !destSpaceID.Valid {
		if u := auth.FromContext(r.Context()); u != nil {
			if defaultDest, _ := h.Service.GetMenteeDestinationSpaceID(r.Context(), u.UserID); defaultDest.Valid {
				destSpaceID = defaultDest
			}
		}
	}

	// §102 match scoring: when sort=match and the user is authenticated,
	// fetch the mentee's embedding for pgvector cosine similarity. Falls
	// back to keyword-only scoring when no embedding is available.
	var menteeEmbedding *pgvector.Vector

	if sort == "match" {
		if u := auth.FromContext(r.Context()); u != nil {
			menteeEmbedding, _ = h.Service.GetMenteeEmbedding(r.Context(), u.UserID)
		}
	}

	mentors, total, err := h.Service.ListMentors(r.Context(), destSpaceID, areas, sort, menteeEmbedding, limit, offset)
	if err != nil {
		slog.Error("listing mentors", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list mentors")

		return
	}

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}
	totalPages := 1
	if limit > 0 {
		totalPages = (total + int(limit) - 1) / int(limit)
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": mentors,
		"meta": map[string]any{
			"total":       total,
			"page":        page,
			"limit":       limit,
			"total_pages": totalPages,
		},
	})
}

// GetMentorByUsername returns a mentor's public profile.
// GET /api/v1/mentors/{username}.
func (h *Handlers) GetMentorByUsername(w http.ResponseWriter, r *http.Request) {

	username := chi.URLParam(r, "username")
	if username == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_username", "Username is required")
		return
	}

	mentor, err := h.Service.GetMentorByUsername(r.Context(), username)
	if err != nil {
		if errors.Is(err, ErrMentorNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "mentor_not_found", "Mentor not found")
			return
		}

		slog.Error("getting mentor by username", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get mentor")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": mentor})
}

// CompareMentors returns up to 4 mentors for the comparison endpoint.
// GET /api/v1/compare/mentors?ids=a,b,c,d.
func (h *Handlers) CompareMentors(w http.ResponseWriter, r *http.Request) {

	idsStr := r.URL.Query().Get("ids")
	if idsStr == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_ids", "ids parameter is required")
		return
	}

	idStrs := strings.Split(idsStr, ",")
	if len(idStrs) > 4 {
		errpkg.WriteError(w, http.StatusBadRequest, "too_many_ids", "Maximum 4 mentors can be compared")
		return
	}

	userIDs := make([]pgtype.UUID, 0, len(idStrs))

	for _, s := range idStrs {
		var id pgtype.UUID
		if err := id.Scan(strings.TrimSpace(s)); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid mentor ID: "+s)
			return
		}

		userIDs = append(userIDs, id)
	}

	mentors, err := h.Service.GetMentorsByIDs(r.Context(), userIDs)
	if err != nil {
		slog.Error("comparing mentors", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to compare mentors")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": mentors})
}

// GetMyMentorProfile returns the current user's mentor profile settings.
// GET /api/v1/me/mentor-profile.
func (h *Handlers) GetMyMentorProfile(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	profile, err := h.Service.GetMentorProfile(r.Context(), userID)
	if err != nil {
		slog.Error("getting mentor profile", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to get mentor profile")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// UpdateMyMentorProfile toggles is_mentor and sets capacity/areas/bio.
// PATCH /api/v1/me/mentor-profile.
func (h *Handlers) UpdateMyMentorProfile(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var input struct {
		IsMentor       *bool    `json:"is_mentor,omitempty"`
		MentorCapacity *int16   `json:"mentor_capacity,omitempty"`
		MentorAreas    []string `json:"mentor_areas,omitempty"`
		MentorBio      *string  `json:"mentor_bio,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	profile, err := h.Service.UpdateMentorProfile(r.Context(), userID,
		input.IsMentor, input.MentorCapacity, input.MentorAreas, input.MentorBio)
	if err != nil {
		if errors.Is(err, ErrCapacityExceeded) {
			errpkg.WriteError(w, http.StatusBadRequest, "capacity_exceeded",
				"mentor_capacity exceeds the platform max_capacity setting")
			return
		}

		slog.Error("updating mentor profile", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update mentor profile")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": profile})
}

// RequestMentorship creates a new mentorship request.
// POST /api/v1/mentorship/request.
func (h *Handlers) RequestMentorship(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var input struct {
		MentorID           string  `json:"mentor_id"`
		Goals              *string `json:"goals,omitempty"`
		DestinationSpaceID *string `json:"destination_space_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.MentorID == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "mentor_id is required")
		return
	}

	var mentorID pgtype.UUID
	if err := mentorID.Scan(input.MentorID); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_mentor_id", "Invalid mentor ID")
		return
	}

	var destSpaceID pgtype.UUID
	if input.DestinationSpaceID != nil && *input.DestinationSpaceID != "" {
		if err := destSpaceID.Scan(*input.DestinationSpaceID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_dest", "Invalid destination space ID")
			return
		}
	}

	conn, err := h.Service.CreateConnection(r.Context(), mentorID, userID, input.Goals, destSpaceID)
	if err != nil {
		switch {
		case errors.Is(err, ErrCannotSelfMentor):
			errpkg.WriteError(w, http.StatusBadRequest, "cannot_self_mentor", "Cannot request mentorship from yourself")
		case errors.Is(err, ErrDuplicateRequest):
			errpkg.WriteError(w, http.StatusConflict, "duplicate_request",
				"A pending request to this mentor already exists")
		default:
			slog.Error("creating mentorship connection", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to create mentorship request")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": conn})
}

// ListOutgoingConnections lists requests sent by the current user (as mentee).
// GET /api/v1/me/mentorship/outgoing.
func (h *Handlers) ListOutgoingConnections(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	limit, offset := parsePage(r)
	status := r.URL.Query().Get("status")

	conns, err := h.Service.ListOutgoingConnections(r.Context(), userID, status, limit, offset)
	if err != nil {
		slog.Error("listing outgoing connections", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list outgoing connections")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conns})
}

// CancelOutgoingConnection cancels a pending request by the mentee.
// DELETE /api/v1/me/mentorship/outgoing/{id}.
func (h *Handlers) CancelOutgoingConnection(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	if err := h.Service.CancelOutgoingConnection(r.Context(), userID, connID); err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found or not pending")
			return
		}

		slog.Error("cancelling outgoing connection", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cancel connection")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"cancelled": true}})
}

// ListIncomingConnections lists requests received by the current user (as mentor).
// GET /api/v1/me/mentorship/incoming.
func (h *Handlers) ListIncomingConnections(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	limit, offset := parsePage(r)
	status := r.URL.Query().Get("status")

	conns, err := h.Service.ListIncomingConnections(r.Context(), userID, status, limit, offset)
	if err != nil {
		slog.Error("listing incoming connections", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list incoming connections")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conns})
}

// AcceptConnection accepts a pending mentorship request.
// Only the mentor of the connection can accept.
// PATCH /api/v1/me/mentorship/incoming/{id}/accept.
func (h *Handlers) AcceptConnection(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	conn, err := h.Service.AcceptConnection(r.Context(), userID, connID)
	if err != nil {
		switch {
		case errors.Is(err, ErrConnectionNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found or not pending")
		case errors.Is(err, ErrNotAuthorized):
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Only the mentor can accept this request")
		default:
			slog.Error("accepting connection", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to accept connection")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conn})
}

// DeclineConnection declines a pending mentorship request.
// Only the mentor of the connection can decline.
// PATCH /api/v1/me/mentorship/incoming/{id}/decline.
func (h *Handlers) DeclineConnection(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	conn, err := h.Service.DeclineConnection(r.Context(), userID, connID)
	if err != nil {
		switch {
		case errors.Is(err, ErrConnectionNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found or not pending")
		case errors.Is(err, ErrNotAuthorized):
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Only the mentor can decline this request")
		default:
			slog.Error("declining connection", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to decline connection")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conn})
}

// CompleteConnection completes an active connection.
// Either party (mentor or mentee) can complete.
// PATCH /api/v1/mentorship/{id}/complete.
func (h *Handlers) CompleteConnection(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	conn, err := h.Service.CompleteConnection(r.Context(), userID, connID)
	if err != nil {
		switch {
		case errors.Is(err, ErrConnectionNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found or not active")
		case errors.Is(err, ErrNotAuthorized):
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Only a participant can complete this connection")
		default:
			slog.Error("completing connection", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to complete connection")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conn})
}

// CancelConnection cancels an active connection.
// Either party (mentor or mentee) can cancel.
// PATCH /api/v1/mentorship/{id}/cancel.
func (h *Handlers) CancelConnection(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.requireAuthUser(w, r)
	if !ok {
		return
	}

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	var input struct {
		CancelReason *string `json:"cancel_reason,omitempty"`
	}

	// Body is optional for cancel.
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
			return
		}
	}

	conn, err := h.Service.CancelConnection(r.Context(), userID, connID, input.CancelReason)
	if err != nil {
		switch {
		case errors.Is(err, ErrConnectionNotFound):
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found or not active")
		case errors.Is(err, ErrNotAuthorized):
			errpkg.WriteError(w, http.StatusForbidden, "forbidden", "Only a participant can cancel this connection")
		default:
			slog.Error("cancelling connection", "error", err)
			errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to cancel connection")
		}

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conn})
}

// ListAllConnections lists all connections for admin.
// GET /api/v1/admin/mentorship/connections.
func (h *Handlers) ListAllConnections(w http.ResponseWriter, r *http.Request) {

	limit, offset := parsePage(r)
	status := r.URL.Query().Get("status")

	var mentorID, menteeID pgtype.UUID
	if v := r.URL.Query().Get("mentor_id"); v != "" {
		_ = mentorID.Scan(v)
	}

	if v := r.URL.Query().Get("mentee_id"); v != "" {
		_ = menteeID.Scan(v)
	}

	conns, err := h.Service.ListAllConnections(r.Context(), status, mentorID, menteeID, limit, offset)
	if err != nil {
		slog.Error("listing all connections", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to list connections")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conns})
}

// AdminUpdateConnection allows an admin to override a connection's status.
// PATCH /api/v1/admin/mentorship/connections/{id}.
func (h *Handlers) AdminUpdateConnection(w http.ResponseWriter, r *http.Request) {

	var connID pgtype.UUID
	if err := connID.Scan(chi.URLParam(r, "id")); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid connection ID")
		return
	}

	var input struct {
		Status *string `json:"status,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_json", "Invalid JSON body")
		return
	}

	if input.Status == nil {
		errpkg.WriteError(w, http.StatusBadRequest, "validation_error", "status is required")
		return
	}

	conn, err := h.Service.AdminUpdateConnection(r.Context(), connID, *input.Status)
	if err != nil {
		if errors.Is(err, ErrConnectionNotFound) {
			errpkg.WriteError(w, http.StatusNotFound, "not_found", "Connection not found")
			return
		}

		slog.Error("admin updating connection", "error", err)
		errpkg.WriteError(w, http.StatusInternalServerError, "internal_error", "Failed to update connection")

		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": conn})
}
