// Ref: japaconnect-engineering/sections/38-tickets.md (user portal)
// Support inbox at /messages/support — the tickets surface inside the
// Messages tab family (the Communications Hub's Support chip links here
// for the full list). Auth-gated on feature.tickets like the dashboard
// surface (routeFeatureFlags); the backend returns only tickets where
// submitter_user_id = the caller.
//
// URL params: status, page, t=<ticket id> (deep-link — opens the thread
// pane). Same lookahead pagination as /dashboard/tickets: the endpoint
// returns no meta total, so fetch page_size+1 and slice.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import type { TicketDetailResponse, TicketListItem } from '#lib/types/index.js';

const DEFAULT_PAGE_SIZE = 20;
const MAX_PAGE_SIZE = 50;
const VALID_STATUSES = new Set([
	'open',
	'in_progress',
	'waiting_on_user',
	'resolved',
	'closed',
	'spam'
]);
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (features && features['feature.tickets'] === false) {
		throw error(404, { code: 'feature_disabled', message: 'Support tickets are not available' });
	}

	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const params = event.url.searchParams;
	const rawStatus = params.get('status')?.trim() ?? '';
	const status = VALID_STATUSES.has(rawStatus) ? rawStatus : '';
	const page = Math.max(1, Math.floor(Number(params.get('page'))) || 1);
	const pageSize = Math.min(
		MAX_PAGE_SIZE,
		Math.max(
			1,
			Math.floor(Number(params.get('page_size')) || DEFAULT_PAGE_SIZE) || DEFAULT_PAGE_SIZE
		)
	);
	const selectedId = params.get('t') ?? '';

	// +1 lookahead: no meta total on the response — a full fetched page
	// beyond pageSize is the only "next page exists" signal.
	const q = new URLSearchParams({
		page_size: String(Math.min(MAX_PAGE_SIZE, pageSize + 1)),
		page_offset: String((page - 1) * pageSize)
	});
	if (status) q.set('status', status);

	const ctx = apiContext(event);
	let tickets: TicketListItem[] = [];
	let fetched = 0;
	let loadError = false;
	try {
		const res = await apiList<TicketListItem>(ctx, `/me/tickets?${q.toString()}`);
		tickets = res.items.slice(0, pageSize);
		fetched = res.items.length;
	} catch {
		loadError = true;
	}

	let selected: TicketDetailResponse | null = null;
	let selectedMissing = false;
	if (selectedId && UUID_RE.test(selectedId)) {
		try {
			selected = await apiGet<TicketDetailResponse>(
				ctx,
				`/me/tickets/${encodeURIComponent(selectedId)}`
			);
		} catch {
			selectedMissing = true;
		}
	} else if (selectedId) {
		selectedMissing = true;
	}

	return {
		tickets,
		hasMore: fetched > pageSize,
		page,
		pageSize,
		status,
		selected,
		selectedMissing,
		loadError
	};
};
