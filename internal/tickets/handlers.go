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
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/auth"
	errpkg "github.com/jedafe/japa/internal/errors"

	"github.com/jedafe/japa/internal/db/gen"
)

const defaultPageSize int32 = 20

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


// requireUser resolves the authenticated user from the request context.
func (h *Handlers) requireUser(w http.ResponseWriter, r *http.Request) (*auth.AuthenticatedUser, bool) {
	user := auth.FromContext(r.Context())
	if user == nil {
		errpkg.WriteError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
		return nil, false
	}

	return user, true
}

// handleServiceError translates a Service-layer error into the appropriate HTTP response.
func handleServiceError(w http.ResponseWriter, err error) {
	errpkg.HandleServiceError(w, err)
}

// parsePageSize extracts and clamps the page_size query parameter.
func parsePageSize(r *http.Request) int32 {
	s := r.URL.Query().Get("page_size")
	if s == "" {
		return defaultPageSize
	}

	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultPageSize
	}

	if n > MaxPageSize {
		return MaxPageSize
	}

	// #nosec G109 // value bounded by MaxPageSize check
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

	if n > math.MaxInt32 {
		return math.MaxInt32
	}

	// #nosec G109 // value bounded by MaxPageSize check
	return int32(n)
}

// ticketRowToTicket converts a GetTicketByIDRow (which includes the joined
// type_name/type_slug) to a gen.Ticket for handlers that need to return the
// base ticket shape after a partial update.
func ticketRowToTicket(r gen.GetTicketByIDRow) gen.Ticket {
	return gen.Ticket{
		ID: r.ID,

		TypeID:          r.TypeID,
		Subject:         r.Subject,
		Description:     r.Description,
		Status:          r.Status,
		Priority:        r.Priority,
		SubmitterUserID: r.SubmitterUserID,
		SubmitterEmail:  r.SubmitterEmail,
		AssignedTo:      r.AssignedTo,
		AssignedGroupID: r.AssignedGroupID,
		ResolutionNote:  r.ResolutionNote,
		FieldValues:     r.FieldValues,
		SlaBreachAt:     r.SlaBreachAt,
		ResolvedAt:      r.ResolvedAt,
		ClosedAt:        r.ClosedAt,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

// ---------------------------------------------------------------------------
// User Portal Handlers
// ---------------------------------------------------------------------------

// CreateTicket handles POST /api/v1/tickets.
func (h *Handlers) CreateTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
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
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Subject == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_subject", "subject is required")
		return
	}

	if input.Description == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_description", "description is required")
		return
	}

	var typeID pgtype.UUID
	if input.TypeID != "" {
		if err := typeID.Scan(input.TypeID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id")
			return
		}
	}

	ticket, err := h.Service.CreateTicket(r.Context(), CreateTicketInput{
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": ticket})
}

// ListMyTickets handles GET /api/v1/me/tickets.
// Ref: japaconnect-engineering/sections/38-ticket-system.md (GET /me/tickets ?status=).
func (h *Handlers) ListMyTickets(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)
	status := r.URL.Query().Get("status")

	tickets, err := h.Service.ListTicketsBySubmitter(r.Context(), user.UserID, status, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

// GetMyTicket handles GET /api/v1/me/tickets/{id}.
func (h *Handlers) GetMyTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	ticket, err := h.Service.GetTicket(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	// Verify the user is the submitter.
	if ticket.SubmitterUserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only view your own tickets")
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"ticket":      ticket,
			"comments":    comments,
			"attachments": attachments,
		},
	})
}

// AddReply handles POST /api/v1/me/tickets/{id}/comments.
func (h *Handlers) AddReply(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the user is the submitter.
	ticket, err := h.Service.GetTicket(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	if ticket.SubmitterUserID != user.UserID {
		errpkg.WriteError(w, http.StatusForbidden, "forbidden", "You can only reply to your own tickets")
		return
	}

	var input struct {
		Body string `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "body is required")
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

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
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
	ok = true
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
		if err := typeID.Scan(tid); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_type_id", "Invalid type_id format")
			return
		}
	}

	if aid := r.URL.Query().Get("assigned_to"); aid != "" {
		if err := assignedTo.Scan(aid); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_assigned_to", "Invalid assigned_to format")
			return
		}
	}

	tickets, err := h.Service.ListTickets(r.Context(), status, priority, typeID, assignedTo, search, limit, offset)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": tickets})
}

// GetTicket handles GET /api/v1/admin/tickets/:id.
func (h *Handlers) GetTicket(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	ticket, err := h.Service.GetTicket(r.Context(), id)
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

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{
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
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	var input struct {
		Subject        string `json:"subject"`
		Status         string `json:"status"`
		Priority       string `json:"priority"`
		AssignedTo     string `json:"assigned_to"`
		ResolutionNote string `json:"resolution_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	// Handle subject update.
	if input.Subject != "" {
		ticket, err := h.Service.UpdateTicketSubject(r.Context(), id, UpdateTicketSubjectInput{
			Subject: input.Subject,
			UserID:  user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ticket})

		return
	}

	// Handle status update.
	if input.Status != "" {
		ticket, err := h.Service.UpdateTicketStatus(r.Context(), id, UpdateTicketStatusInput{
			Status:         input.Status,
			ResolutionNote: input.ResolutionNote,
			UserID:         user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ticket})

		return
	}

	// Handle priority update.
	if input.Priority != "" {
		ticket, err := h.Service.UpdateTicketPriority(r.Context(), id, UpdateTicketPriorityInput{
			Priority: input.Priority,
			UserID:   user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ticket})

		return
	}

	// Handle assignment update.
	if input.AssignedTo != "" {
		var assignedTo pgtype.UUID
		if err := assignedTo.Scan(input.AssignedTo); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_assigned_to", "Invalid assigned_to")
			return
		}

		ticket, err := h.Service.AssignTicket(r.Context(), id, AssignTicketInput{
			AssignedTo: assignedTo,
			UserID:     user.UserID,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}

		errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ticket})

		return
	}

	errpkg.WriteError(w, http.StatusBadRequest, "missing_fields", "At least one field to update is required")
}

// AddComment handles POST /api/v1/admin/tickets/:id/comments.
// Ref: japaconnect-engineering/sections/38-ticket-system.md
// The /comments endpoint defaults is_internal=false (public comment visible
// to the submitter). The /notes endpoint (AddInternalNote) defaults to true.
func (h *Handlers) AddComment(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the ticket belongs to the caller's platform before mutating.
	if _, err := h.Service.GetTicket(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	var input struct {
		Body       string `json:"body"`
		IsInternal *bool  `json:"is_internal"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	// /comments defaults to public (is_internal=false); /notes defaults to
	// internal (is_internal=true). The is_internal field overrides the default.
	isInternal := false
	if input.IsInternal != nil {
		isInternal = *input.IsInternal
	}

	comment, err := h.Service.AddComment(r.Context(), id, AddCommentInput{
		Body:       input.Body,
		IsInternal: isInternal,
		UserID:     user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// AddInternalNote handles POST /api/v1/admin/tickets/:id/notes.
// Ref: japaconnect-engineering/sections/38-ticket-system.md
// The /notes endpoint defaults is_internal=true (staff-only, hidden from
// submitter). The /comments endpoint (AddComment) defaults to false.
func (h *Handlers) AddInternalNote(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the ticket belongs to the caller's platform before mutating.
	if _, err := h.Service.GetTicket(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	var input struct {
		Body       string `json:"body"`
		IsInternal *bool  `json:"is_internal"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Body == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_body", "body is required")
		return
	}

	// /notes always creates internal notes; /comments respects is_internal.
	isInternal := true
	if input.IsInternal != nil {
		isInternal = *input.IsInternal
	}

	comment, err := h.Service.AddComment(r.Context(), id, AddCommentInput{
		Body:       input.Body,
		IsInternal: isInternal,
		UserID:     user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": comment})
}

// GetTicketEvents handles GET /api/v1/admin/tickets/:id/events.
// Ref: japaconnect-engineering/sections/38-ticket-system.md
// Verifies the ticket belongs to the caller's platform before listing events.
func (h *Handlers) GetTicketEvents(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the ticket belongs to the caller's platform before listing events.
	if _, err := h.Service.GetTicket(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	events, err := h.Service.ListEvents(r.Context(), id)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": events})
}

// ResolveTicket handles POST /api/v1/admin/tickets/:id/resolve.
func (h *Handlers) ResolveTicket(w http.ResponseWriter, r *http.Request) {
	user, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	var input struct {
		ResolutionNote string `json:"resolution_note"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.ResolutionNote == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_resolution_note", "resolution_note is required")
		return
	}

	ticket, err := h.Service.ResolveTicket(r.Context(), id, ResolveTicketInput{
		ResolutionNote: input.ResolutionNote,
		UserID:         user.UserID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": ticket})
}

// DeleteTicket handles DELETE /api/v1/admin/tickets/:id.
func (h *Handlers) DeleteTicket(w http.ResponseWriter, r *http.Request) {

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	if err := h.Service.DeleteTicket(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
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
	ok = true
	if !ok {
		return
	}

	limit := parsePageSize(r)
	offset := parsePageSizeOffset(r)

	var isPublic *bool

	if v := r.URL.Query().Get("is_public"); v != "" {
		b := v == "true"
		isPublic = &b
	}

	types, err := h.Service.ListTicketTypes(r.Context(), limit, offset, isPublic)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": types})
}

// AddAttachment handles POST /api/v1/me/tickets/:id/attachments.
// Ref: japaconnect-engineering/sections/38-ticket-system.md
// Verifies the ticket belongs to the caller's platform before adding an
// attachment.
func (h *Handlers) AddAttachment(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	ticketID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the ticket belongs to the caller's platform before mutating.
	if _, err := h.Service.GetTicket(r.Context(), ticketID); err != nil {
		handleServiceError(w, err)
		return
	}

	var input struct {
		MediaID   pgtype.UUID  `json:"media_id"`
		CommentID *pgtype.UUID `json:"comment_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if !input.MediaID.Valid {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_media_id", "media_id is required")
		return
	}

	var commentID pgtype.UUID
	if input.CommentID != nil {
		commentID = *input.CommentID
	}

	attachment, err := h.Service.AddAttachment(r.Context(), ticketID, AddAttachmentInput{
		CommentID: commentID,
		MediaID:   input.MediaID,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": attachment})
}

// ListAttachments handles GET /api/v1/me/tickets/:id/attachments.
// Ref: japaconnect-engineering/sections/38-ticket-system.md
// Verifies the ticket belongs to the caller's platform before listing
// attachments.
func (h *Handlers) ListAttachments(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	ticketID, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket ID")
		return
	}

	// Verify the ticket belongs to the caller's platform before listing.
	if _, err := h.Service.GetTicket(r.Context(), ticketID); err != nil {
		handleServiceError(w, err)
		return
	}

	attachments, err := h.Service.ListAttachments(r.Context(), ticketID)
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": attachments})
}

// CreateTicketType handles POST /api/v1/admin/ticket-types.
func (h *Handlers) CreateTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
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
		SLAHours          *int32 `json:"sla_hours"`
		IsPublic          *bool  `json:"is_public"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	if input.Slug == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_slug", "slug is required")
		return
	}

	var fieldGroupID, autoAssignGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id format")
			return
		}
	}

	if input.AutoAssignGroupID != "" {
		if err := autoAssignGroupID.Scan(input.AutoAssignGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_auto_assign_group_id", "Invalid auto_assign_group_id format")
			return
		}
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	result, err := h.Service.CreateTicketType(r.Context(), CreateTicketTypeInput{
		Name:              input.Name,
		Slug:              input.Slug,
		Description:       input.Description,
		FieldGroupID:      fieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: autoAssignGroupID,
		SLAHours:          input.SLAHours,
		IsPublic:          isPublic,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusCreated, map[string]any{"data": result})
}

// UpdateTicketType handles PUT /api/v1/admin/ticket-types/:id.
func (h *Handlers) UpdateTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket type ID")
		return
	}

	var input struct {
		Name              string `json:"name"`
		Description       string `json:"description"`
		FieldGroupID      string `json:"field_group_id"`
		DefaultPriority   string `json:"default_priority"`
		AutoAssignGroupID string `json:"auto_assign_group_id"`
		SLAHours          *int32 `json:"sla_hours"`
		IsPublic          *bool  `json:"is_public"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_body", "Invalid JSON body")
		return
	}

	if input.Name == "" {
		errpkg.WriteError(w, http.StatusBadRequest, "missing_name", "name is required")
		return
	}

	var fieldGroupID, autoAssignGroupID pgtype.UUID
	if input.FieldGroupID != "" {
		if err := fieldGroupID.Scan(input.FieldGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_field_group_id", "Invalid field_group_id format")
			return
		}
	}

	if input.AutoAssignGroupID != "" {
		if err := autoAssignGroupID.Scan(input.AutoAssignGroupID); err != nil {
			errpkg.WriteError(w, http.StatusBadRequest, "invalid_auto_assign_group_id", "Invalid auto_assign_group_id format")
			return
		}
	}

	isPublic := true
	if input.IsPublic != nil {
		isPublic = *input.IsPublic
	}

	result, err := h.Service.UpdateTicketType(r.Context(), id, UpdateTicketTypeInput{
		Name:              input.Name,
		Description:       input.Description,
		FieldGroupID:      fieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: autoAssignGroupID,
		SLAHours:          input.SLAHours,
		IsPublic:          isPublic,
	})
	if err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

// DeleteTicketType handles DELETE /api/v1/admin/ticket-types/:id.
func (h *Handlers) DeleteTicketType(w http.ResponseWriter, r *http.Request) {
	_, ok := h.requireUser(w, r)
	if !ok {
		return
	}
	ok = true
	if !ok {
		return
	}

	id, err := parseUUIDParam(r, "id")
	if err != nil {
		errpkg.WriteError(w, http.StatusBadRequest, "invalid_id", "Invalid ticket type ID")
		return
	}

	if err := h.Service.DeleteTicketType(r.Context(), id); err != nil {
		handleServiceError(w, err)
		return
	}

	errpkg.WriteJSON(w, http.StatusNoContent, nil)
}
