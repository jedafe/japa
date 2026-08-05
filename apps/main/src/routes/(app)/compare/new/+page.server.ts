import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import type { MeSpace, ComparisonCreated, ComparisonType } from '$lib/types';

/**
 * Create-comparison form at /compare/new — auth-gated. Loads the user's spaces
 * and the available comparison types (Section 30) for the type selector
 * (Phase 16.04: select comparison type → add items → dimensions). Both
 * degrade to [] on error. POST /comparisons creates the comparison; on
 * success we redirect to /compare/{slug}.
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
		comparisonTypes = (await apiList<ComparisonType>(ctx, '/comparisons/types?page_size=50'))
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
		const itemSlugs = form.getAll('item_slug[]').map((v) => String(v).trim()).filter(Boolean);
		const dimensions = form.getAll('dimension[]').map((v) => String(v).trim()).filter(Boolean);

		if (!title) {
			return fail(400, { message: 'Title is required', values: { title, body, visibility, space_id: spaceId, type_id: typeId } });
		}

		const payload: Record<string, unknown> = { title, body, visibility, status: 'draft' };
		if (spaceId) payload.space_id = spaceId;
		if (typeId) payload.type_id = typeId;
		if (itemNames.length >= 2) {
			payload.items = itemNames.map((name, i) => ({ name, slug: itemSlugs[i] ?? '' }));
		}
		if (dimensions.length > 0) {
			payload.dimensions = dimensions;
		}

		try {
			const created = await apiPost<ComparisonCreated>(ctx, '/comparisons', payload);
			// Redirect to the /create editor with the draft ID so the user can
			// fill cell values and publish via the standard composer flow
			// (Section 30 — "Publish (standard content publish flow)"). The
			// comparison is created as a draft, not auto-published.
			throw redirect(303, `/create?draft=${created.id}`);
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
