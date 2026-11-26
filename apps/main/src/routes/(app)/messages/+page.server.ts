import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '#lib/server/api.js';
import { bookingsInboxEnabled } from '#lib/features.js';
import { isMainToolEnabled, MAIN_TOOLS } from '#lib/tools/registry.js';
import type {
	DMConversation,
	DMMessage,
	DMRequest,
	ServiceBooking,
	TicketListItem,
	TicketDetailResponse
} from '#lib/types/index.js';
import type { HubAiConversation } from '#lib/components/hub/hub.js';

/**
 * Communications Hub at /messages — auth-gated (feature.dm). One unified
 * stream unions the member's inboxes: member DMs + org-inbox threads
 * (direct/services), DM requests, service bookings, mentorship bookings,
 * support tickets and Japa AI conversations.
 *
 * URL params:
 *   q, filter=unread, sort=recent|oldest|unread|name, page_size — forwarded
 *     to GET /dm/conversations (other sources filter client-side).
 *   sel=<kind>:<id> — deep-link selection across sources (dm, request,
 *     booking, mentorship, ticket, ai). Legacy ?c=<uuid> still selects a DM.
 *
 * Ref: japaconnect-engineering/sections/48-direct-messages.md (DMs),
 * sections/38-ticket-system.md, sections/111-service-packages-bookings.md,
 * sections/102-mentorship-matching.md, adr/ADR-130-unified-japa-ai.md
 */
const CONVERSATION_SORTS = new Set(['recent', 'oldest', 'unread', 'name']);
const PAGE_SIZES = new Set([10, 20, 30, 50]);
const DEFAULT_PAGE_SIZE = 30;
const MESSAGES_PAGE_SIZE = 50;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
/** Rows fetched per secondary source — the stream merges them client-side. */
const SECONDARY_PAGE_SIZE = 20;

/** sel=<kind>:<id> — kind prefixes the HubItem chip mapping. */
function parseSelParam(raw: string | null): { kind: string; id: string } | null {
	if (!raw) return null;
	const idx = raw.indexOf(':');
	if (idx <= 0) return null;
	const kind = raw.slice(0, idx);
	const id = raw.slice(idx + 1);
	if (!UUID_RE.test(id)) return null;
	const valid = new Set(['dm', 'org', 'request', 'booking', 'mentorship', 'ticket', 'ai']);
	return valid.has(kind) ? { kind, id } : null;
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.dm']) {
		throw error(404, { code: 'feature_disabled', message: 'Direct Messages are not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/messages');
	}

	const ctx = apiContext(event);
	const params = event.url.searchParams;

	const q = params.get('q')?.trim() ?? '';
	const filter = params.get('filter') === 'unread' ? 'unread' : null;
	const sortParam = params.get('sort') ?? 'recent';
	const sort = CONVERSATION_SORTS.has(sortParam) ? sortParam : 'recent';
	const sizeParam = Number(params.get('page_size'));
	const pageSize = PAGE_SIZES.has(sizeParam) ? sizeParam : DEFAULT_PAGE_SIZE;

	// ?sel= is the unified selector; ?c= stays a valid DM deep link.
	const sel = parseSelParam(params.get('sel'));
	const legacyC = params.get('c');
	const selOrLegacy =
		sel ?? (legacyC && UUID_RE.test(legacyC) ? { kind: 'dm' as const, id: legacyC } : null);

	const convQuery = new URLSearchParams();
	convQuery.set('page_size', String(pageSize));
	if (q) convQuery.set('q', q);
	if (filter) convQuery.set('filter', filter);
	if (sort !== 'recent') convQuery.set('sort', sort);

	// Secondary sources are flag-gated the same way their standalone
	// surfaces are — a disabled capability contributes nothing to the
	// stream rather than erroring the whole page.
	const bookingsEnabled = bookingsInboxEnabled(features);
	const providerArmEnabled = !!features['mentorship.enabled'] || !!features['service_packages.enabled'];
	const japabotTool = MAIN_TOOLS.find((tl) => tl.labelKey === 'tools.japabot')!;
	const aiEnabled = isMainToolEnabled(japabotTool, features);

	let loadError = false;

	const [
		conversationsRes,
		requestsRes,
		bookingsSentRes,
		bookingsReceivedRes,
		ticketsRes,
		aiRes,
		selectedRes
	] = await Promise.all([
		apiList<DMConversation>(ctx, `/dm/conversations?${convQuery}`)
			.then((res) => ({ items: res.items, cursor: res.meta?.next_cursor ?? null }))
			.catch(() => {
				loadError = true;
				return { items: [] as DMConversation[], cursor: null };
			}),
		// Pending DM requests join the stream (assigned-to-me rows).
		apiList<DMRequest>(ctx, '/dm/requests?page_size=50')
			.then((res) => res.items)
			.catch(() => [] as DMRequest[]),
		bookingsEnabled
			? apiList<ServiceBooking>(
					ctx,
					`/me/service-bookings?role=booker&limit=${SECONDARY_PAGE_SIZE}&page=1`
				)
					.then((res) => res.items)
					.catch(() => [] as ServiceBooking[])
			: Promise.resolve([] as ServiceBooking[]),
		bookingsEnabled && providerArmEnabled
			? apiList<ServiceBooking>(
					ctx,
					`/me/service-bookings?role=provider&limit=${SECONDARY_PAGE_SIZE}&page=1`
				)
					.then((res) => res.items)
					.catch(() => [] as ServiceBooking[])
			: Promise.resolve([] as ServiceBooking[]),
		apiList<TicketListItem>(ctx, `/me/tickets?page_size=${SECONDARY_PAGE_SIZE}`)
			.then((res) => res.items)
			.catch(() => [] as TicketListItem[]),
		aiEnabled
			? apiList<HubAiConversation>(ctx, `/ai/conversations?page_size=${SECONDARY_PAGE_SIZE}`)
					.then((res) => res.items)
					.catch(() => [] as HubAiConversation[])
			: Promise.resolve([] as HubAiConversation[]),
		// The selected item's detail payload — resolved by kind.
		(async () => {
			if (!selOrLegacy) return null;
			const { kind, id } = selOrLegacy;
			try {
				if (kind === 'dm' || kind === 'org') {
					const res = await apiList<DMMessage>(
						ctx,
						`/dm/conversations/${encodeURIComponent(id)}/messages?page_size=${MESSAGES_PAGE_SIZE}`
					);
					// API returns newest-first; the thread renders oldest→newest.
					return { dmMessages: res.items.reverse(), dmCursor: res.meta?.next_cursor ?? null };
				}
				if (kind === 'booking' || kind === 'mentorship') {
					const booking = await apiGet<ServiceBooking>(
						ctx,
						`/me/service-bookings/${encodeURIComponent(id)}`
					);
					return { booking };
				}
				if (kind === 'ticket') {
					const ticketDetail = await apiGet<TicketDetailResponse>(
						ctx,
						`/me/tickets/${encodeURIComponent(id)}`
					);
					return { ticketDetail };
				}
				// request + ai selections need no server payload — request rows
				// are already in the list; the AI pane loads its transcript.
				return {};
			} catch (err) {
				if (err instanceof ApiError && (err.status === 404 || err.status === 403)) {
					return { selectedMissing: true };
				}
				return { selectedMissing: true };
			}
		})()
	]);

	const selPayload = selectedRes as {
		dmMessages?: DMMessage[];
		dmCursor?: string | null;
		booking?: ServiceBooking;
		ticketDetail?: TicketDetailResponse;
		selectedMissing?: boolean;
	} | null;

	return {
		conversations: conversationsRes.items,
		conversationsCursor: conversationsRes.cursor,
		requests: requestsRes,
		bookingsSent: bookingsSentRes,
		bookingsReceived: bookingsReceivedRes,
		tickets: ticketsRes,
		aiConversations: aiRes,
		sel: selOrLegacy,
		selMissing: selPayload?.selectedMissing ?? false,
		activeMessages: selPayload?.dmMessages ?? [],
		messagesCursor: selPayload?.dmCursor ?? null,
		selectedBooking: selPayload?.booking ?? null,
		ticketDetail: selPayload?.ticketDetail ?? null,
		loadError,
		query: { q, filter, sort, pageSize },
		messagesPageSize: MESSAGES_PAGE_SIZE
	};
};
