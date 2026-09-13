-- name: CreateTicketType :one
INSERT INTO ticket_types (name, slug, description, field_group_id, default_priority, auto_assign_group_id, sla_hours, is_public, is_system) VALUES (@name, @slug, sqlc.narg(description), sqlc.narg(field_group_id), @default_priority, sqlc.narg(auto_assign_group_id), sqlc.narg(sla_hours), @is_public, @is_system)
RETURNING id, name, slug, description, field_group_id, default_priority,
 auto_assign_group_id, sla_hours, is_public, is_system, created_at, updated_at;

-- name: GetTicketTypeByID :one
SELECT id, name, slug, description, field_group_id, default_priority,
 auto_assign_group_id, sla_hours, is_public, is_system, created_at, updated_at
FROM ticket_types
WHERE id = @id;

-- name: GetTicketTypeBySlug :one
SELECT id, name, slug, description, field_group_id, default_priority,
 auto_assign_group_id, sla_hours, is_public, is_system, created_at, updated_at
FROM ticket_types
WHERE slug = @slug;

-- name: ListTicketTypesByTenant :many
SELECT id, name, slug, description, field_group_id, default_priority,
 auto_assign_group_id, sla_hours, is_public, is_system, created_at, updated_at
FROM ticket_types
WHERE (sqlc.narg(is_public)::bool IS NULL OR is_public = sqlc.narg(is_public))
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountTicketTypesByTenant :one
SELECT COUNT(*) FROM ticket_types
WHERE (sqlc.narg(is_public)::bool IS NULL OR is_public = sqlc.narg(is_public));

-- name: UpdateTicketType :one
UPDATE ticket_types
SET name = @name, description = sqlc.narg(description),
 field_group_id = sqlc.narg(field_group_id), default_priority = @default_priority,
 auto_assign_group_id = sqlc.narg(auto_assign_group_id), sla_hours = sqlc.narg(sla_hours),
 is_public = @is_public, updated_at = now()
WHERE id = @id
RETURNING id, name, slug, description, field_group_id, default_priority,
 auto_assign_group_id, sla_hours, is_public, is_system, created_at, updated_at;

-- name: DeleteTicketType :exec
DELETE FROM ticket_types
WHERE id = @id;

-- name: CountTicketsByType :one
SELECT COUNT(*) FROM tickets
WHERE type_id = @type_id;

-- =====================
-- Tickets CRUD
-- =====================

-- name: CreateTicket :one
INSERT INTO tickets (type_id, subject, description, status, priority, submitter_user_id, submitter_email, assigned_to, assigned_group_id, field_values, sla_breach_at) VALUES (@type_id, @subject, @description, @status, @priority, sqlc.narg(submitter_user_id), sqlc.narg(submitter_email), sqlc.narg(assigned_to), sqlc.narg(assigned_group_id), @field_values, sqlc.narg(sla_breach_at))
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: GetTicketByID :one
SELECT t.id, t.type_id, t.subject, t.description, t.status, t.priority,
 t.submitter_user_id, t.submitter_email, t.assigned_to, t.assigned_group_id,
 t.resolution_note, t.field_values, t.sla_breach_at, t.resolved_at, t.closed_at,
 t.created_at, t.updated_at,
 tt.name AS type_name, tt.slug AS type_slug
FROM tickets t
JOIN ticket_types tt ON tt.id = t.type_id
WHERE t.id = @id;

-- name: ListTicketsByTenant :many
SELECT t.id, t.type_id, t.subject, t.description, t.status, t.priority,
 t.submitter_user_id, t.submitter_email, t.assigned_to, t.assigned_group_id,
 t.resolution_note, t.field_values, t.sla_breach_at, t.resolved_at, t.closed_at,
 t.created_at, t.updated_at,
 tt.name AS type_name, tt.slug AS type_slug
FROM tickets t
JOIN ticket_types tt ON tt.id = t.type_id
WHERE (@status::text IS NULL OR t.status = @status)
 AND (@priority::text IS NULL OR t.priority = @priority)
 AND (@type_id::uuid IS NULL OR t.type_id = @type_id)
 AND (@assigned_to::uuid IS NULL OR t.assigned_to = @assigned_to)
 AND (@search::text IS NULL OR t.subject ILIKE '%' || @search || '%')
ORDER BY t.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListTicketsBySubmitter :many
SELECT t.id, t.type_id, t.subject, t.description, t.status, t.priority,
 t.submitter_user_id, t.submitter_email, t.assigned_to, t.assigned_group_id,
 t.resolution_note, t.field_values, t.sla_breach_at, t.resolved_at, t.closed_at,
 t.created_at, t.updated_at,
 tt.name AS type_name, tt.slug AS type_slug
FROM tickets t
JOIN ticket_types tt ON tt.id = t.type_id
WHERE t.submitter_user_id = @submitter_user_id
 AND (sqlc.narg(status)::text IS NULL OR t.status = sqlc.narg(status))
ORDER BY t.created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: UpdateTicketStatus :one
UPDATE tickets
SET status = @status, updated_at = now(),
 resolved_at = CASE WHEN @status = 'resolved' THEN now() ELSE resolved_at END,
 closed_at = CASE WHEN @status = 'closed' THEN now() ELSE closed_at END
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: UpdateTicketPriority :one
UPDATE tickets
SET priority = @priority, updated_at = now()
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: AssignTicket :one
UPDATE tickets
SET assigned_to = sqlc.narg(assigned_to), assigned_group_id = sqlc.narg(assigned_group_id),
 updated_at = now()
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: UpdateTicketSubject :one
UPDATE tickets
SET subject = @subject, updated_at = now()
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: DeleteTicket :exec
DELETE FROM tickets
WHERE id = @id;

-- name: ResolveTicket :one
UPDATE tickets
SET status = 'resolved', resolution_note = sqlc.narg(resolution_note),
 resolved_at = now(), updated_at = now()
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: CloseTicket :one
UPDATE tickets
SET status = 'closed', closed_at = now(), updated_at = now()
WHERE id = @id
RETURNING id, type_id, subject, description, status, priority,
 submitter_user_id, submitter_email, assigned_to, assigned_group_id,
 resolution_note, field_values, sla_breach_at, resolved_at, closed_at,
 created_at, updated_at;

-- name: GetLeastLoadedStaffInGroup :one
SELECT u.id AS user_id
FROM users u
JOIN user_group_memberships ugm ON ugm.user_id = u.id
WHERE ugm.group_id = @group_id
 AND u.status = 'active'
ORDER BY (
 SELECT COUNT(*) FROM tickets t
 WHERE t.assigned_to = u.id AND t.status NOT IN ('resolved', 'closed', 'spam')
) ASC
LIMIT 1;

-- name: GetOpenTicketCountByAssignee :many
SELECT assigned_to, COUNT(*) AS open_count
FROM tickets
WHERE assigned_to IS NOT NULL
 AND status NOT IN ('resolved', 'closed', 'spam')
GROUP BY assigned_to;

-- =====================
-- Ticket Comments
-- =====================

-- name: CreateTicketComment :one
INSERT INTO ticket_comments (ticket_id, user_id, body, is_internal) VALUES (@ticket_id, @user_id, @body, @is_internal)

RETURNING id, ticket_id, user_id, body, is_internal, created_at, updated_at;

-- name: ListTicketComments :many
SELECT id, ticket_id, user_id, body, is_internal, created_at, updated_at
FROM ticket_comments
WHERE ticket_id = @ticket_id
 AND (@include_internal::bool = true OR is_internal = false)
ORDER BY created_at ASC;

-- name: DeleteTicketComment :exec
DELETE FROM ticket_comments
WHERE id = @id AND ticket_id = @ticket_id;

-- =====================
-- Ticket Attachments
-- =====================

-- name: CreateTicketAttachment :one
INSERT INTO ticket_attachments (ticket_id, comment_id, media_id) VALUES (@ticket_id, sqlc.narg(comment_id), @media_id)

RETURNING id, ticket_id, comment_id, media_id, created_at;

-- name: ListTicketAttachments :many
SELECT id, ticket_id, comment_id, media_id, created_at
FROM ticket_attachments
WHERE ticket_id = @ticket_id
ORDER BY created_at ASC;

-- =====================
-- Ticket Events (Audit Trail)
-- =====================

-- name: CreateTicketEvent :one
INSERT INTO ticket_events (ticket_id, user_id, event_type, old_value, new_value) VALUES (@ticket_id, sqlc.narg(user_id), @event_type, sqlc.narg(old_value), sqlc.narg(new_value))

RETURNING id, ticket_id, user_id, event_type, old_value, new_value, created_at;

-- name: ListTicketEvents :many
SELECT id, ticket_id, user_id, event_type, old_value, new_value, created_at
FROM ticket_events
WHERE ticket_id = @ticket_id
ORDER BY created_at ASC;

-- name: CountTicketsByStatus :one
SELECT COUNT(*) AS count
FROM tickets
WHERE status = @status;

-- name: CountTicketsByTenant :one
SELECT COUNT(*) FROM tickets
WHERE (@status::text IS NULL OR status = @status)
 AND (@priority::text IS NULL OR priority = @priority)
 AND (@type_id::uuid IS NULL OR type_id = @type_id)
 AND (@assigned_to::uuid IS NULL OR assigned_to = @assigned_to)
 AND (@search::text IS NULL OR subject ILIKE '%' || @search || '%');

-- name: UpdateTicketSLABreach :exec
UPDATE tickets
SET sla_breach_at = now(), updated_at = now()
WHERE id = @id AND sla_breach_at IS NULL AND status NOT IN ('resolved', 'closed', 'spam');

-- name: ListSLABreachedTickets :many
SELECT id, subject, sla_breach_at, assigned_to
FROM tickets
WHERE sla_breach_at IS NOT NULL
 AND sla_breach_at < now()
 AND status NOT IN ('resolved', 'closed')
ORDER BY sla_breach_at ASC;

-- name: AutoCloseResolvedTickets :execrows
-- Closes resolved tickets older than the configured auto-close window.
-- Called by maintenance:check_sla_breaches using tickets.auto_close_resolved_days.
UPDATE tickets
SET status = 'closed', closed_at = now(), updated_at = now()
WHERE status = 'resolved'
 AND resolved_at IS NOT NULL
 AND resolved_at < now() - (@days || ' days')::interval;
