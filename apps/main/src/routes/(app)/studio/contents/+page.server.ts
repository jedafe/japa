// Ref: .jeos/specs/2026-09-18-studio-web-prompts-tab.md §11 — Contents tab.
// Member content manager (moved from /dashboard/content): status/type/q/sort
// filters resolve to GET /me/content, /me/drafts, or /me/scheduled via
// buildContentQuery.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { buildContentQuery } from '$lib/studio/contentQuery';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const params = event.url.searchParams;

	// Fetch page_size+1 so the client can tell whether a "Load more"
	// page exists without a separate count query (the API convention
	// returns { data: [...] } with no total). Ref: §50, api-response-format.
	const pageSize = 20;
	const path = buildContentQuery({
		status: params.get('status') ?? undefined,
		type: params.get('type') ?? undefined,
		q: params.get('q') ?? undefined,
		sort: params.get('sort') ?? undefined,
		pageSize: pageSize + 1
	});
	let batch: unknown[] = [];
	try {
		const contentRes = await apiList<unknown>(ctx, path);
		batch = contentRes.items;
	} catch {
		// Degrade to an empty list rather than 500ing the tab when the
		// content list endpoint is unhealthy.
	}
	const hasMore = batch.length > pageSize;
	const items = hasMore ? batch.slice(0, pageSize) : batch;
	return {
		items,
		// Rule 16 — content type options come from the API, not hardcoded.
		contentTypes: (await apiList<unknown>(ctx, '/content-types').catch(() => ({ items: [] }))).items,
		pageSize,
		hasMore
	};
};
