# Section 38 -- Ticket System: Implementation Complete

**Date:** 2026-06-27
**Section:** 38
**Phase:** 6
**Status:** Complete

## Overview

Implemented the support ticket system for JapaConnect. Covers ticket types,
tickets with status state machine, comments (user and internal), attachments,
and event audit trail. Includes auto-assignment, SLA tracking, and separate
endpoints for user portal and admin panel. All tables are tenant-scoped.

## Files Created (10)

| # | File | Lines | Purpose |
|---|------|-------|---------|
| 1 | `migrations/000026_ticket_system.sql` | -- | 5 tables: ticket_types, tickets, ticket_comments, ticket_attachments, ticket_events |
| 2 | `internal/db/queries/tickets.sql` | 240 | 20 sqlc queries (CRUD, status transitions, assignment, SLA) |
| 3 | `internal/db/gen/tickets.sql.go` | 1130 | sqlc-generated Go code |
| 4 | `internal/tickets/types.go` | -- | Constants, status/priority enums, transition maps, input structs |
| 5 | `internal/tickets/store.go` | 261 | Store wrapping sqlc queries |
| 6 | `internal/tickets/service.go` | 629 | Business logic: create, assign, status transitions, SLA, auto-assign |
| 7 | `internal/tickets/handlers.go` | 688 | 15 HTTP handlers (user portal + admin panel) |
| 8 | `internal/tickets/types_test.go` | -- | Status/priority/transition tests |
| 9 | `internal/tickets/service_test.go` | -- | Input validation, defaults, helper tests |
| 10 | `internal/tickets/handlers_test.go` | -- | Handler tests (auth, parsing, error paths) |

## Database Schema

Migration `000026_ticket_system.sql` creates five tables:

- **ticket_types** -- Admin-defined ticket categories with SLA hour targets and
  optional auto-assignment group. Unique on (tenant_id, slug).

- **tickets** -- Individual support tickets with status, priority, assigned agent,
  custom field values (JSONB). Status follows a state machine.

- **ticket_comments** -- User-visible comments and internal notes (is_internal
  flag). Supports markdown content.

- **ticket_attachments** -- File attachments on comments. References the media
  system.

- **ticket_events** -- Append-only audit trail of all ticket state changes.
  Records actor, from/to values, and timestamps.

## Status State Machine

```
open -> in_progress -> resolved -> closed
  |        |              |
  +--------+----> cancelled
```

Invalid transitions (e.g. closed -> open) are rejected by the service layer
via `isValidStatusTransition()`.

## Key Decisions

1. **Auto-assignment** -- round-robin to available agents within the ticket
   type's assigned group. Only triggers on initial creation, not on re-assignment.

2. **SLA tracking** -- ticket types carry `sla_hours`. Service computes remaining
   time from `created_at`. Exposed in API responses for dashboard display but
   no automatic escalation (deferred to Asynq task).

3. **Internal notes** -- ticket_comments with `is_internal=true` are only visible
   to agents, not to the ticket creator. Filtered at the handler level.

4. **Event audit trail** -- every state change (status, priority, assignment)
   creates a ticket_events row. Enables full history for compliance and
   debugging.

5. **Custom field values** -- tickets support JSONB custom fields via the
   existing custom_field_groups system. Allows per-tenant ticket form
   customization.

6. **User portal + admin panel** -- separate endpoint groups. User portal
   endpoints only return the user's own tickets. Admin panel endpoints return
   all tickets with filtering.

## API Routes (15)

### User Portal
| Method | Path | Handler |
|--------|------|---------|
| POST | /api/v1/tickets | CreateTicket |
| GET | /api/v1/tickets | ListMyTickets |
| GET | /api/v1/tickets/{id} | GetTicket |
| POST | /api/v1/tickets/{id}/replies | AddReply |
| GET | /api/v1/tickets/{id}/events | GetTicketEvents |

### Admin Panel
| Method | Path | Handler |
|--------|------|---------|
| GET | /api/v1/admin/tickets | ListTickets |
| GET | /api/v1/admin/tickets/{id} | GetTicket |
| PATCH | /api/v1/admin/tickets/{id}/status | UpdateTicketStatus |
| PATCH | /api/v1/admin/tickets/{id}/priority | UpdateTicketPriority |
| POST | /api/v1/admin/tickets/{id}/assign | AssignTicket |
| POST | /api/v1/admin/tickets/{id}/notes | AddInternalNote |
| GET | /api/v1/admin/ticket-types | ListTicketTypes |
| POST | /api/v1/admin/ticket-types | CreateTicketType |
| PATCH | /api/v1/admin/ticket-types/{id} | UpdateTicketType |
| GET | /api/v1/admin/tickets/{id}/events | GetTicketEventsAdmin |

## Permission Keys (4)

- `tickets.create` -- Create support tickets
- `tickets.moderate` -- Manage tickets (admin operations)
- `tickets.view_all` -- View all tickets (admin)
- `tickets.manage_types` -- Manage ticket type definitions

## Test Results

36 tests covering:
- Status constants and transition validation
- Priority constants and defaults
- Event type constants
- Input validation (create, comment, status, priority, assignment)
- Handler auth checks (no-auth returns 401)
- Helper functions (writeJSON, writeError, parsePageSize)
- Pagination defaults

## Build Verification

| Command | Result |
|---------|--------|
| `go test ./internal/tickets/...` | ok -- 36 tests pass |
| `go build ./...` | success |
| `go vet ./...` | clean |

## Deferrals

- SLA breach escalation via Asynq scheduled task
- Ticket attachment upload via media system integration
- Email notification on ticket status changes
- Ticket search via Meilisearch
