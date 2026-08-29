import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { ForumThread } from '$lib/types';

/**
 * Forum list at /forum — Reddit-style compact thread list inside the app shell.
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *
 * The backend lists threads by tenant, keyset-paginated by created_at desc
 * (page_size + cursor_created_at). Sort=top orders by vote_score descending.
 * Supports ?tag=slug, ?sort=latest|top and ?status=all|solved|open.
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tagSlug = event.url.searchParams.get('tag') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';
	const status = event.url.searchParams.get('status') ?? 'all';
	const authorId = event.url.searchParams.get('author_id') ?? '';

	const params = new URLSearchParams();
	params.set('page_size', String(PAGE_SIZE));
	if (sort === 'top') params.set('sort', 'top');
	if (status === 'solved') params.set('solved', 'true');
	if (status === 'open') params.set('solved', 'false');
	if (tagSlug) params.set('tag', tagSlug);
	if (authorId) params.set('author_id', authorId);

	try {
		const { items } = await apiList<ForumThread>(ctx, `/forum?${params.toString()}`);
		return { threads: items, authed, pageSize: PAGE_SIZE, loadError: false, tagSlug, sort, status, authorId };
	} catch {
		return { threads: [] as ForumThread[], authed, pageSize: PAGE_SIZE, loadError: true, tagSlug, sort, status, authorId };
	}
};
