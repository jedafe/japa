-- Section 38: Ticket System

-- =====================
-- Ticket Types
-- =====================

CREATE TABLE ticket_types (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text,
    field_group_id  uuid REFERENCES custom_field_groups(id),
    default_priority text NOT NULL DEFAULT 'normal',
    auto_assign_group_id uuid REFERENCES user_groups(id),
    sla_hours       int,
    is_public       boolean NOT NULL DEFAULT true,
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);
CREATE INDEX idx_ticket_types_tenant ON ticket_types(tenant_id);

-- =====================
-- Tickets
-- =====================

CREATE TABLE tickets (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    type_id         uuid NOT NULL REFERENCES ticket_types(id),
    subject         text NOT NULL,
    description     text NOT NULL,
    status          text NOT NULL DEFAULT 'open',
    priority        text NOT NULL DEFAULT 'normal',
    submitter_user_id uuid REFERENCES users(id),
    submitter_email text,
    assigned_to     uuid REFERENCES users(id),
    assigned_group_id uuid REFERENCES user_groups(id),
    resolution_note text,
    field_values    jsonb NOT NULL DEFAULT '{}',
    sla_breach_at   timestamptz,
    resolved_at     timestamptz,
    closed_at       timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_tickets_status ON tickets (tenant_id, status, created_at DESC);
CREATE INDEX idx_tickets_assigned ON tickets (assigned_to, status);
CREATE INDEX idx_tickets_submitter ON tickets (submitter_user_id, status);
CREATE INDEX idx_tickets_type ON tickets (type_id, status);

-- =====================
-- Ticket Comments
-- =====================

CREATE TABLE ticket_comments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id     uuid NOT NULL REFERENCES users(id),
    body        text NOT NULL,
    is_internal boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_comments_ticket ON ticket_comments(ticket_id, created_at);

-- =====================
-- Ticket Attachments
-- =====================

CREATE TABLE ticket_attachments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    comment_id  uuid REFERENCES ticket_comments(id),
    media_id    uuid NOT NULL REFERENCES media(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- =====================
-- Ticket Events (Audit Trail)
-- =====================

CREATE TABLE ticket_events (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id   uuid NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id     uuid REFERENCES users(id),
    event_type  text NOT NULL,
    old_value   text,
    new_value   text,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_ticket_events_ticket ON ticket_events(ticket_id, created_at);
