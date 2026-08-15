// My Reports at /dashboard/reports — lists the current user's submitted
// content reports and their statuses. Auth-gated; the backend returns only
// reports where reporter_user_id = the caller (GET /me/reports, Section 51).
//
// Ref: japaconnect-engineering/sections/51-content-reports-flagging.md
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { MyReport } from '$lib/types';

const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);

	let reports: MyReport[] = [];
	try {
		reports = (
			await apiList<MyReport>(ctx, `/me/reports?page_size=${PAGE_SIZE}`)
		).items;
	} catch {
		reports = [];
	}

	return { reports };
};
