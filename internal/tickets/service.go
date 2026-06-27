package tickets

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db"
	"github.com/jedafe/japa/internal/db/gen"
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

// NewServiceWithEnqueuer creates a Service with a custom enqueuer for background tasks.
func NewServiceWithEnqueuer(database *db.DB, enqueuer Enqueuer) *Service {
	svc := NewService(database)
	svc.enqueuer = enqueuer
	return svc
}

// noopEnqueuer is a no-op implementation for development.
type noopEnqueuer struct{}

func (n *noopEnqueuer) Enqueue(taskType string, payload []byte, opts ...interface{}) error {
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
	SlaHours          *int32
	IsPublic          bool
	IsSystem          bool
}

// CreateTicketType creates a new admin-defined ticket type.
func (s *Service) CreateTicketType(ctx context.Context, tenantID pgtype.UUID, input CreateTicketTypeInput) (gen.TicketType, error) {
	if input.Name == "" {
		return gen.TicketType{}, fmt.Errorf("name is required")
	}
	if input.Slug == "" {
		return gen.TicketType{}, fmt.Errorf("slug is required")
	}
	if input.DefaultPriority == "" {
		input.DefaultPriority = string(PriorityNormal)
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.createTicketType(ctx, gen.CreateTicketTypeParams{
		TenantID:          tenantID,
		Name:              input.Name,
		Slug:              input.Slug,
		Description:       description,
		FieldGroupID:      input.FieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: input.AutoAssignGroupID,
		SlaHours:          input.SlaHours,
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
func (s *Service) ListTicketTypes(ctx context.Context, tenantID pgtype.UUID, limit, offset int32) ([]gen.TicketType, error) {
	return s.store.listTicketTypesByTenant(ctx, tenantID, limit, offset)
}

// UpdateTicketTypeInput holds parameters for updating a ticket type.
type UpdateTicketTypeInput struct {
	Name              string
	Description       string
	FieldGroupID      pgtype.UUID
	DefaultPriority   string
	AutoAssignGroupID pgtype.UUID
	SlaHours          *int32
	IsPublic          bool
}

// UpdateTicketType updates an existing ticket type.
func (s *Service) UpdateTicketType(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketTypeInput) (gen.TicketType, error) {
	if input.Name == "" {
		return gen.TicketType{}, fmt.Errorf("name is required")
	}

	var description *string
	if input.Description != "" {
		description = &input.Description
	}

	return s.store.updateTicketType(ctx, gen.UpdateTicketTypeParams{
		ID:                id,
		TenantID:          tenantID,
		Name:              input.Name,
		Description:       description,
		FieldGroupID:      input.FieldGroupID,
		DefaultPriority:   input.DefaultPriority,
		AutoAssignGroupID: input.AutoAssignGroupID,
		SlaHours:          input.SlaHours,
		IsPublic:          input.IsPublic,
	})
}

// DeleteTicketType removes a ticket type and its cascaded children.
func (s *Service) DeleteTicketType(ctx context.Context, id, tenantID pgtype.UUID) error {
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
		return gen.Ticket{}, fmt.Errorf("subject is required")
	}
	if input.Description == "" {
		return gen.Ticket{}, fmt.Errorf("description is required")
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

	// Compute SLA breach time.
	var slaBreachAt pgtype.Timestamptz
	if ticketType.SlaHours != nil && *ticketType.SlaHours > 0 {
		breachTime := time.Now().Add(time.Duration(*ticketType.SlaHours) * time.Hour)
		slaBreachAt = pgtype.Timestamptz{Time: breachTime, Valid: true}
	}

	// Auto-assign if configured and no explicit assignment.
	assignedTo := input.AssignedTo
	assignedGroupID := input.AssignedGroupID
	if assignedTo.Valid && assignedGroupID.Valid {
		// Both set — use as-is.
	} else if ticketType.AutoAssignGroupID.Valid {
		// Try auto-assignment.
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
		TenantID:        tenantID,
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
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  ticket.ID,
		UserID:    input.SubmitterUserID,
		EventType: string(EventStatusChanged),
		OldValue:  nil,
		NewValue:  strPtr(string(StatusOpen)),
	})

	// Enqueue confirmation email.
	s.enqueueTicketConfirmation(ticket)

	return ticket, nil
}

// GetTicket retrieves a ticket by ID.
func (s *Service) GetTicket(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetTicketByIDRow, error) {
	return s.store.getTicketByID(ctx, id, tenantID)
}

// ListTickets returns tickets for admin view with filters.
func (s *Service) ListTickets(ctx context.Context, tenantID pgtype.UUID, status, priority string, typeID, assignedTo pgtype.UUID, search string, limit, offset int32) ([]gen.ListTicketsByTenantRow, error) {
	return s.store.listTicketsByTenant(ctx, gen.ListTicketsByTenantParams{
		TenantID:   tenantID,
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
func (s *Service) ListTicketsBySubmitter(ctx context.Context, tenantID, submitterUserID pgtype.UUID, limit, offset int32) ([]gen.ListTicketsBySubmitterRow, error) {
	return s.store.listTicketsBySubmitter(ctx, gen.ListTicketsBySubmitterParams{
		TenantID:         tenantID,
		SubmitterUserID: submitterUserID,
		PageLimit:        limit,
		PageOffset:       offset,
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
		return gen.Ticket{}, fmt.Errorf("resolution note is required when resolving a ticket")
	}

	// Update status.
	ticket, err := s.store.updateTicketStatus(ctx, gen.UpdateTicketStatusParams{
		ID:       id,
		TenantID: tenantID,
		Status:   input.Status,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket status: %w", err)
	}

	// Log status change event.
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(input.Status),
	})

	// Enqueue status update email.
	s.enqueueTicketStatusUpdate(ticket)

	return ticket, nil
}

// UpdateTicketPriorityInput holds parameters for updating ticket priority.
type UpdateTicketPriorityInput struct {
	Priority string
	UserID   pgtype.UUID
}

// UpdateTicketPriority changes the ticket priority.
func (s *Service) UpdateTicketPriority(ctx context.Context, id, tenantID pgtype.UUID, input UpdateTicketPriorityInput) (gen.Ticket, error) {
	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	ticket, err := s.store.updateTicketPriority(ctx, gen.UpdateTicketPriorityParams{
		ID:       id,
		TenantID: tenantID,
		Priority: input.Priority,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket priority: %w", err)
	}

	// Log priority change event.
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventPriorityChanged),
		OldValue:  strPtr(current.Priority),
		NewValue:  strPtr(input.Priority),
	})

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
		ID:              id,
		TenantID:        tenantID,
		AssignedTo:      input.AssignedTo,
		AssignedGroupID: input.AssignedGroupID,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("assigning ticket: %w", err)
	}

	// Log assignment event.
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventAssigned),
		OldValue:  nil,
		NewValue:  strPtr(input.AssignedTo.String()),
	})

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
		ID:             id,
		TenantID:       tenantID,
		ResolutionNote: resolutionNote,
	})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("resolving ticket: %w", err)
	}

	// Log resolution event.
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    input.UserID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(string(StatusResolved)),
	})

	return ticket, nil
}

// CloseTicket closes a ticket (admin action from any state).
func (s *Service) CloseTicket(ctx context.Context, id, tenantID pgtype.UUID, userID pgtype.UUID) (gen.Ticket, error) {
	current, err := s.store.getTicketByID(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("fetching current ticket: %w", err)
	}

	ticket, err := s.store.closeTicket(ctx, id, tenantID)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("closing ticket: %w", err)
	}

	// Log close event.
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  id,
		UserID:    userID,
		EventType: string(EventStatusChanged),
		OldValue:  strPtr(current.Status),
		NewValue:  strPtr(string(StatusClosed)),
	})

	return ticket, nil
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
		return gen.TicketComment{}, fmt.Errorf("body is required")
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
	_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
		TicketID:  ticketID,
		UserID:    input.UserID,
		EventType: string(EventCommentAdded),
		OldValue:  nil,
		NewValue:  strPtr("comment_added"),
	})

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
// SLA Breach Detection
// ---------------------------------------------------------------------------

// CheckSLABreaches finds tickets where SLA breach time has passed and marks them.
func (s *Service) CheckSLABreaches(ctx context.Context, tenantID pgtype.UUID) (int, error) {
	tickets, err := s.store.listTicketsByTenant(ctx, gen.ListTicketsByTenantParams{
		TenantID:   tenantID,
		Status:     "",
		PageLimit:  100,
		PageOffset: 0,
	})
	if err != nil {
		return 0, fmt.Errorf("listing tickets for SLA check: %w", err)
	}

	breached := 0
	now := time.Now()
	for _, t := range tickets {
		if t.SlaBreachAt.Valid && t.SlaBreachAt.Time.Before(now) &&
			t.Status != string(StatusResolved) && t.Status != string(StatusClosed) && t.Status != string(StatusSpam) {
			_, _ = s.store.createTicketEvent(ctx, gen.CreateTicketEventParams{
				TicketID:  t.ID,
				UserID:    pgtype.UUID{},
				EventType: string(EventSLABreached),
				OldValue:  nil,
				NewValue:  strPtr("sla_breached"),
			})
			breached++
		}
	}
	return breached, nil
}

// ---------------------------------------------------------------------------
// Background task helpers
// ---------------------------------------------------------------------------

func (s *Service) enqueueTicketConfirmation(ticket gen.Ticket) {
	payload, err := json.Marshal(map[string]any{
		"ticket_id": ticket.ID.String(),
		"tenant_id": ticket.TenantID.String(),
		"subject":   ticket.Subject,
	})
	if err != nil {
		return
	}
	_ = s.enqueuer.Enqueue(worker.TypeTicketSendConfirmation, payload)
}

func (s *Service) enqueueTicketStatusUpdate(ticket gen.Ticket) {
	payload, err := json.Marshal(map[string]any{
		"ticket_id": ticket.ID.String(),
		"tenant_id": ticket.TenantID.String(),
		"status":    ticket.Status,
		"subject":   ticket.Subject,
	})
	if err != nil {
		return
	}
	_ = s.enqueuer.Enqueue(worker.TypeTicketSendStatusUpdate, payload)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func strPtr(s string) *string {
	return &s
}
