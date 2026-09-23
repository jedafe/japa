import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import { suggestSlug } from '$lib/components/create/slug';
import type { MeSpace, ForumThread } from '$lib/types';

/**
 * Full-page new forum thread composer at /forum/[category]/new — the
 * documented exception to the modal composer for full-page new threads.
 * Auth-gated; the [category] param pre-selects the space when it matches a
 * joined space slug.
 *
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *      ADR-058 (Compose flow — /forum/[category]/new remains for full-page threads)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=/forum/${event.params.category}/new`);
	}

	const ctx = apiContext(event);
	const category = event.params.category;

	let spaces: MeSpace[] = [];
	try {
		spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	} catch {
		spaces = [];
	}

	if (spaces.length === 0) {
		throw error(400, {
			code: 'no_spaces',
			message: 'Join a space before starting a discussion.'
		});
	}

	// Pre-select the space whose slug matches the [category] param.
	const preselected = spaces.find((s) => s.slug === category) ?? null;

	return { spaces, preselected, category };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			throw redirect(307, '/login');
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();
		const title = String(form.get('title') ?? '').trim();
		const spaceId = String(form.get('space_id') ?? '').trim();
		const body = String(form.get('body') ?? '');
		const isAnonymous = form.get('is_anonymous') === 'on';
		// Comma-separated tag slugs from the TagInput field (Section 08).
		const tagSlugs = String(form.get('tags') ?? '')
			.split(',')
			.map((s) => s.trim().toLowerCase().replace(/\s+/g, '-'))
			.filter(Boolean)
			.slice(0, 5);

		// Poll builder fields (Phase 07.13) — optional.
		const pollQuestion = String(form.get('poll_question') ?? '').trim();
		const pollOptionsRaw = form.getAll('poll_option[]').map((v) => String(v).trim()).filter(Boolean);
		const pollAllowMultiple = form.get('poll_allow_multiple') === 'on';
		const pollExpiresAt = String(form.get('poll_expires_at') ?? '').trim() || null;

		if (!title) {
			return { error: 'missing_title', title, body, spaceId, isAnonymous, tags: tagSlugs };
		}
		if (!spaceId) {
			return { error: 'missing_space', title, body, spaceId, isAnonymous, tags: tagSlugs };
		}

		const slug = suggestSlug(title);
		const poll =
			pollQuestion && pollOptionsRaw.length >= 2
				? {
						question: pollQuestion,
						is_multi_select: pollAllowMultiple,
						close_at: pollExpiresAt,
						options: pollOptionsRaw.map((label, i) => ({ label, position: i }))
					}
				: undefined;

		try {
			const thread = await apiPost<ForumThread>(ctx, '/forum', {
				title,
				slug,
				body,
				space_id: spaceId,
				is_anonymous: isAnonymous,
				poll
			});

			// Attach tags (non-fatal — the thread is already created). Tags are
			// universal (Sections 08/14): POST /content/{id}/tags accepts
			// tag_slugs and get-or-creates missing slugs server-side (BUG-006).
			if (tagSlugs.length > 0 && thread.content_id) {
				try {
					await apiPost(ctx, `/content/${thread.content_id}/tags`, { tag_slugs: tagSlugs });
				} catch {
					// non-fatal — thread exists without tags
				}
			}

			throw redirect(303, `/forum/${thread.slug}`);
		} catch (e) {
			if (e instanceof ApiError) {
				return { error: e.code, title, body, spaceId, isAnonymous, tags: tagSlugs };
			}
			throw e;
		}
	}
};
