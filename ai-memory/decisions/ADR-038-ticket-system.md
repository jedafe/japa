# ADR-038: Ticket System

**Status:** Accepted

**Context:** The platform needed a support/helpdesk system covering user-submitted tickets, bug reports, and feature requests, with SLA tracking, internal notes, auto-assignment, and admin bulk operations.

**Decision:** Full ticket system with `ticket_types` (admin-configurable, with custom field groups from Section 06), `tickets` (status machine: open→in_progress→waiting_on_user→resolved→closed|spam), `ticket_comments` (with `is_internal` flag for staff-only notes), `ticket_attachments`, and `ticket_events` (full audit trail). Auto-assignment to least-loaded staff member in the type's assigned UserGroup. SLA tracked via `sla_breach_at` computed at creation time; `maintenance:check_sla_breaches` task monitors every 15 minutes. The feedback Form Builder (Section 06) has a workflow trigger `create_ticket` — feedback form submissions automatically create tickets. User-facing portal at `/support`; staff interface in Admin Panel queue (paginated, filterable, bulk actions).

**Rationale:** Integrates with the existing Custom Field Architecture (Section 06) for type-specific fields, the Notification System (Section 35) for status update emails, and the Form Builder for seamless feedback → ticket conversion. Staff queue follows the Admin Panel convention of filter/sort/paginate with no infinite scroll.

**Consequences:** SLA monitoring adds one always-running low-tier Asynq task. Ticket type deletion requires a check for existing tickets using that type — cannot be deleted if tickets exist.
