import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	SpaceDetail,
	SpaceMember,
	SpaceContentItem,
	ForumThread
} from '$lib/types';

/**
 * Space hub at /spaces/[slug] — SSR, indexable. Center column swaps between
 * Feed / Forum / Members / About tabs; the right sidebar shows the space
 * card (member/follower counts, join + follow actions, rules).
 *
 * Ref: japaconnect-engineering/sections/22-spaces.md
 *      ADR-058 (Space: Feed/Forum/Members/About tabs inside the shell)
 */
const PAGE_SIZE = 20;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Space detail bundle: { space, rules }. 404 propagates as a SvelteKit error.
	let detail: SpaceDetail;
	try {
		detail = await apiGet<SpaceDetail>(ctx, `/spaces/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Space not found' : 'Failed to load space'
		});
	}

	const space = detail.space;
	const spaceId = space.id;

	// Tab data — each degrades to an empty list on failure so a missing tab
	// never blanks the whole space view.
	const [content, members, forumThreads, analytics] = await Promise.all([
		apiList<SpaceContentItem>(ctx, `/spaces/${slug}/content?page_size=${PAGE_SIZE}`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<SpaceMember>(ctx, `/spaces/${slug}/members?page_size=${PAGE_SIZE}`)
			.then((r) => r.items)
			.catch(() => []),
		apiList<ForumThread>(ctx, `/forum?page_size=${PAGE_SIZE}`)
			.then((r) => r.items.filter((t) => t.space_slug === slug))
			.catch(() => []),
		// Phase 08.10 — space analytics (best-effort).
		apiGet<unknown>(ctx, `/spaces/${spaceId}/analytics`).catch(() => null)
	]);

	return {
		space,
		rules: detail.rules,
		content,
		members,
		forumThreads,
		analytics,
		pageSize: PAGE_SIZE
	};
};
