// Package tickets — HTTP handlers for the Ticket System (Section 38).
//
// Every handler is a method on Handlers so dependencies (Service) are
// injected via the constructor, never read from globals. Responses follow
// the JapaConnect API envelope: success bodies are wrapped in {"data": ...}
// and errors in {"error":{"code":"...","message":"..."}}. Permission
// enforcement is handled by middleware wired at the router level.
package tickets

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	"github.com/jedafe/japa/internal/tenant"
)

// Handlers holds dependencies for ticket HTTP handlers.
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

// parseUUIDParam extracts a URL parameter and scans it into a pgtype.UUID.
func parseUUIDParam(r *http.Request, param string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(chi.URLParam(r, param)); err != nil {
		return pgtype.UUID{}, errors.New("invalid " + param)
	}
	return id, nil
}

// requireTenant resolves the tenant from the request context.
func (h *Handlers) requireTenant(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	t := tenant.FromContext(r.Context())
	if t == nil {
		writeError(w, http.StatusServiceUnavailable, "tenant_unavailable", "No tenant available")
		return pgtype.UUID{}, false
	}
	return t.ID, true
}

// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}
	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		writeError(w, http.StatusNotFound, "not_found", "Resource not found")
	case strings.Contains(err.Error(), "already_exists"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "duplicate"):
		writeError(w, http.StatusConflict, "already_exists", "Resource already exists")
	case strings.Contains(err.Error(), "invalid status transition"):
		writeError(w, http.StatusBadRequest, "invalid_transition", err.Error())
	case strings.Contains(err.Error(), "required"):
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "Something went wrong")
	}
}

// writeJSON serializes the given value as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.Warn("json encode error", "error", err)
		}
	}
}

// writeError writes a JSON error response following the JapaConnect API format.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return DefaultPageSize
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return DefaultPageSize
	}
	if n > MaxPageSize {
		return MaxPageSize
	}
	return int32(n)
}

// parsePageSizeOffset extracts the page_offset query parameter.
func parsePageSizeOffset(r *http.Request) int32 {
	s := r.URL.Query().Get("page_offset")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return int32(n)
}

// ---------------------------------------------------------------------------
// User Portal Handlers
// ---------------------------------------------------------------------------

// CreateTicket handles POST /support.
func (h *Handlers) CreateTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		TypeID      string         `json:"type_id"`
		Subject     string         `json:"subject"`
		Description string         `json:"description"`
		Priority    string         `json:"priority"`
		Email       string         `json:"email"`
		FieldValues map[string]any `json:"field_values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Subject == "" {
		writeError(w, http.StatusBadRequest, "missing_subject", "subject is required")
		return
	}
	if input.Description == "" {
		writeError(w, http.StatusBadRequest, "missing_description", "description is required")
		return
	}

	var typeID pgtype.UUID
	if input.TypeID != "" {
		if err := typeID.Scan(input.TypeID); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	}

	ticket, err := h.Service.CreateTicket(r.Context(), tenantID, CreateTicketInput{
		TypeID:          typeID,
		Subject:         input.Subject,
		Description:     input.Description,
		Priority:        input.Priority,
		SubmitterUserID: user.UserID,
		SubmitterEmail:  input.Email,
		FieldValues:     input.FieldValues,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": ticket})
}

// ListMyTickets handles GET /support/my.
func (h *Handlers) ListMyTickets(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	tickets, err := h.Service.ListTicketsBySubmitter(r.Context(), tenantID, user.UserID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

// GetMyTicket handles GET /support/my/:id.
func (h *Handlers) GetMyTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	ticket, err := h.Service.GetTicket(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Verify the user is the submitter.
	if ticket.SubmitterUserID != user.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "You can only view your own tickets")
		return
	}

	// Fetch comments (excluding internal notes for non-staff).
	comments, err := h.Service.ListComments(r.Context(), id, false)
	if err != nil {
		slog.Error("listing ticket comments", "ticket_id", id, "error", err)
	}

	// Fetch attachments.
	attachments, err := h.Service.ListAttachments(r.Context(), id)
	if err != nil {
		slog.Error("listing ticket attachments", "ticket_id", id, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ticket":     ticket,
			"comments":   comments,
			"attachments": attachments,
		},
	})
}

// AddReply handles POST /support/my/:id/reply.
func (h *Handlers) AddReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the user is the submitter.
	ticket, err := h.Service.GetTicket(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	if ticket.SubmitterUserID != user.UserID {
		writeError(w, http.StatusForbidden, "forbidden", "You can only reply to your own tickets")
		return
	}

	var input struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	comment, err := h.Service.AddComment(r.Context(), id, AddCommentInput{
		Body:       input.Body,
		IsInternal: false,
		UserID:     user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// ---------------------------------------------------------------------------
// Admin Panel Handlers
// ---------------------------------------------------------------------------

// ListTickets handles GET /api/v1/admin/tickets.
func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	status := r.URL.Query().Get("status")
	priority := r.URL.Query().Get("priority")
	search := r.URL.Query().Get("search")
	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	var typeID, assignedTo pgtype.UUID
	if tid := r.URL.Query().Get("type_id"); tid != "" {
		_ = typeID.Scan(tid)
	}
	if aid := r.URL.Query().Get("assigned_to"); aid != "" {
		_ = assignedTo.Scan(aid)
	}

	tickets, err := h.Service.ListTickets(r.Context(), tenantID, status, priority, typeID, assignedTo, search, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

// GetTicket handles GET /api/v1/admin/tickets/:id.
func (h *Handlers) GetTicket(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	ticket, err := h.Service.GetTicket(r.Context(), id, tenantID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Fetch all comments including internal notes for staff.
	comments, err := h.Service.ListComments(r.Context(), id, true)
	if err != nil {
		slog.Error("listing ticket comments", "ticket_id", id, "error", err)
	}
	attachments, err := h.Service.ListAttachments(r.Context(), id)
	if err != nil {
		slog.Error("listing ticket attachments", "ticket_id", id, "error", err)
	}
	events, err := h.Service.ListEvents(r.Context(), id)
	if err != nil {
		slog.Error("listing ticket events", "ticket_id", id, "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ticket":      ticket,
			"comments":    comments,
			"attachments": attachments,
			"events":      events,
		},
	})
}

// UpdateTicket handles PATCH /api/v1/admin/tickets/:id.
func (h *Handlers) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	var input struct {
		Status         string `json:"status"`
		Priority       string `json:"priority"`
		AssignedTo     string `json:"assigned_to"`
		ResolutionNote string `json:"resolution_note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Handle status update.
	if input.Status != "" {
		ticket, err := h.Service.UpdateTicketStatus(r.Context(), id, tenantID, UpdateTicketStatusInput{
			Status:         input.Status,
			ResolutionNote: input.ResolutionNote,
			UserID:         user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": ticket})
		return
	}

	// Handle priority update.
	if input.Priority != "" {
		ticket, err := h.Service.UpdateTicketPriority(r.Context(), id, tenantID, UpdateTicketPriorityInput{
			Priority: input.Priority,
			UserID:   user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": ticket})
		return
	}

	// Handle assignment update.
	if input.AssignedTo != "" {
		var assignedTo pgtype.UUID
		if err := assignedTo.Scan(input.AssignedTo); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_assigned_to", "Invalid assigned_to")
			return
		}
		ticket, err := h.Service.AssignTicket(r.Context(), id, tenantID, AssignTicketInput{
			AssignedTo: assignedTo,
			UserID:     user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": ticket})
		return
	}

	writeError(w, http.StatusBadRequest, "missing_fields", "At least one field to update is required")
}

// AddInternalNote handles POST /api/v1/admin/tickets/:id/notes.
func (h *Handlers) AddInternalNote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	var input struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Body == "" {
		writeError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	comment, err := h.Service.AddComment(r.Context(), id, AddCommentInput{
		Body:       input.Body,
		IsInternal: true,
		UserID:     user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// GetTicketEvents handles GET /api/v1/admin/tickets/:id/events.
func (h *Handlers) GetTicketEvents(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	events, err := h.Service.ListEvents(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": events})
}

// ---------------------------------------------------------------------------
// Ticket Type Management Handlers
// ---------------------------------------------------------------------------

// ListTicketTypes handles GET /api/v1/admin/ticket-types.
func (h *Handlers) ListTicketTypes(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	types, err := h.Service.ListTicketTypes(r.Context(), tenantID, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": types})
}

// CreateTicketType handles POST /api/v1/admin/ticket-types.
func (h *Handlers) CreateTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}

	var input struct {
		Name              string `json:"name"`
		Slug              string `json:"slug"`
		Description       string `json:"description"`
		FieldGroupID      string `json:"field_group_id"`
		DefaultPriority   string `json:"default_priority"`
		AutoAssignGroupID string `json:"auto_assign_group_id"`
		SlaHours          *int32 `json:"sla_hours"`
		IsPublic          *bool  `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}
	if input.Slug == "" {
		writeError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID, autoAssignGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		_ = fieldGroupID.Scan(input.FieldGroupID)
	}
	if input.AutoAssignGroupID != "" {
		_ = autoAssignGroupID.Scan(input.AutoAssignGroupID)
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	result, err := h.Service.CreateTicketType(r.Context(), tenantID, CreateTicketTypeInput{
		Name:              input.Name,
		Slug:              input.Slug,
		Description:       input.Description,
		FieldGroupID:      fieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: autoAssignGroupID,
		SlaHours:          input.SlaHours,
		IsPublic:          isPublic,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// UpdateTicketType handles PUT /api/v1/admin/ticket-types/:id.
func (h *Handlers) UpdateTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket type ID")
		return
	}

	var input struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		FieldGroupID      string `json:"field_group_id"`
		DefaultPriority   string `json:"default_priority"`
		AutoAssignGroupID string `json:"auto_assign_group_id"`
		SlaHours          *int32 `json:"sla_hours"`
		IsPublic          *bool  `json:"is_public"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}
	if input.Name == "" {
		writeError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var fieldGroupID, autoAssignGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		_ = fieldGroupID.Scan(input.FieldGroupID)
	}
	if input.AutoAssignGroupID != "" {
		_ = autoAssignGroupID.Scan(input.AutoAssignGroupID)
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	result, err := h.Service.UpdateTicketType(r.Context(), id, tenantID, UpdateTicketTypeInput{
		Name:              input.Name,
		Description:       input.Description,
		FieldGroupID:      fieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: autoAssignGroupID,
		SlaHours:          input.SlaHours,
		IsPublic:          isPublic,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteTicketType handles DELETE /api/v1/admin/ticket-types/:id.
func (h *Handlers) DeleteTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	tenantID, ok := h.requireTenant(w, r)
	if !ok {
		return
	}
	id, err := parseUUIDParam(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket type ID")
		return
	}

	if err := h.Service.DeleteTicketType(r.Context(), id, tenantID); err != nil {
		handleServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
