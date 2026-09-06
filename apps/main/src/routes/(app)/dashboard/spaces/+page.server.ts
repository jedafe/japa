// Ref: japaconnect-engineering/sections/05-spaces.md (user's joined spaces)
// Lists the spaces the current user has joined, moderates, owns, or has muted. Auth-gated.
import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, ApiError } from '$lib/server/api';
import type { MeSpace, MutedSpace } from '$lib/types';

const VALID_TABS = ['joined', 'moderated', 'owned', 'muted'] as const;
type Tab = (typeof VALID_TABS)[number];

const DEFAULT_PAGE_SIZE = 25;
const MAX_PAGE_SIZE = 100;

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const rawTab = event.url.searchParams.get('tab') ?? 'joined';
	if (!VALID_TABS.includes(rawTab as Tab)) {
		const url = new URL(event.url);
		url.searchParams.set('tab', 'joined');
		throw redirect(307, url.toString());
	}
	const tab = rawTab as Tab;

	const q = event.url.searchParams.get('q')?.trim() ?? '';
	const requestedPage = parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1;
	const requestedPageSize = parseInt(event.url.searchParams.get('page_size') ?? String(DEFAULT_PAGE_SIZE), 10) || DEFAULT_PAGE_SIZE;
	const page = Math.max(1, requestedPage);
	const pageSize = Math.min(MAX_PAGE_SIZE, Math.max(1, requestedPageSize));
	const pageOffset = (page - 1) * pageSize;

	const ctx = apiContext(event);
	const endpoint =
		tab === 'moderated'
			? '/me/spaces/moderated'
			: tab === 'owned'
				? '/me/spaces/owned'
				: tab === 'muted'
					? '/me/muted-spaces'
					: '/me/spaces';

	const params = new URLSearchParams();
	if (q) params.set('q', q);
	params.set('page_size', String(pageSize));
	params.set('page_offset', String(pageOffset));
	// member_only forces the membership listing even for super admins, whose
	// /me/spaces response otherwise returns every space in the tenant.
	if (tab === 'joined') params.set('member_only', '1');

	try {
		if (tab === 'muted') {
			// Custom invalidation key so client-side unmute can refresh this load
			// without depending on the exact backend fetch URL.
			event.depends('/api/v1/me/muted-spaces');
			const { items } = await apiList<MutedSpace>(ctx, `${endpoint}?${params.toString()}`);
			const spaces: MeSpace[] = items.map((m) => ({
				id: m.space_id,
				slug: m.space_slug,
				name: m.space_name,
				is_primary: m.is_primary ?? false
			}));
			return { spaces, tab, q, page, pageSize };
		}

		const { items: spaces } = await apiList<MeSpace>(ctx, `${endpoint}?${params.toString()}`);
		return { spaces, tab, q, page, pageSize };
	} catch (err) {
		if (err instanceof ApiError) {
			throw error(err.status, { code: err.code, message: err.message });
		}
		throw error(500, { code: 'api_error', message: 'Failed to load spaces' });
	}
};
