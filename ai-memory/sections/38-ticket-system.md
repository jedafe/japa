# Section 38 — Ticket System

**Status:** ✅ Locked

## Scope

A full feature-rich ticket system covering: user-submitted support tickets, bug reports, feature requests, and any internal workflow requiring tracked resolution. Sits inside the Admin Panel (Section 22) for staff-facing views and exposes a minimal user-facing portal at `/support` on the main site.

## Schema

```sql
CREATE TABLE ticket_types (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  name            text NOT NULL,    -- 'Support', 'Bug Report', 'Feature Request', 'Account Issue'
  slug            text NOT NULL,
  description     text,
  field_group_id  uuid REFERENCES custom_field_groups(id),  -- Section 06
  default_priority text NOT NULL DEFAULT 'normal',
  auto_assign_group_id uuid REFERENCES user_groups(id),
  sla_hours       int,              -- NULL = no SLA
  is_public       boolean NOT NULL DEFAULT true,  -- visible to submitter
  is_system       boolean NOT NULL DEFAULT false,
  UNIQUE(tenant_id, slug)
);

CREATE TABLE tickets (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  type_id         uuid NOT NULL REFERENCES ticket_types(id),
  subject         text NOT NULL,
  description     text NOT NULL,   -- Plain text or TipTap JSON
  status          text NOT NULL DEFAULT 'open',
  -- 'open'|'in_progress'|'waiting_on_user'|'resolved'|'closed'|'spam'
  priority        text NOT NULL DEFAULT 'normal',
  -- 'low'|'normal'|'high'|'urgent'
  submitter_user_id uuid REFERENCES users(id),
  submitter_email text,            -- for anonymous/guest submissions
  assigned_to     uuid REFERENCES users(id),
  assigned_group_id uuid REFERENCES user_groups(id),
  resolution_note text,
  field_values    jsonb NOT NULL DEFAULT '{}',  -- custom fields from ticket_type
  sla_breach_at   timestamptz,
  resolved_at     timestamptz,
  closed_at       timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tickets_status    ON tickets (tenant_id, status, created_at DESC);
CREATE INDEX idx_tickets_assigned  ON tickets (assigned_to, status);
CREATE INDEX idx_tickets_submitter ON tickets (submitter_user_id, status);

CREATE TABLE ticket_comments (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users(id),
  body        text NOT NULL,
  is_internal boolean NOT NULL DEFAULT false,  -- staff-only note, hidden from submitter
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_attachments (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  comment_id  uuid REFERENCES ticket_comments(id),
  media_id    uuid NOT NULL REFERENCES media(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE ticket_events (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
  user_id     uuid REFERENCES users(id),
  event_type  text NOT NULL,
  -- 'status_changed'|'priority_changed'|'assigned'|'comment_added'|'sla_breached'
  old_value   text,
  new_value   text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

## Status State Machine

```
open → in_progress → waiting_on_user → in_progress → resolved → closed
open → spam
any state → closed (admin action)
```

## Auto-Assignment

When a ticket is submitted for a type with `auto_assign_group_id` set, it's assigned to the least-loaded active staff member in that group:

```go
func AutoAssignTicket(ctx context.Context, ticket *Ticket) error {
  groupMembers, _ := getActiveGroupMembers(ctx, ticket.TypeID)
  counts, _ := getOpenTicketCountsByAssignee(ctx, groupMembers)
  assignee := minBy(counts, func(m Member) int { return m.OpenCount })
  return assignTicket(ctx, ticket.ID, assignee.ID)
}
```

## SLA Tracking

Tickets with `sla_hours` set get a `sla_breach_at` computed at creation time (`now() + sla_hours`). The `maintenance:check_sla_breaches` task (low tier, every 15 min) finds unresolved tickets past their SLA and fires a `ticket:sla_breached` notification to the assigned agent and their group admin.

## User Portal (main site `/support`)

```
/support           — Submit new ticket (type selector → dynamic form via custom_field_group)
/support/my        — Logged-in user's ticket list with status badges
/support/my/:id    — Thread view: description + public comments + attachments
                     User can add replies (comments with is_internal=false)
                     Status updates shown as timeline events
```

## Admin Panel Interface

- Full ticket queue with bulk status updates, filters (type / status / priority / assignee / date), column sort, paginated (no infinite scroll per conventions)
- Ticket detail: full event timeline, internal notes, assignment panel, SLA countdown, custom field values
- Reports: tickets by type / status / resolution time, exportable as CSV

## Asynq Tasks

- `ticket:send_confirmation` (critical) — email to submitter on create
- `ticket:send_status_update` (default) — email on status change
- `maintenance:check_sla_breaches` (low, every 15 min)

## Form Builder Integration

The "Platform Feedback" form (Section 06, seeded at install) has a workflow trigger of type `create_ticket` — submissions create a ticket of type "Feedback" automatically.

## RBAC Permission Nodes

```
tickets.submit              — submit a ticket (default: all)
tickets.view_own            — view own tickets (default: all)
tickets.view_all            — view all tickets (staff only)
tickets.manage              — change status, priority, assignment
tickets.comment.internal    — add internal notes
tickets.admin               — delete, configure ticket types
```

## See Also

Section 06 (custom field groups for ticket types), Section 22 (admin panel queue), Section 35 (notifications on ticket events). ADR-038.
