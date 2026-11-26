// Communications Hub — unified conversation stream model.
// Ref: japaconnect-engineering/sections/48-direct-messages.md (DMs),
// sections/38-ticket-system.md (support tickets), sections/111-service-
// packages-bookings.md (service + mentorship bookings), ADR-130 (AI convos).
//
// One stream unions the member's inboxes: member DMs (direct), org-inbox
// conversations (services), service bookings, mentorship bookings, support
// tickets and Japa AI threads. `kind` is the source; `chip` is the filter
// bucket the UI groups it under.
import type {
	DMConversation,
	DMRequest,
	ServiceBooking,
	TicketListItem,
	TicketStatus,
	TicketPriority
} from '#lib/types/index.js';
import { ticketRef } from '#lib/tickets.js';

export type HubKind = 'dm' | 'org' | 'request' | 'booking' | 'mentorship' | 'ticket' | 'ai';
export type HubChip = 'all' | 'direct' | 'services' | 'mentorship' | 'support' | 'ai';
export type HubTab = 'all' | 'unread' | 'assigned';

/** AI conversation list row — GET /ai/conversations (ADR-130). */
export interface HubAiConversation {
	conversation_id: string;
	title?: string;
	preview: string;
	turn_count: number;
	last_entry_at: string;
	legacy?: boolean;
}

export interface HubItem {
	kind: HubKind;
	/** each-block key + `sel=` param value, e.g. `dm:<uuid>`. */
	sel: string;
	chip: Exclude<HubChip, 'all'>;
	title: string;
	/** Secondary line — @handle, listing title, ticket ref. */
	subtitle: string;
	avatarUrl: string;
	/** Fallback avatar when no image: icon key rendered by HubListItem. */
	avatarIcon: 'user' | 'org' | 'headset' | 'sparkles' | 'users';
	verified: boolean;
	verificationType: string | null;
	verifiedScope: 'user' | 'organization';
	preview: string;
	/** Sort + display timestamp (last activity). */
	at: string | null;
	unread: number;
	/** Status pill — ticket status, booking status, "Request". */
	statusLabel: string;
	statusTone: 'info' | 'success' | 'warning' | 'danger' | 'muted' | 'primary';
	/** Presence-backed online flag (member DMs only). */
	onlineUserId: string;
	/** "Assigned to me": the item awaits the viewer's action. */
	assigned: boolean;
	// Source payloads for the thread panes.
	dm?: DMConversation;
	request?: DMRequest;
	booking?: ServiceBooking;
	bookingRole: 'sent' | 'received';
	ticket?: TicketListItem;
	ai?: HubAiConversation;
}

export interface Translator {
	(key: string, fallback?: string, vars?: Record<string, string | number>): string;
}

const TICKET_STATUS_KEY: Record<TicketStatus, string> = {
	open: 'tickets.status.open',
	in_progress: 'tickets.status.in_progress',
	waiting_on_user: 'tickets.status.waiting_on_user',
	resolved: 'tickets.status.resolved',
	closed: 'tickets.status.closed',
	spam: 'tickets.status.spam'
};

function ticketStatusTone(status: TicketStatus): HubItem['statusTone'] {
	switch (status) {
		case 'open':
			return 'success';
		case 'in_progress':
			return 'warning';
		case 'waiting_on_user':
			return 'primary';
		case 'resolved':
		case 'closed':
			return 'muted';
		default:
			return 'muted';
	}
}

// Booking statuses reuse the bookings.status_* i18n keys; mentorship-kind
// rows map to the mentors.status_* vocabulary (see bookingDisplay.ts).

function bookingTone(status: string): HubItem['statusTone'] {
	switch (status) {
		case 'pending':
			return 'warning';
		case 'confirmed':
			return 'info';
		case 'fulfilled':
			return 'success';
		case 'disputed':
		case 'refunded':
			return 'danger';
		default:
			return 'muted';
	}
}

/** Latest of the booking's lifecycle timestamps, for stream ordering. */
function bookingTimestamp(b: ServiceBooking): string {
	return (
		b.fulfilled_at ?? b.confirmed_at ?? b.cancelled_at ?? b.created_at
	);
}

function bookingCounterpartName(b: ServiceBooking, role: 'sent' | 'received'): string {
	if (role === 'received') {
		return b.booker_display_name || b.booker_username || '';
	}
	return (
		b.provider_display_name ||
		b.provider_username ||
		b.org_name ||
		b.listing_title ||
		''
	);
}

function bookingCounterpartAvatar(b: ServiceBooking, role: 'sent' | 'received'): string {
	return role === 'received' ? (b.booker_avatar ?? '') : (b.provider_avatar ?? '');
}

export function dmToHubItem(c: DMConversation): HubItem {
	const isOrg = c.conversation_type === 'org';
	return {
		kind: isOrg ? 'org' : 'dm',
		sel: `dm:${c.id}`,
		chip: isOrg ? 'services' : 'direct',
		title: c.other_display_name,
		subtitle: c.other_username ? `@${c.other_username}` : '',
		avatarUrl: c.other_avatar_url ?? '',
		avatarIcon: isOrg ? 'org' : 'user',
		verified: !!c.other_verified,
		verificationType: c.other_verification_type ?? null,
		verifiedScope: isOrg ? 'organization' : 'user',
		preview: c.last_message_preview ?? '',
		at: c.last_message_at ?? c.created_at,
		unread: c.unread_count ?? 0,
		statusLabel: '',
		statusTone: 'muted',
		onlineUserId: isOrg ? '' : c.other_user_id,
		assigned: false,
		dm: c,
		bookingRole: 'sent'
	};
}

export function requestToHubItem(r: DMRequest): HubItem {
	return {
		kind: 'request',
		sel: `request:${r.id}`,
		chip: 'direct',
		title: r.requester_display_name,
		subtitle: `@${r.requester_username}`,
		avatarUrl: r.requester_avatar_url ?? '',
		avatarIcon: 'user',
		verified: !!r.requester_verified,
		verificationType: r.requester_verification_type ?? null,
		verifiedScope: 'user',
		preview: r.preview_message,
		at: r.created_at,
		unread: 1,
		statusLabel: '',
		statusTone: 'muted',
		onlineUserId: r.requester_id,
		assigned: true,
		request: r,
		bookingRole: 'sent'
	};
}

export function bookingToHubItem(b: ServiceBooking, role: 'sent' | 'received'): HubItem {
	const mentorship = b.listing_kind === 'mentorship';
	const statusKey = b.connection_status ?? b.status;
	return {
		kind: mentorship ? 'mentorship' : 'booking',
		sel: `booking:${b.id}`,
		chip: mentorship ? 'mentorship' : 'services',
		title: bookingCounterpartName(b, role) || b.listing_title,
		subtitle: b.listing_title ?? '',
		avatarUrl: bookingCounterpartAvatar(b, role),
		avatarIcon: mentorship ? 'user' : 'org',
		verified:
			role === 'received'
				? !!b.booker_verified
				: !!(b.provider_verified || b.org_verified),
		verificationType:
			role === 'received'
				? (b.booker_verification_type ?? null)
				: (b.provider_verification_type ?? b.org_verification_type ?? null),
		verifiedScope: b.org_id && !b.provider_user_id ? 'organization' : 'user',
		preview: b.note ?? b.goals ?? b.listing_title ?? '',
		at: bookingTimestamp(b),
		unread: 0,
		statusLabel: statusKey,
		statusTone: bookingTone(statusKey),
		onlineUserId: role === 'received' ? (b.booker_user_id ?? '') : (b.provider_user_id ?? ''),
		// Provider-side pending rows are leads awaiting the viewer's response.
		assigned: role === 'received' && statusKey === 'pending',
		booking: b,
		bookingRole: role,
		ticket: undefined
	};
}

export function ticketToHubItem(tk: TicketListItem, t: Translator): HubItem {
	return {
		kind: 'ticket',
		sel: `ticket:${tk.id}`,
		chip: 'support',
		title: tk.subject,
		subtitle: `${ticketRef(tk)} · ${tk.type_name}`,
		avatarUrl: '',
		avatarIcon: 'headset',
		verified: false,
		verificationType: null,
		verifiedScope: 'organization',
		preview: tk.description ?? '',
		at: tk.updated_at || tk.created_at,
		unread: tk.status === 'waiting_on_user' ? 1 : 0,
		statusLabel: t(TICKET_STATUS_KEY[tk.status] ?? 'tickets.status.open', tk.status),
		statusTone: ticketStatusTone(tk.status),
		onlineUserId: '',
		assigned: tk.status === 'waiting_on_user',
		ticket: tk,
		bookingRole: 'sent'
	};
}

export function aiToHubItem(c: HubAiConversation, t: Translator): HubItem {
	return {
		kind: 'ai',
		sel: `ai:${c.conversation_id}`,
		chip: 'ai',
		title: c.title?.trim() || t('hub.ai_fallback_title', 'Japa AI'),
		subtitle: t('hub.ai_subtitle', 'AI assistant'),
		avatarUrl: '',
		avatarIcon: 'sparkles',
		verified: false,
		verificationType: null,
		verifiedScope: 'user',
		preview: c.preview ?? '',
		at: c.last_entry_at ?? null,
		unread: 0,
		statusLabel: '',
		statusTone: 'muted',
		onlineUserId: '',
		assigned: false,
		ai: c,
		bookingRole: 'sent'
	};
}

/** Merge + sort the stream by activity desc (server already orders DMs). */
export function sortHubItems(items: HubItem[], sort: string): HubItem[] {
	const time = (i: HubItem) => (i.at ? new Date(i.at).getTime() : 0);
	const copy = [...items];
	switch (sort) {
		case 'oldest':
			return copy.sort((a, b) => time(a) - time(b));
		case 'name':
			return copy.sort((a, b) => a.title.localeCompare(b.title));
		case 'unread':
			return copy.sort((a, b) => b.unread - a.unread || time(b) - time(a));
		default:
			return copy.sort((a, b) => time(b) - time(a));
	}
}

/** Chip filter — 'all' passes everything through. */
export function chipMatches(item: HubItem, chip: HubChip): boolean {
	if (chip === 'all') return true;
	return item.chip === chip;
}

/** Tab filter — unread = has an unread marker; assigned = awaits my action. */
export function tabMatches(item: HubItem, tab: HubTab): boolean {
	if (tab === 'unread') return item.unread > 0;
	if (tab === 'assigned') return item.assigned;
	return true;
}

/** Client-side search for non-DM sources (the DM list is searched server-side). */
export function itemMatchesQuery(item: HubItem, q: string): boolean {
	if (!q) return true;
	const needle = q.toLowerCase();
	return (
		item.title.toLowerCase().includes(needle) ||
		item.subtitle.toLowerCase().includes(needle) ||
		item.preview.toLowerCase().includes(needle)
	);
}

/** Parse `sel=<kind>:<id>` (+ legacy `?c=<uuid>` for DM deep links). */
export function parseSel(raw: string | null): { kind: HubKind; id: string } | null {
	if (!raw) return null;
	const idx = raw.indexOf(':');
	if (idx <= 0) return null;
	const kind = raw.slice(0, idx) as HubKind;
	const id = raw.slice(idx + 1);
	const valid: HubKind[] = ['dm', 'org', 'request', 'booking', 'mentorship', 'ticket', 'ai'];
	return valid.includes(kind) && id ? { kind, id } : null;
}

const PRIORITY_KEY: Record<TicketPriority, string> = {
	low: 'tickets.priority.low',
	normal: 'tickets.priority.normal',
	high: 'tickets.priority.high',
	urgent: 'tickets.priority.urgent'
};

export function ticketStatusLabel(status: TicketStatus, t: Translator): string {
	return t(TICKET_STATUS_KEY[status] ?? 'tickets.status.open', status);
}

export function ticketPriorityLabel(p: TicketPriority, t: Translator): string {
	return t(PRIORITY_KEY[p] ?? 'tickets.priority.normal', p);
}

export function bookingStatusLabel(status: string, t: Translator): string {
	if (status === 'active' || status === 'completed') {
		return t(`mentors.status_${status}`, status);
	}
	return t(`bookings.status_${status}`, status);
}

/** Human-readable file size for attachment cards (2.4 MB, 860 KB). */
export function formatBytes(bytes: number | undefined): string {
	if (!bytes || bytes <= 0) return '';
	const units = ['B', 'KB', 'MB', 'GB'];
	let n = bytes;
	let u = 0;
	while (n >= 1024 && u < units.length - 1) {
		n /= 1024;
		u++;
	}
	return `${n >= 10 || u === 0 ? Math.round(n) : n.toFixed(1)} ${units[u]}`;
}
