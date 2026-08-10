package tickets

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jedafe/japa/internal/db/gen"
)

// Store provides low-level database access for ticket operations.
// The Service layer wraps Store methods with business logic.
type Store struct {
	write *gen.Queries
	read  *gen.Queries
}

// NewStore creates a Store backed by the given gen.Queries instances.
func NewStore(read, write *gen.Queries) *Store {
	return &Store{read: read, write: write}
}

// ---------------------------------------------------------------------------
// Ticket Types
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createTicketType(ctx context.Context, p gen.CreateTicketTypeParams) (gen.TicketType, error) {
	row, err := s.write.CreateTicketType(ctx, p)
	if err != nil {
		return gen.TicketType{}, fmt.Errorf("creating ticket type: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getTicketTypeByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.TicketType, error) {
	row, err := s.read.GetTicketTypeByID(ctx, gen.GetTicketTypeByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.TicketType{}, fmt.Errorf("getting ticket type by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getTicketTypeBySlug(ctx context.Context, slug string, tenantID pgtype.UUID) (gen.TicketType, error) {
	row, err := s.read.GetTicketTypeBySlug(ctx, gen.GetTicketTypeBySlugParams{Slug: slug, TenantID: tenantID})
	if err != nil {
		return gen.TicketType{}, fmt.Errorf("getting ticket type by slug: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketTypesByTenant(ctx context.Context, tenantID pgtype.UUID, limit, offset int32, isPublic *bool) (ListTicketTypesResponse, error) {
	if limit <= 0 || limit > MaxPageSize {
		limit = DefaultPageSize
	}

	rows, err := s.read.ListTicketTypesByTenant(ctx, gen.ListTicketTypesByTenantParams{
		TenantID:   tenantID,
		IsPublic:   isPublic,
		PageLimit:  limit,
		PageOffset: offset,
	})
	if err != nil {
		return ListTicketTypesResponse{}, fmt.Errorf("listing ticket types: %w", err)
	}

	total, err := s.read.CountTicketTypesByTenant(ctx, gen.CountTicketTypesByTenantParams{
		TenantID: tenantID,
		IsPublic: isPublic,
	})
	if err != nil {
		return ListTicketTypesResponse{}, fmt.Errorf("counting ticket types: %w", err)
	}

	totalPages := int32(0)
	if total > 0 && limit > 0 {
		totalPages = int32((total + int64(limit) - 1) / int64(limit))
	}

	items := rows
	if items == nil {
		items = make([]gen.TicketType, 0)
	}

	return ListTicketTypesResponse{
		Items:      items,
		Total:      total,
		Page:       offset / limit,
		TotalPages: totalPages,
	}, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateTicketType(ctx context.Context, p gen.UpdateTicketTypeParams) (gen.TicketType, error) {
	row, err := s.write.UpdateTicketType(ctx, p)
	if err != nil {
		return gen.TicketType{}, fmt.Errorf("updating ticket type: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteTicketType(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteTicketType(ctx, gen.DeleteTicketTypeParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting ticket type: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Tickets
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createTicket(ctx context.Context, p gen.CreateTicketParams) (gen.Ticket, error) {
	row, err := s.write.CreateTicket(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("creating ticket: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) getTicketByID(ctx context.Context, id, tenantID pgtype.UUID) (gen.GetTicketByIDRow, error) {
	row, err := s.read.GetTicketByID(ctx, gen.GetTicketByIDParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.GetTicketByIDRow{}, fmt.Errorf("getting ticket by ID: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketsByTenant(ctx context.Context, p gen.ListTicketsByTenantParams) (ListTicketsResponse, error) {
	if p.PageLimit <= 0 || p.PageLimit > MaxPageSize {
		p.PageLimit = DefaultPageSize
	}

	rows, err := s.read.ListTicketsByTenant(ctx, p)
	if err != nil {
		return ListTicketsResponse{}, fmt.Errorf("listing tickets: %w", err)
	}

	total, err := s.read.CountTicketsByTenant(ctx, gen.CountTicketsByTenantParams{
		TenantID:   p.TenantID,
		Status:     p.Status,
		Priority:   p.Priority,
		TypeID:     p.TypeID,
		AssignedTo: p.AssignedTo,
		Search:     p.Search,
	})
	if err != nil {
		return ListTicketsResponse{}, fmt.Errorf("counting tickets: %w", err)
	}

	totalPages := int32(0)
	if total > 0 && p.PageLimit > 0 {
		totalPages = int32((total + int64(p.PageLimit) - 1) / int64(p.PageLimit))
	}

	items := rows
	if items == nil {
		items = make([]gen.ListTicketsByTenantRow, 0)
	}

	return ListTicketsResponse{
		Tickets:     items,
		Total:       total,
		Page:        p.PageOffset / p.PageLimit,
		TotalPages:  totalPages,
	}, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketsBySubmitter(ctx context.Context, p gen.ListTicketsBySubmitterParams) ([]gen.ListTicketsBySubmitterRow, error) {
	if p.PageLimit <= 0 || p.PageLimit > MaxPageSize {
		p.PageLimit = DefaultPageSize
	}

	rows, err := s.read.ListTicketsBySubmitter(ctx, p)
	if err != nil {
		return nil, fmt.Errorf("listing tickets by submitter: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateTicketStatus(ctx context.Context, p gen.UpdateTicketStatusParams) (gen.Ticket, error) {
	row, err := s.write.UpdateTicketStatus(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket status: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateTicketPriority(ctx context.Context, p gen.UpdateTicketPriorityParams) (gen.Ticket, error) {
	row, err := s.write.UpdateTicketPriority(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket priority: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) assignTicket(ctx context.Context, p gen.AssignTicketParams) (gen.Ticket, error) {
	row, err := s.write.AssignTicket(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("assigning ticket: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) resolveTicket(ctx context.Context, p gen.ResolveTicketParams) (gen.Ticket, error) {
	row, err := s.write.ResolveTicket(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("resolving ticket: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) closeTicket(ctx context.Context, id, tenantID pgtype.UUID) (gen.Ticket, error) {
	row, err := s.write.CloseTicket(ctx, gen.CloseTicketParams{ID: id, TenantID: tenantID})
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("closing ticket: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) updateTicketSubject(ctx context.Context, p gen.UpdateTicketSubjectParams) (gen.Ticket, error) {
	row, err := s.write.UpdateTicketSubject(ctx, p)
	if err != nil {
		return gen.Ticket{}, fmt.Errorf("updating ticket subject: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteTicket(ctx context.Context, id, tenantID pgtype.UUID) error {
	if err := s.write.DeleteTicket(ctx, gen.DeleteTicketParams{ID: id, TenantID: tenantID}); err != nil {
		return fmt.Errorf("deleting ticket: %w", err)
	}

	return nil
}

//nolint:unused // reserved for future features
func (s *Store) getLeastLoadedStaffInGroup(ctx context.Context, groupID, tenantID pgtype.UUID) (pgtype.UUID, error) {
	userID, err := s.read.GetLeastLoadedStaffInGroup(ctx, gen.GetLeastLoadedStaffInGroupParams{
		GroupID:  groupID,
		TenantID: tenantID,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("getting least loaded staff: %w", err)
	}

	return userID, nil
}

//nolint:unused // reserved for future features
func (s *Store) getOpenTicketCountByAssignee(ctx context.Context, tenantID pgtype.UUID) ([]gen.GetOpenTicketCountByAssigneeRow, error) {
	rows, err := s.read.GetOpenTicketCountByAssignee(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("getting open ticket counts: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Ticket Comments
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createTicketComment(ctx context.Context, p gen.CreateTicketCommentParams) (gen.TicketComment, error) {
	row, err := s.write.CreateTicketComment(ctx, p)
	if err != nil {
		return gen.TicketComment{}, fmt.Errorf("creating ticket comment: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketComments(ctx context.Context, ticketID pgtype.UUID, includeInternal bool) ([]gen.TicketComment, error) {
	rows, err := s.read.ListTicketComments(ctx, gen.ListTicketCommentsParams{
		TicketID:        ticketID,
		IncludeInternal: includeInternal,
	})
	if err != nil {
		return nil, fmt.Errorf("listing ticket comments: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) deleteTicketComment(ctx context.Context, id, ticketID pgtype.UUID) error {
	if err := s.write.DeleteTicketComment(ctx, gen.DeleteTicketCommentParams{ID: id, TicketID: ticketID}); err != nil {
		return fmt.Errorf("deleting ticket comment: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Ticket Attachments
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createTicketAttachment(ctx context.Context, p gen.CreateTicketAttachmentParams) (gen.TicketAttachment, error) {
	row, err := s.write.CreateTicketAttachment(ctx, p)
	if err != nil {
		return gen.TicketAttachment{}, fmt.Errorf("creating ticket attachment: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketAttachments(ctx context.Context, ticketID pgtype.UUID) ([]gen.TicketAttachment, error) {
	rows, err := s.read.ListTicketAttachments(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("listing ticket attachments: %w", err)
	}

	return rows, nil
}

// ---------------------------------------------------------------------------
// Ticket Events (Audit Trail)
// ---------------------------------------------------------------------------

//nolint:unused // reserved for future features
func (s *Store) createTicketEvent(ctx context.Context, p gen.CreateTicketEventParams) (gen.TicketEvent, error) {
	row, err := s.write.CreateTicketEvent(ctx, p)
	if err != nil {
		return gen.TicketEvent{}, fmt.Errorf("creating ticket event: %w", err)
	}

	return row, nil
}

//nolint:unused // reserved for future features
func (s *Store) listTicketEvents(ctx context.Context, ticketID pgtype.UUID) ([]gen.TicketEvent, error) {
	rows, err := s.read.ListTicketEvents(ctx, ticketID)
	if err != nil {
		return nil, fmt.Errorf("listing ticket events: %w", err)
	}

	return rows, nil
}

//nolint:unused // reserved for future features
func (s *Store) countTicketsByStatus(ctx context.Context, tenantID pgtype.UUID, status string) (int64, error) {
	count, err := s.read.CountTicketsByStatus(ctx, gen.CountTicketsByStatusParams{
		TenantID: tenantID,
		Status:   status,
	})
	if err != nil {
		return 0, fmt.Errorf("counting tickets by status: %w", err)
	}

	return count, nil
}
