import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { DMRequest } from '$lib/types';

/**
 * Message requests at /messages/requests — auth-gated DM requests surface
 * (feature.dm). Lists pending requests with accept/decline/block actions.
 *
 * URL params forwarded to GET /dm/requests: q, sort=recent|oldest|name,
 * page_size.
 *
 * Ref: japaconnect-engineering/sections/48-direct-messages.md
 */
const REQUEST_SORTS = new Set(['recent', 'oldest', 'name']);
const PAGE_SIZES = new Set([10, 20, 30, 50]);
const DEFAULT_PAGE_SIZE = 30;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.dm']) {
		throw error(404, { code: 'feature_disabled', message: 'Direct Messages are not available' });
	}

	if (!event.locals.user) {
		throw redirect(307, '/login?redirect=/messages/requests');
	}

	const ctx = apiContext(event);
	const params = event.url.searchParams;

	const q = params.get('q')?.trim() ?? '';
	const sortParam = params.get('sort') ?? 'recent';
	const sort = REQUEST_SORTS.has(sortParam) ? sortParam : 'recent';
	const sizeParam = Number(params.get('page_size'));
	const pageSize = PAGE_SIZES.has(sizeParam) ? sizeParam : DEFAULT_PAGE_SIZE;

	const reqQuery = new URLSearchParams();
	reqQuery.set('page_size', String(pageSize));
	if (q) reqQuery.set('q', q);
	if (sort !== 'recent') reqQuery.set('sort', sort);

	let requests: DMRequest[] = [];
	let requestsCursor: string | null = null;
	let loadError = false;
	try {
		const res = await apiList<DMRequest>(ctx, `/dm/requests?${reqQuery}`);
		requests = res.items;
		requestsCursor = res.meta?.next_cursor ?? null;
	} catch {
		loadError = true;
	}

	return {
		requests,
		requestsCursor,
		loadError,
		query: { q, sort, pageSize }
	};
};
