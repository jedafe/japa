package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/config"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
	errpkg "github.com/jedafe/japa/internal/errors"
	"github.com/jedafe/japa/internal/worker"
)

// Enqueuer is the interface for enqueuing background tasks.
type Enqueuer interface {
	Enqueue(taskType string, payload []byte, opts ...interface{}) error
}

// Service provides ticket operations with reader/writer pool separation.
type Service struct {
	store    *Store
	db       *db.DB
	enqueuer Enqueuer
	settings *config.Settings
}

// NewService creates a Service backed by the given database pool.
func NewService(database *db.DB) *Service {
	return &Service{
		store: &Store{
			read:  gen.New(database.Reader),
			write: gen.New(database.Writer),
		},
		db:       database,
		enqueuer: &noopEnqueuer{},
	}
}

// SetSettings injects the config.Settings provider for hot-reloadable ticket
// settings (tickets.default_sla_hours, tickets.auto_close_resolved_days).
func (s *Service) SetSettings(settings *config.Settings) { s.settings = settings }

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer

	return svc
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(_ string, _ []byte, _ ...interface{}) error {
	return nil
}

// ---------------------------------------------------------------------------
// Ticket Types
// ---------------------------------------------------------------------------

// CreateTicketTypeInput holds parameters for creating a ticket type.
type CreateTicketTypeInput struct {
	Name              string
	Slug              string
	Description       string
	FieldGroupID      pgtype.UUID
	DefaultPriority   string
	AutoAssignGroupID pgtype.UUID
	SLAHours          *int32
	IsPublic          bool
	IsSystem          bool
}

// CreateTicketType creates a new admin-defined ticket type.
func (s *Service) CreateTicketType(ctx context.Context, tenantID pgtype.UUID, input CreateTicketTypeInput) (gen.TicketType, error) {
	if input.Name == "" {
		return gen.TicketType{}, errors.New("name is required")
	}

	if input.Slug == "" {
		return gen.TicketType{}, errors.New("slug is required")
	}

	if input.DefaultPriority == "" {
		input.DefaultPriority = string(PriorityNormal)
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.createTicketType(ctx, gen.CreateTicketTypeParams{

		Name:              input.Name,
		Slug:              input.Slug,
		Description:       description,
		FieldGroupID:      input.FieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: input.AutoAssignGroupID,
		SlaHours:          input.SLAHours,
		IsPublic:          input.IsPublic,
		IsSystem:          input.IsSystem,
	})
}

// GetTicketType retrieves a ticket type by ID.
func (s *Service) GetTicketType(ctx context.Context, id, tenantID pgtype.UUID) (gen.TicketType, error) {
	return s.store.getTicketTypeByID(ctx, id, tenantID)
}

// GetTicketTypeBySlug retrieves a ticket type by slug.
func (s *Service) GetTicketTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.TicketType, error) {
	return s.store.getTicketTypeBySlug(ctx, slug, tenantID)
}

// ListTicketTypes returns all ticket types for a tenant.
func (s *Service) ListTicketTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32, isPublic *bool) (ListTicketTypesResponse, error) {
	return s.store.listTicketTypesByTenant(ctx, tenantID, limit, offset, isPublic)
}

// UpdateTicketTypeInput holds parameters for updating a ticket type.
type UpdateTicketTypeInput struct {
	Name              string
	Description       string
	FieldGroupID      pgtype.UUID
	DefaultPriority   string
	AutoAssignGroupID pgtype.UUID
	SLAHours          *int32
	IsPublic          bool
}

// UpdateTicketType updates an existing ticket type.
func (s *Service) UpdateTicketType(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketTypeInput) (gen.TicketType, error) {
	if input.Name == "" {
		return gen.TicketType{}, errors.New("name is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.updateTicketType(ctx, gen.UpdateTicketTypeParams{
		ID: id,

		Name:              input.Name,
		Description:       description,
		FieldGroupID:      input.FieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: input.AutoAssignGroupID,
		SlaHours:          input.SLAHours,
		IsPublic:          input.IsPublic,
	})
}

// DeleteTicketType removes a ticket type and its cascaded children.
// DeleteTicketType deletes a ticket type only if no tickets reference it.
// Ref: japaconnect-engineering/sections/38-ticket-system.md (delete only if no tickets).
func (s *Service) DeleteTicketType(ctx context.Context, id, tenantID pgtype.UUID) error {
	count, err := s.store.countTicketsByType(ctx, id, tenantID)
	if err != nil {
		return fmt.Errorf("checking ticket type usage: %w", err)
	}

	if count > 0 {
		return fmt.Errorf("%w: ticket type has %d existing tickets", errpkg.ErrInUse, count)
	}

	return s.store.deleteTicketType(ctx, id, tenantID)
}

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

// CreateTicketInput holds parameters for creating a ticket.
type CreateTicketInput struct {
	TypeID          pgtype.UUID
	Subject         string
	Description     string
	Priority        string
	SubmitterUserID pgtype.UUID
	SubmitterEmail  string
	AssignedTo      pgtype.UUID
	AssignedGroupID pgtype.UUID
	FieldValues     map[string]any
}

// CreateTicket creates a new ticket with auto-assignment if configured.
func (s *Service) CreateTicket(ctx context.Context, tenantID pgtype.UUID, input CreateTicketInput) (gen.Ticket, error) {
	if input.Subject == "" {
		return gen.Ticket{}, errors.New("subject is required")
	}

	if input.Description == "" {
		return gen.Ticket{}, errors.New("description is required")
	}

	// Resolve ticket type for default priority and SLA.
	ticketType, err := s.store.getTicketTypeByID(ctx, input.TypeID, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("resolving ticket type: %w", err)
	}

	priority := input.Priority
	if priority == "" {
		priority = ticketType.DefaultPriority
	}

	if !ValidPriority(priority) {
		return gen.Ticket{}, fmt.Errorf("invalid priority %q: must be one of low, normal, high, urgent", priority)
	}

	// Compute SLA breach time. Fall back to tickets.default_sla_hours
	// (system_settings, default 48) when the ticket type has no SLA configured.
	var slaBreachAt pgtype.Timestamptz

	slaHours := int32(0)
	if ticketType.SlaHours != nil && *ticketType.SlaHours > 0 {
		slaHours = *ticketType.SlaHours
	} else if s.settings != nil {
		slaHours = int32(s.settings.GetIntOr(ctx, pgtype.UUID{}, "tickets.default_sla_hours", 48)) // #nosec
	}

	if slaHours > 0 {
		breachTime := time.Now().Add(time.Duration(slaHours) * time.Hour)
		slaBreachAt = pgtype.Timestamptz{Time: breachTime, Valid: true}
	}

	// Auto-assign if configured and no explicit assignment.
	assignedTo := input.AssignedTo
	assignedGroupID := input.AssignedGroupID

	if (!assignedTo.Valid || !assignedGroupID.Valid) && ticketType.AutoAssignGroupID.Valid {
		// Try auto-assignment when neither is explicitly set.
		leastLoaded, assignErr := s.store.getLeastLoadedStaffInGroup(ctx, ticketType.AutoAssignGroupID, tenantID)
		if assignErr == nil && leastLoaded.Valid {
			assignedTo = leastLoaded
			assignedGroupID = ticketType.AutoAssignGroupID
		}
	}

	// Marshal field values.
	fieldValuesJSON := []byte("{}")

	if len(input.FieldValues) > 0 {
		b, err := json.Marshal(input.FieldValues)
		if err != nil {
			return gen.Ticket{}, fmt.Errorf("marshaling field values: %w", err)
		}

		fieldValuesJSON = b
	}

	// Determine submitter email.
	var submitterEmail *string
	if input.SubmitterEmail != "" {
		submitterEmail = &input.SubmitterEmail
	}

	ticket, err := s.store.createTicket(ctx, gen.CreateTicketParams{

		TypeID:          input.TypeID,
		Subject:         input.Subject,
		Description:     input.Description,
		Status:          string(StatusOpen),
		Priority:        priority,
		SubmitterUserID: input.SubmitterUserID,
		SubmitterEmail:  submitterEmail,
		AssignedTo:      assignedTo,
		AssignedGroupID: assignedGroupID,
		FieldValues:     fieldValuesJSON,
		SlaBreachAt:     slaBreachAt,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("creating ticket: %w", err)
	}

	// Log creation event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  ticket.ID,
		UserID:    input.SubmitterUserID,
		EventType: string(EventStatusChanged),
		OldValue:  nil,
		NewValue:  strPtr(string(StatusOpen)),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", ticket.ID, "error", err)
	}

	// Enqueue confirmation email.
	s.enqueueTicketConfirmation(ticket)

	return ticket, nil
}

// GetTicket retrieves a ticket by ID.
func (s *Service) GetTicket(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetTicketByIDRow, error) {
	return s.store.getTicketByID(ctx, id, tenantID)
}

// ListTickets returns tickets for admin view with filters.
func (s *Service) ListTickets(ctx context.Context, tenantID pgtype.UUID, status, priority string, typeID, assignedTo pgtype.UUID, search string, limit, offset int32) (ListTicketsResponse, error) {
	return s.store.listTicketsByTenant(ctx, gen.ListTicketsByTenantParams{

		Status:     status,
		Priority:   priority,
		TypeID:     typeID,
		AssignedTo: assignedTo,
		Search:     search,
		PageLimit:  limit,
		PageOffset: offset,
	})
}

// ListTicketsBySubmitter returns tickets submitted by a specific user.
// Ref: japaconnect-engineering/sections/38-ticket-system.md (GET /me/tickets ?status=)
// An empty status string returns all tickets; a non-empty status filters by it.
func (s *Service) ListTicketsBySubmitter(ctx context.Context, tenantID, submitterUserID pgtype.UUID, status string, limit, offset int32) ([]gen.ListTicketsBySubmitterRow, error) {
	var statusPtr *string

	if status != "" {
		s := status
		statusPtr = &s
	}

	return s.store.listTicketsBySubmitter(ctx, gen.ListTicketsBySubmitterParams{

		SubmitterUserID: submitterUserID,
		Status:          statusPtr,
		PageLimit:       limit,
		PageOffset:      offset,
	})
}

// UpdateTicketStatusInput holds parameters for updating ticket status.
type UpdateTicketStatusInput struct {
	Status         string
	ResolutionNote string
	UserID         pgtype.UUID
}

// UpdateTicketStatus changes the ticket status with state machine validation.
func (s *Service) UpdateTicketStatus(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketStatusInput) (gen.Ticket, error) {
	// Fetch current ticket for state machine validation.
	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	newStatus := TicketStatus(input.Status)
	oldStatus := TicketStatus(current.Status)

	// Validate state machine transition.
	if !IsValidStatusTransition(oldStatus, newStatus) {
		return gen.Ticket{}, fmt.Errorf("invalid status transition: %s → %s", oldStatus, newStatus)
	}

	// Resolve or close tickets need resolution note.
	if newStatus == StatusResolved && input.ResolutionNote == "" {
		return gen.Ticket{}, errors.New("resolution note is required when resolving a ticket")
	}

	// Update status.
	ticket, err := s.store.updateTicketStatus(ctx, gen.UpdateTicketStatusParams{
		ID: id,

		Status: input.Status,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket status: %w", err)
	}

	// Log status change event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(input.Status),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", id, "error", err)
	}

	// Enqueue status update email.
	s.enqueueTicketStatusUpdate(ticket, current.Status)

	return ticket, nil
}

// UpdateTicketPriorityInput holds parameters for updating ticket priority.
type UpdateTicketPriorityInput struct {
	Priority string
	UserID   pgtype.UUID
}

// UpdateTicketPriority changes the ticket priority.
func (s *Service) UpdateTicketPriority(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketPriorityInput) (gen.Ticket, error) {
	if !ValidPriority(input.Priority) {
		return gen.Ticket{}, fmt.Errorf("invalid priority %q: must be one of low, normal, high, urgent", input.Priority)
	}

	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	ticket, err := s.store.updateTicketPriority(ctx, gen.UpdateTicketPriorityParams{
		ID: id,

		Priority: input.Priority,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket priority: %w", err)
	}

	// Log priority change event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventPriorityChanged),
		OldValue:  strPtr(current.Priority),
		NewValue:  strPtr(input.Priority),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", id, "error", err)
	}

	return ticket, nil
}

// AssignTicketInput holds parameters for assigning a ticket.
type AssignTicketInput struct {
	AssignedTo      pgtype.UUID
	AssignedGroupID pgtype.UUID
	UserID          pgtype.UUID
}

// AssignTicket assigns a ticket to a staff member.
func (s *Service) AssignTicket(ctx context.Context, id, tenantID pgtype.UUID, input AssignTicketInput) (gen.Ticket, error) {
	ticket, err := s.store.assignTicket(ctx, gen.AssignTicketParams{
		ID: id,

		AssignedTo:      input.AssignedTo,
		AssignedGroupID: input.AssignedGroupID,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("assigning ticket: %w", err)
	}

	// Log assignment event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventAssigned),
		OldValue:  nil,
		NewValue:  strPtr(input.AssignedTo.String()),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", id, "error", err)
	}

	return ticket, nil
}

// ResolveTicketInput holds parameters for resolving a ticket.
type ResolveTicketInput struct {
	ResolutionNote string
	UserID         pgtype.UUID
}

// ResolveTicket marks a ticket as resolved.
func (s *Service) ResolveTicket(ctx context.Context, id, tenantID pgtype.UUID, input ResolveTicketInput) (gen.Ticket, error) {
	// Validate current status allows resolution.
	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	if !IsValidStatusTransition(TicketStatus(current.Status), StatusResolved) {
		return gen.Ticket{}, fmt.Errorf("ticket cannot be resolved from status: %s", current.Status)
	}

	var resolutionNote *string
	if input.ResolutionNote != "" {
		resolutionNote = &input.ResolutionNote
	}

	ticket, err := s.store.resolveTicket(ctx, gen.ResolveTicketParams{
		ID: id,

		ResolutionNote: resolutionNote,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("resolving ticket: %w", err)
	}

	// Log resolution event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(string(StatusResolved)),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", id, "error", err)
	}

	// Enqueue status update email (fire-and-forget per Rule 8).
	s.enqueueTicketStatusUpdate(ticket, current.Status)

	return ticket, nil
}

// CloseTicket closes a ticket (admin action from any state).
func (s *Service) CloseTicket(ctx context.Context, id, tenantID, userID pgtype.UUID) (gen.Ticket, error) {
	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	ticket, err := s.store.closeTicket(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("closing ticket: %w", err)
	}

	// Log close event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    userID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(string(StatusClosed)),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", id, "error", err)
	}

	return ticket, nil
}

// UpdateTicketSubjectInput holds parameters for updating a ticket subject.
type UpdateTicketSubjectInput struct {
	Subject string
	UserID  pgtype.UUID
}

// UpdateTicketSubject changes the ticket subject.
func (s *Service) UpdateTicketSubject(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketSubjectInput) (gen.Ticket, error) {
	if input.Subject == "" {
		return gen.Ticket{}, errors.New("subject is required")
	}

	ticket, err := s.store.updateTicketSubject(ctx, gen.UpdateTicketSubjectParams{
		ID: id,

		Subject: input.Subject,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket subject: %w", err)
	}

	return ticket, nil
}

// DeleteTicket permanently removes a ticket (admin action, spam/test only).
// Ref: japaconnect-engineering/sections/38-ticket-system.md.
func (s *Service) DeleteTicket(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.store.deleteTicket(ctx, id, tenantID); err != nil {
		return fmt.Errorf("deleting ticket: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Ticket Comments
// ---------------------------------------------------------------------------

// AddCommentInput holds parameters for adding a comment.
type AddCommentInput struct {
	Body       string
	IsInternal bool
	UserID     pgtype.UUID
}

// AddComment adds a comment to a ticket.
func (s *Service) AddComment(ctx context.Context, ticketID pgtype.UUID, input AddCommentInput) (gen.TicketComment, error) {
	if input.Body == "" {
		return gen.TicketComment{}, errors.New("body is required")
	}

	comment, err := s.store.createTicketComment(ctx, gen.CreateTicketCommentParams{
		TicketID:   ticketID,
		UserID:     input.UserID,
		Body:       input.Body,
		IsInternal: input.IsInternal,
	})
	if err != nil {
		return gen.TicketComment{}, fmt.Errorf("adding comment: %w", err)
	}

	// Log comment event.
	if _, err := s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  ticketID,
		UserID:    input.UserID,
		EventType: string(EventCommentAdded),
		OldValue:  nil,
		NewValue:  strPtr("comment_added"),
	}); err != nil {
		slog.Error("create ticket event", "ticket_id", ticketID, "error", err)
	}

	return comment, nil
}

// ListComments returns comments for a ticket, optionally including internal notes.
func (s *Service) ListComments(ctx context.Context, ticketID pgtype.UUID, includeInternal bool) ([]gen.TicketComment, error) {
	return s.store.listTicketComments(ctx, ticketID, includeInternal)
}

// DeleteComment removes a comment from a ticket.
func (s *Service) DeleteComment(ctx context.Context, commentID, ticketID pgtype.UUID) error {
	return s.store.deleteTicketComment(ctx, commentID, ticketID)
}

// ---------------------------------------------------------------------------
// Ticket Attachments
// ---------------------------------------------------------------------------

// AddAttachmentInput holds parameters for adding an attachment.
type AddAttachmentInput struct {
	CommentID pgtype.UUID
	MediaID   pgtype.UUID
}

// AddAttachment adds a media attachment to a ticket.
func (s *Service) AddAttachment(ctx context.Context, ticketID pgtype.UUID, input AddAttachmentInput) (gen.TicketAttachment, error) {
	return s.store.createTicketAttachment(ctx, gen.CreateTicketAttachmentParams{
		TicketID:  ticketID,
		CommentID: input.CommentID,
		MediaID:   input.MediaID,
	})
}

// ListAttachments returns attachments for a ticket.
func (s *Service) ListAttachments(ctx context.Context, ticketID pgtype.UUID) ([]gen.TicketAttachment, error) {
	return s.store.listTicketAttachments(ctx, ticketID)
}

// ---------------------------------------------------------------------------
// Ticket Events (Audit Trail)
// ---------------------------------------------------------------------------

// ListEvents returns the audit trail for a ticket.
func (s *Service) ListEvents(ctx context.Context, ticketID pgtype.UUID) ([]gen.TicketEvent, error) {
	return s.store.listTicketEvents(ctx, ticketID)
}

// ---------------------------------------------------------------------------
// Background task helpers
// ---------------------------------------------------------------------------

func (s *Service) enqueueTicketConfirmation(ticket gen.Ticket) {
	payload, err := json.Marshal(map[string]any{
		"ticket_id": ticket.ID.String(),
	})
	if err != nil {
		return
	}

	if err := s.enqueuer.Enqueue(worker.TypeTicketSendConfirmation, payload); err != nil {
		slog.Error("enqueue ticket confirmation email", "ticket_id", ticket.ID, "error", err)
	}
}

func (s *Service) enqueueTicketStatusUpdate(ticket gen.Ticket, oldStatus string) {
	payload, err := json.Marshal(map[string]any{
		"ticket_id":  ticket.ID.String(),
		"old_status": oldStatus,
		"new_status": ticket.Status,
	})
	if err != nil {
		return
	}

	if err := s.enqueuer.Enqueue(worker.TypeTicketSendStatusUpdate, payload); err != nil {
		slog.Error("enqueue ticket status update email", "ticket_id", ticket.ID, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string {
	return &s
}
