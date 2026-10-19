import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { DMConversation, DMMessage, DMRequest } from '$lib/types';

/**
 * Messages at /messages — auth-gated DM surface (feature.dm). Renders the
 * conversation list; the active conversation's messages are fetched either
 * server-side (when ?c= is present) or client-side on selection.
 *
 * URL params forwarded to GET /dm/conversations: q, filter=unread,
 * sort=recent|oldest|unread|name, page_size. Conversation and message lists
 * are keyset-paginated — the load returns the opaque next_cursor values the
 * client passes back as ?cursor= for subsequent pages (§48).
 *
 * Ref: japaconnect-engineering/sections/48-direct-messages.md
 */
const CONVERSATION_SORTS = new Set(['recent', 'oldest', 'unread', 'name']);
const PAGE_SIZES = new Set([10, 20, 30, 50]);
const DEFAULT_PAGE_SIZE = 30;
const MESSAGES_PAGE_SIZE = 50;
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

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
	// ?c= is a raw user param — validate it as a conversation UUID before it
	// hits an API path (an invalid value is treated as no active
	// conversation rather than smuggled into the request URL).
	const rawActiveId = params.get('c');
	const activeId = rawActiveId && UUID_RE.test(rawActiveId) ? rawActiveId : null;

	const convQuery = new URLSearchParams();
	convQuery.set('page_size', String(pageSize));
	if (q) convQuery.set('q', q);
	if (filter) convQuery.set('filter', filter);
	if (sort !== 'recent') convQuery.set('sort', sort);

	let loadError = false;
	const [conversationsRes, activeRes, requestCount] = await Promise.all([
		apiList<DMConversation>(ctx, `/dm/conversations?${convQuery}`)
			.then((res) => ({
				items: res.items,
				cursor: res.meta?.next_cursor ?? null
			}))
			.catch(() => {
				loadError = true;
				return { items: [] as DMConversation[], cursor: null };
			}),
		activeId
			? apiList<DMMessage>(
					ctx,
					`/dm/conversations/${encodeURIComponent(activeId)}/messages?page_size=${MESSAGES_PAGE_SIZE}`
				)
					.then((res) => ({
						// The API returns newest-first; the thread renders oldest→newest.
						items: res.items.reverse(),
						cursor: res.meta?.next_cursor ?? null
					}))
					.catch(() => ({ items: [] as DMMessage[], cursor: null }))
			: Promise.resolve({ items: [] as DMMessage[], cursor: null }),
		// Pending-request count drives the Requests tab badge.
		apiList<DMRequest>(ctx, '/dm/requests?page_size=50')
			.then((res) => res.items.length)
			.catch(() => 0)
	]);
	const conversations = conversationsRes.items;
	const conversationsCursor = conversationsRes.cursor;
	const activeMessages = activeRes.items;
	const messagesCursor = activeRes.cursor;

	return {
		conversations,
		activeId,
		activeMessages,
		conversationsCursor,
		messagesCursor,
		requestCount,
		loadError,
		query: { q, filter, sort, pageSize },
		messagesPageSize: MESSAGES_PAGE_SIZE
	};
};
