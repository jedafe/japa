import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ForumThread } from '$lib/types';

/**
 * Forum list at /forum — Reddit-style compact thread list inside the app shell.
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *
 * The backend lists threads by tenant, keyset-paginated by created_at desc
 * (page_size + cursor_created_at). There is no `sort` query param on this
 * endpoint — the sort bar is omitted on the forum index for that reason.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	try {
		const { items } = await apiList<ForumThread>(ctx, `/forum?page_size=${PAGE_SIZE}`);
		return { threads: items, authed, pageSize: PAGE_SIZE, loadError: false };
	} catch {
		return { threads: [] as ForumThread[], authed, pageSize: PAGE_SIZE, loadError: true };
	}
};
