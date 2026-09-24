// Ref: japaconnect-engineering/sections/79-community-story-wall.md
//      japaconnect-engineering/sections/14-content-type-system.md
// Stories are authored and edited through the unified composer at /create
// (§79, ADR-058). This route is kept as a redirect so existing
// bookmarks/links to /story/{slug}/edit continue to work.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { Story } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let story: Story | null = null;
	try {
		story = await apiGet<Story>(ctx, `/stories/${slug}?raw=1`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Story not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load story' });
	}

	const isAuthor = !!story.author_id && story.author_id === event.locals.user.id;
	const isSuperAdmin = (event.locals.user.groups ?? []).some((g) => g === 'super_admin' || g === 'Super Admin');

	if (!isAuthor && !isSuperAdmin) {
		let canEditAny = false;
		try {
			const permRes = await apiGet<{ permissions: string[] }>(ctx, '/me/permissions');
			const perms = permRes?.permissions ?? [];
			canEditAny = perms.includes('story.edit_any') || perms.includes('content.edit_any');
		} catch {
			canEditAny = false;
		}
		if (!canEditAny) {
			throw error(403, { code: 'not_owner', message: 'You can only edit your own stories' });
		}
	}

	throw redirect(307, `/create?draft=${story.id}&type=story`);
};
