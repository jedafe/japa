import type { PageServerLoad } from './$types';
import { apiContext, apiGetEnvelope, apiList } from '#lib/server/api.js';
import type { FeedItem, ForumThread } from '#lib/types/index.js';
import type { GuideListItem } from '#lib/types/content.js';

/**
 * Thread index at /thread — lists ALL threads: every contents row is a
 * thread (unified-discussion spec), so the default list is the explore
 * feed of all published content, and /thread/{slug} is its discussion
 * page. The "New discussion" composer still creates forum questions.
 *
 * The forum-only filters (?status=solved|open, ?tag=slug) narrow to
 * forum_thread rows via /thread and render the ForumList; sort=featured
 * pulls featured content of every type (§14 content_featured junction).
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const authed = !!event.locals.user;
	const tagSlug = event.url.searchParams.get('tag') ?? '';
	const sort = event.url.searchParams.get('sort') ?? 'latest';
	const status = event.url.searchParams.get('status') ?? 'all';
	const authorId = event.url.searchParams.get('author_id') ?? '';

	const base = {
		items: [] as FeedItem[],
		threads: [] as ForumThread[],
		featured: [] as GuideListItem[],
		authed,
		pageSize: PAGE_SIZE,
		loadError: false,
		tagSlug,
		sort,
		status,
		authorId
	};

	if (sort === 'featured') {
		try {
			const res = await apiGetEnvelope<GuideListItem[]>(
				ctx,
				`/content?featured=1&page_size=${PAGE_SIZE}`
			);
			return { ...base, featured: res.data ?? [] };
		} catch {
			return { ...base, loadError: true };
		}
	}

	// Forum-specific filters keep the forum thread list — solved/open and
	// tag filtering only apply to forum_thread rows.
	if (status !== 'all' || tagSlug || authorId) {
		const params = new URLSearchParams();
		params.set('page_size', String(PAGE_SIZE));
		if (sort === 'top') params.set('sort', 'top');
		if (status === 'solved') params.set('solved', 'true');
		if (status === 'open') params.set('solved', 'false');
		if (tagSlug) params.set('tag', tagSlug);
		if (authorId) params.set('author_id', authorId);

		try {
			const { items } = await apiList<ForumThread>(ctx, `/thread?${params.toString()}`);
			return { ...base, threads: items };
		} catch {
			return { ...base, loadError: true };
		}
	}

	try {
		const { items } = await apiList<FeedItem>(
			ctx,
			`/explore?sort=${sort === 'top' ? 'top' : 'latest'}&page_size=${PAGE_SIZE}&page_offset=0`
		);
		return { ...base, items };
	} catch {
		return { ...base, loadError: true };
	}
};
