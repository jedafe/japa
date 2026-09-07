// Ref: japaconnect-engineering/sections/79-community-story-wall.md
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList, ApiError } from '$lib/server/api';
import type { Story, GeoCountry } from '$lib/types';

/**
 * Edit-story form at /stories/[slug]/edit — auth-gated, author or moderator.
 * Loads the story by slug and the country list for the select dropdowns.
 * The author may always edit; moderators need story.edit_any or
 * content.edit_any (matching the PATCH /stories/:id backend gate, which uses
 * RequireAnyPermission(story.edit_own, story.edit_any)).
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let story: Story | null = null;
	try {
		story = await apiGet<Story>(ctx, `/stories/${slug}`);
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'not_found', message: 'Story not found' });
		}
		throw error(500, { code: 'load_failed', message: 'Failed to load story' });
	}

	// Only the author or a story moderator may edit. The backend enforces this
	// too (story.edit_own + author match, or story.edit_any bypass), but we
	// check here to avoid showing the form to non-editors.
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

	let countries: GeoCountry[] = [];
	try {
		const { items } = await apiList<GeoCountry>(ctx, '/geo/countries');
		countries = items;
	} catch {
		countries = [];
	}

	return { story, countries, authed: true };
};
