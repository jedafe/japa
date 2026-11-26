// Ref: japaconnect-engineering/sections/38-ticket-system.md (ticket numbers)

interface TicketRef {
	id: string;
	ticket_number?: number | null;
}

/** Human-readable ticket reference — `JC-<n>`. Falls back to the UUID
 * prefix for rows predating ticket_number. */
export function ticketRef(t: TicketRef): string {
	return t.ticket_number ? `JC-${t.ticket_number}` : `#${t.id.slice(0, 8)}`;
}

/** Detail route for a ticket — the support workbench thread pane
 * (/messages/support?t=<ref>). JC-<n> when the number exists (the
 * backend resolves either form). */
export function ticketHref(t: TicketRef, base = '/messages/support'): string {
	return `${base}?t=${encodeURIComponent(t.ticket_number ? `JC-${t.ticket_number}` : t.id)}`;
}
