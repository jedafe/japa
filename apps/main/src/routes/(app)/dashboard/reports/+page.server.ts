// My Reports at /dashboard/reports — lists the current user's submitted
// content reports and their statuses. Auth-gated; the backend returns only
// reports where reporter_user_id = the caller (GET /me/reports, Section 51).
//
// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MyReport } from '$lib/types';

const DEFAULT_PAGE_SIZE = 20;
const MAX_PAGE_SIZE = 100;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const pageSize = Math.min(
		MAX_PAGE_SIZE,
		Math.max(1, parseInt(event.url.searchParams.get('page_size') ?? String(DEFAULT_PAGE_SIZE), 10) || DEFAULT_PAGE_SIZE)
	);

	// The endpoint returns no meta total — fetch one extra row to detect a
	// next page, then slice back down (lookahead pattern).
	const params = new URLSearchParams({
		page_size: String(Math.min(MAX_PAGE_SIZE, pageSize + 1)),
		page_offset: String((page - 1) * pageSize)
	});

	const ctx = apiContext(event);
	let reports: MyReport[] = [];
	let hasMore = false;
	let loadError = false;
	try {
		const items = (await apiList<MyReport>(ctx, `/me/reports?${params.toString()}`)).items;
		hasMore = items.length > pageSize;
		reports = items.slice(0, pageSize);
	} catch {
		loadError = true;
	}

	return { reports, hasMore, page, pageSize, loadError };
};
