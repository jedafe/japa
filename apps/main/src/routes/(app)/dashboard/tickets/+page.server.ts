// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Lists the current user's submitted tickets. Auth-gated; the backend returns
// only tickets where submitter_user_id = the caller.
// ?status= filters server-side (open|in_progress|waiting_on_user|resolved|
// closed|spam); the endpoint returns no meta total so we fetch page_size+1
// and slice to derive hasMore — the same lookahead pattern the relationships
// loader uses.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { TicketListItem } from '$lib/types';

const DEFAULT_PAGE_SIZE = 25;
const MAX_PAGE_SIZE = 100;
const VALID_STATUSES = new Set(['open', 'in_progress', 'waiting_on_user', 'resolved', 'closed', 'spam']);

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const rawStatus = event.url.searchParams.get('status')?.trim() ?? '';
	const status = VALID_STATUSES.has(rawStatus) ? rawStatus : '';
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(
		MAX_PAGE_SIZE,
		Math.max(1, parseInt(event.url.searchParams.get('page_size') ?? String(DEFAULT_PAGE_SIZE), 10) || DEFAULT_PAGE_SIZE)
	);
	const pageOffset = (page - 1) * pageSize;

	// +1 lookahead: the response has no meta total, so a full fetched page
	// beyond pageSize is the only "next page exists" signal. fetchSize is
	// capped at the backend max — at pageSize=100 hasMore stays false.
	const fetchSize = Math.min(MAX_PAGE_SIZE, pageSize + 1);
	const params = new URLSearchParams({
		page_size: String(fetchSize),
		page_offset: String(pageOffset)
	});
	if (status) params.set('status', status);

	const ctx = apiContext(event);
	const items = (await apiList<TicketListItem>(ctx, `/me/tickets?${params.toString()}`)).items;
	return {
		tickets: items.slice(0, pageSize),
		hasMore: items.length > pageSize,
		page,
		pageSize,
		status
	};
};
