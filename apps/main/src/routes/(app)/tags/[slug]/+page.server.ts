import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { TagDetail } from '$lib/types';
import type { ContentArticle } from '$lib/types/content';

/**
 * Tag hub at /tags/[slug] — SSR, indexable. Center column shows the tag
 * header (name, description, follower count, follow button) and a grid of
 * tagged content. The right sidebar shows the tag card with follow action.
 *
 * Ref: japaconnect-engineering/sections/08-tags.md
 *      ADR-058 (renders inside the app shell; not a documented tab surface)
 */
const PAGE_SIZE = 12;

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	// Tag detail — 404 propagates as a SvelteKit error.
	let tag: TagDetail;
	try {
		tag = await apiGet<TagDetail>(ctx, `/tags/${slug}`);
	} catch (e) {
		const status = e instanceof Error && 'status' in e ? (e as { status: number }).status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Tag not found' : 'Failed to load tag'
		});
	}

	// Tagged content — degrades to an empty list on failure so a missing feed
	// never blanks the whole tag view.
	let items: ContentArticle[] = [];
	let loadError = false;
	try {
		items = (
			await apiList<ContentArticle>(ctx, `/tags/${slug}/content?page_size=${PAGE_SIZE}`)
		).items;
	} catch {
		loadError = true;
	}

	// Hydrate the follow button's initial state. Best-effort: a failure
	// degrades to `false` (not following). Only fetch when the viewer is
	// authenticated — guests can't follow.
	const initiallyFollowing = event.locals.user
		? apiGet<{ following: boolean }>(ctx, `/users/${tag.id}/follow-status?entity_type=tag`)
				.then((r) => r.following)
				.catch(() => false)
		: Promise.resolve(false);

	const isFollowing = await initiallyFollowing;

	return {
		tag,
		items,
		pageSize: PAGE_SIZE,
		loadError,
		initiallyFollowing: isFollowing
	};
};
