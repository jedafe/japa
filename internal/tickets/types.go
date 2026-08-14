// Package tickets implements the Ticket System for JapaConnect (Section 38).
//
// Two-layer architecture: admin-defined Ticket Types with configurable
// SLA and auto-assignment, and user-submitted tickets with full audit trail,
// internal notes, and SLA breach tracking.
//
// Status state machine:
//
//	open → in_progress → waiting_on_user → resolved → closed
//	Any state → closed (admin action)
//	Any state → spam (admin action)
package tickets

import (
	"github.com/jedafe/japa/internal/db/gen"
)

// TicketStatus represents the lifecycle state of a ticket.
type TicketStatus string

// Ticket status constants representing the lifecycle states.
const (
	StatusOpen        TicketStatus = "open"
	StatusInProgress  TicketStatus = "in_progress"
	StatusWaitingUser TicketStatus = "waiting_on_user"
	StatusResolved    TicketStatus = "resolved"
	StatusClosed      TicketStatus = "closed"
	StatusSpam        TicketStatus = "spam"
)

// TicketPriority represents the urgency level of a ticket.
type TicketPriority string

// Ticket priority constants representing urgency levels.
const (
	PriorityLow    TicketPriority = "low"
	PriorityNormal TicketPriority = "normal"
	PriorityHigh   TicketPriority = "high"
	PriorityUrgent TicketPriority = "urgent"
)

// EventType represents the type of audit trail event.
type EventType string

// Audit trail event types for ticket lifecycle tracking.
const (
	EventStatusChanged   EventType = "status_changed"
	EventPriorityChanged EventType = "priority_changed"
	EventAssigned        EventType = "assigned"
	EventCommentAdded    EventType = "comment_added"
	EventSLABreached     EventType = "sla_breached"
)

// validStatusTransitions defines the allowed status state machine transitions.
var validStatusTransitions = map[TicketStatus][]TicketStatus{
	StatusOpen:        {StatusInProgress, StatusSpam, StatusClosed},
	StatusInProgress:  {StatusWaitingUser, StatusResolved, StatusSpam, StatusClosed},
	StatusWaitingUser: {StatusInProgress, StatusResolved, StatusSpam, StatusClosed},
	StatusResolved:    {StatusClosed, StatusSpam},
	StatusClosed:      {StatusSpam},
	StatusSpam:        {},
}

// IsValidStatusTransition checks whether transitioning from one status to another is allowed.
func IsValidStatusTransition(from, to TicketStatus) bool {
	allowed, ok := validStatusTransitions[from]
	if !ok {
		return false
	}

	for _, s := range allowed {
		if s == to {
			return true
		}
	}

	return false
}

// AllStatuses returns all valid ticket statuses.
func AllStatuses() []TicketStatus {
	return []TicketStatus{
		StatusOpen,
		StatusInProgress,
		StatusWaitingUser,
		StatusResolved,
		StatusClosed,
		StatusSpam,
	}
}

// AllPriorities returns all valid ticket priorities.
func AllPriorities() []TicketPriority {
	return []TicketPriority{
		PriorityLow,
		PriorityNormal,
		PriorityHigh,
		PriorityUrgent,
	}
}

// Pagination defaults.
const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// ListTicketsResponse is the paginated response for listing tickets.
type ListTicketsResponse struct {
	Tickets    []gen.ListTicketsByTenantRow `json:"tickets"`
	Total      int64                        `json:"total"`
	Page       int32                        `json:"page"`
	TotalPages int32                        `json:"total_pages"`
}

// ListTicketTypesResponse is the paginated response for listing ticket types.
type ListTicketTypesResponse struct {
	Items      []gen.TicketType `json:"items"`
	Total      int64            `json:"total"`
	Page       int32            `json:"page"`
	TotalPages int32            `json:"total_pages"`
}
