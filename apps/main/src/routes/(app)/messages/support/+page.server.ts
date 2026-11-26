// Ref: japaconnect-engineering/sections/38-tickets.md (user portal),
//      japaconnect-engineering/sections/22-admin-panel.md (Appeals)
// Support inbox at /messages/support — the whole member support surface
// inside the Messages tab family: tickets (list → thread pane) AND
// appeals (eligible decisions + submitted appeals → detail pane).
// Auth-gated on feature.tickets like the old dashboard surface
// (routeFeatureFlags); the backend returns only the caller's own rows.
//
// URL params: view=tickets|appeals, status, page,
//             t=<ticket id|JC-n> (ticket deep-link — thread pane),
//             a=<appeal id> (appeal deep-link — detail pane).
// Same lookahead pagination as /dashboard/tickets: neither endpoint
// returns a meta total, so fetch page_size+1 and slice.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '#lib/server/api.js';
import type {
	AppealEligibleItem,
	AppealListItem,
	TicketDetailResponse,
	TicketListItem
} from '#lib/types/index.js';

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
// Ticket refs — uuid or the public JC-<n> form (the backend resolves
// either, same as the old /dashboard/tickets/[id] route).
const TICKET_REF_RE = /^(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|JC-\d+)$/i;

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
	const view = params.get('view') === 'appeals' ? 'appeals' : 'tickets';
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
	const selectedAppealId = params.get('a') ?? '';

	// +1 lookahead: no meta total on the response — a full fetched page
	// beyond pageSize is the only "next page exists" signal.
	const q = new URLSearchParams({
		page_size: String(Math.min(MAX_PAGE_SIZE, pageSize + 1)),
		page_offset: String((page - 1) * pageSize)
	});
	if (status) q.set('status', status);

	const ctx = apiContext(event);

	if (view === 'appeals') {
		// §22 — the user's appeals + every decision they can currently
		// appeal. Best-effort on both lists like the dashboard surface:
		// a failure degrades to an empty section rather than 500ing.
		const [appealRes, eligible] = await Promise.all([
			apiList<AppealListItem>(ctx, `/me/appeals?${q.toString()}`)
				.then((r) => r.items)
				.catch(() => [] as AppealListItem[]),
			apiList<AppealEligibleItem>(ctx, '/me/appeals/eligible')
				.then((r) => r.items)
				.catch(() => [] as AppealEligibleItem[])
		]);
		const appeals = appealRes.slice(0, pageSize);
		return {
			view,
			tickets: [],
			appeals,
			eligible,
			selectedAppeal: appeals.find((a) => a.id === selectedAppealId) ?? null,
			userStatus: event.locals.user.status ?? 'active',
			hasMore: appealRes.length > pageSize,
			page,
			pageSize,
			status: '',
			selected: null,
			selectedMissing: false,
			loadError: false
		};
	}

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
	if (selectedId && TICKET_REF_RE.test(selectedId)) {
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
		view,
		tickets,
		appeals: [],
		eligible: [],
		selectedAppeal: null,
		userStatus: 'active',
		hasMore: fetched > pageSize,
		page,
		pageSize,
		status,
		selected,
		selectedMissing,
		loadError
	};
};
