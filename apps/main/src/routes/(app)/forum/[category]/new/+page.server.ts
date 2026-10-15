import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiGet, apiList, apiPost, ApiError } from '$lib/server/api';
import { suggestSlug } from '$lib/components/create/slug';
import type { MeSpace, ForumThread, SpaceDetail } from '$lib/types';

/**
 * Full-page new forum thread composer at /forum/[category]/new — the
 * documented exception to the modal composer for full-page new threads.
 * Auth-gated; the [category] param pre-selects the space when it matches a
 * joined space slug. ?type=question renders question chrome and tags the
 * thread "question" on submit — the wiki "Ask a Question" CTA lands here.
 *
 * Ref: japaconnect-engineering/sections/08-forum-qa-solved.md
 *      ADR-058 (Compose flow — /forum/[category]/new remains for full-page threads)
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(
			307,
			`/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`
		);
	}

	const ctx = apiContext(event);
	const category = event.params.category;
	const isQuestion = event.url.searchParams.get('type') === 'question';

	let spaces: MeSpace[] = [];
	try {
		spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	} catch {
		spaces = [];
	}

	// Pre-select the space whose slug matches the [category] param. When the
	// caller hasn't joined it (e.g. a wiki destination's community space),
	// resolve it publicly and offer it — POST /forum doesn't gate on
	// membership.
	let preselected = spaces.find((s) => s.slug === category) ?? null;
	if (!preselected) {
		const detail = await apiGet<SpaceDetail>(
			ctx,
			`/spaces/${encodeURIComponent(category)}`
		).catch(() => null);
		if (detail?.space?.id) {
			preselected = {
				id: detail.space.id,
				slug: detail.space.slug,
				name: detail.space.name
			};
			spaces = [...spaces, preselected];
		}
	}

	if (spaces.length === 0) {
		throw error(400, {
			code: 'no_spaces',
			message: 'Join a space before starting a discussion.'
		});
	}

	return { spaces, preselected, category, isQuestion };
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
		// Question-mode threads carry the "question" tag so they're
		// filterable and eligible for the solved flow — same as the modal
		// composer's Question toggle.
		const isQuestion = form.get('type') === 'question';
		const tagSlugs = String(form.get('tags') ?? '')
			.split(',')
			.map((s) => s.trim().toLowerCase().replace(/\s+/g, '-'))
			.filter(Boolean)
			.slice(0, 5);
		if (isQuestion && !tagSlugs.includes('question')) {
			tagSlugs.unshift('question');
		}

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
