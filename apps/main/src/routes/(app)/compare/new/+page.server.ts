import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import type { MeSpace, ComparisonCreated, ComparisonType } from '$lib/types';

/**
 * Create-comparison form at /compare/new — auth-gated. Loads the user's spaces
 * and the available JCE comparison types for the type selector. Both
 * degrade to [] on error. POST /jce/comparisons creates a quick comparison;
 * on success we redirect to /compare/{slug}.
 *
 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);

	let spaces: MeSpace[] = [];
	try {
		spaces = (await apiList<MeSpace>(ctx, '/me/spaces')).items;
	} catch {
		spaces = [];
	}

	let comparisonTypes: ComparisonType[] = [];
	try {
		comparisonTypes = (await apiList<ComparisonType>(ctx, '/jce/comparison-types?page_size=50'))
			.items;
	} catch {
		comparisonTypes = [];
	}

	// Pre-fill from query params (Section 39 — university "Compare" button
	// navigates here with ?type=university&item1_name=...&item1_slug=...).
	const sp = event.url.searchParams;
	const initialType = sp.get('type') ?? '';
	const initialItem =
		sp.get('item1_name') && sp.get('item1_slug')
			? { name: sp.get('item1_name')!, slug: sp.get('item1_slug')! }
			: null;

	return { spaces, comparisonTypes, initialType, initialItem };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) return fail(403, {
			message: 'Auth required',
			values: { title: '', body: '', visibility: 'public', space_id: '', type_id: '' }
		});

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const title = String(form.get('title') ?? '').trim();
		const body = String(form.get('body') ?? '').trim();
		const visibility = String(form.get('visibility') ?? 'public');
		const spaceId = String(form.get('space_id') ?? '').trim();
		const typeId = String(form.get('type_id') ?? '').trim();

		// Multi-step form data (Phase 16.04): items + dimensions.
		const itemNames = form.getAll('item_name[]').map((v) => String(v).trim()).filter(Boolean);
		const dimensions = form.getAll('dimension[]').map((v) => String(v).trim()).filter(Boolean);

		if (!title) {
			return fail(400, { message: 'Title is required', values: { title, body, visibility, space_id: spaceId, type_id: typeId } });
		}

		const payload: Record<string, unknown> = {
			title,
			body,
			depth: 'quick',
			status: 'draft',
			visibility: 'private'
		};
		if (spaceId) payload.space_id = spaceId;
		if (typeId) payload.type_id = typeId;
		if (itemNames.length >= 2) {
			payload.items = itemNames.map((name) => ({ title: name }));
		}
		if (dimensions.length > 0) {
			payload.dimensions = dimensions;
		}

		try {
			const created = await apiPost<ComparisonCreated>(ctx, '/jce/comparisons', payload);
			// Redirect to the /compare/{slug} preview (quick comparisons are private
			// by default; the creator can later fill values and promote to detailed).
			throw redirect(303, `/compare/${created.slug}`);
		} catch (e) {
			if (e instanceof ApiError) {
				return fail(e.status >= 400 && e.status < 500 ? e.status : 500, {
					message: e.message,
					values: { title, body, visibility, space_id: spaceId, type_id: typeId }
				});
			}
			throw e;
		}
	}
};
