import { error, fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import type { MeSpace, ComparisonCreated } from '$lib/types';

/**
 * Create-comparison form at /compare/new — auth-gated. Loads the user's spaces
 * for the space dropdown (degrades to [] on error). POST /comparisons creates
 * the comparison; on success we redirect to /compare/{id}.
 *
 * Ref: japaconnect-engineering/sections/16-comparisons.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required' });
	}

	const ctx = apiContext(event);

	let spaces: MeSpace[] = [];
	try {
		spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	} catch {
		spaces = [];
	}

	return { spaces };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, { message: 'Auth required' });

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const title = String(form.get('title') ?? '').trim();
		const body = String(form.get('body') ?? '').trim();
		const visibility = String(form.get('visibility') ?? 'public');
		const spaceId = String(form.get('space_id') ?? '').trim();

		// Multi-step form data (Phase 16.04): items + dimensions.
		const itemNames = form.getAll('item_name[]').map((v) => String(v).trim()).filter(Boolean);
		const itemSlugs = form.getAll('item_slug[]').map((v) => String(v).trim()).filter(Boolean);
		const dimensions = form.getAll('dimension[]').map((v) => String(v).trim()).filter(Boolean);

		if (!title) {
			return fail(400, { message: 'Title is required', values: { title, body, visibility, space_id: spaceId } });
		}

		const payload: Record<string, unknown> = { title, body, visibility };
		if (spaceId) payload.space_id = spaceId;
		if (itemNames.length >= 2) {
			payload.items = itemNames.map((name, i) => ({ name, slug: itemSlugs[i] ?? '' }));
		}
		if (dimensions.length > 0) {
			payload.dimensions = dimensions;
		}

		try {
			const created = await apiPost<ComparisonCreated>(ctx, '/comparisons', payload);
			throw redirect(303, `/compare/${created.id}`);
		} catch (e) {
			if (e instanceof ApiError) {
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: e.message,
					values: { title, body, visibility, space_id: spaceId }
				});
			}
			throw e;
		}
	}
};
