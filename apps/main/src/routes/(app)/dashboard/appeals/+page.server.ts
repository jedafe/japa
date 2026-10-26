// Ref: japaconnect-engineering/sections/22-admin-panel.md (Appeals)
// Lists the current user's submitted appeals (GET /me/appeals). Auth-gated;
// the backend returns only appeals where user_id = the caller. Same +1
// lookahead pattern as the tickets loader — the endpoint returns no meta
// total.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { AppealListItem, AppealEligibleItem } from '$lib/types';

const DEFAULT_PAGE_SIZE = 25;
const MAX_PAGE_SIZE = 100;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(
		MAX_PAGE_SIZE,
		Math.max(
			1,
			parseInt(event.url.searchParams.get('page_size') ?? String(DEFAULT_PAGE_SIZE), 10) ||
				DEFAULT_PAGE_SIZE
		)
	);
	const pageOffset = (page - 1) * pageSize;

	const fetchSize = Math.min(MAX_PAGE_SIZE, pageSize + 1);
	const params = new URLSearchParams({
		page_size: String(fetchSize),
		page_offset: String(pageOffset)
	});

	const ctx = apiContext(event);
	const [items, eligible] = await Promise.all([
		apiList<AppealListItem>(ctx, `/me/appeals?${params.toString()}`).then((r) => r.items),
		// §22 discovery surface — every decision the caller can currently
		// appeal. Best-effort: a failure degrades to an empty picker rather
		// than failing the page (the list itself still renders).
		apiList<AppealEligibleItem>(ctx, '/me/appeals/eligible')
			.then((r) => r.items)
			.catch(() => [] as AppealEligibleItem[])
	]);
	return {
		appeals: items.slice(0, pageSize),
		eligible,
		userStatus: event.locals.user.status ?? 'active',
		hasMore: items.length > pageSize,
		page,
		pageSize
	};
};
