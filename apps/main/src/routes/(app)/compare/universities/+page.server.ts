import { fail, redirect } from '@sveltejs/kit';
import type { PageServerLoad, Actions } from './$types';
import { apiContext, apiList, apiPost, ApiError } from '$lib/server/api';
import type { University, ComparisonCreated, ComparisonType } from '$lib/types';

/**
 * University comparison picker at /compare/universities — auth-gated. Lists
 * universities (Section 39) and the seeded `universities` comparison type
 * (Section 30) so the user can pick 2–4 universities to compare side by side
 * without typing names manually. The current university is pre-selected as
 * Item 1 via ?item1_slug=... (Section 39 — "Compare button opens the
 * Comparison System with this university pre-selected as Item 1").
 *
 * POST creates the comparison (type_id + items[{name,slug}] + auto-generated
 * dimensions) and redirects to /compare/{id}, mirroring /compare/new but
 * constrained to the university type and a picker UI.
 *
 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md,
 *      japaconnect-engineering/sections/39-space-types-university.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);

	let universities: University[] = [];
	try {
		universities = (await apiList<University>(ctx, '/universities')).items;
	} catch {
		universities = [];
	}

	// Resolve the seeded `universities` comparison type so the created
	// comparison is linked to the right type definition (Section 30 pre-seeded
	// types: universities, visa_types, banks, courses). Degrades to '' when
	// the type catalog is unavailable — the picker still works without a type.
	let universityTypeId = '';
	try {
		const types = (await apiList<ComparisonType>(ctx, '/comparisons/types?page_size=50')).items;
		const match = types.find(
			(ct) => ct.slug === 'universities' || ct.slug === 'university'
		);
		if (match) universityTypeId = match.id;
	} catch {
		universityTypeId = '';
	}

	// Pre-fill Item 1 from the query param (university "Compare" button).
	const sp = event.url.searchParams;
	const item1Slug = sp.get('item1_slug') ?? '';
	const item1 =
		item1Slug && universities.some((u) => u.slug === item1Slug)
			? universities.find((u) => u.slug === item1Slug)!
			: null;

	return { universities, universityTypeId, item1 };
};

export const actions: Actions = {
	default: async (event) => {
		if (!event.locals.user) {
			return fail(403, {
				message: 'Auth required',
				values: { title: '', visibility: 'public', type_id: '' }
			});
		}

		const ctx = apiContext(event);
		const form = await event.request.formData();

		const title = String(form.get('title') ?? '').trim();
		const visibility = String(form.get('visibility') ?? 'public');
		const typeId = String(form.get('type_id') ?? '').trim();
		const itemNames = form.getAll('item_name[]').map((v) => String(v).trim()).filter(Boolean);
		const itemSlugs = form.getAll('item_slug[]').map((v) => String(v).trim()).filter(Boolean);
		const dimensions = form.getAll('dimension[]').map((v) => String(v).trim()).filter(Boolean);

		if (!title) {
			return fail(400, {
				message: 'Title is required',
				values: { title, visibility, type_id: typeId }
			});
		}
		if (itemNames.length < 2) {
			return fail(400, {
				message: 'At least 2 universities are required',
				values: { title, visibility, type_id: typeId }
			});
		}

		const payload: Record<string, unknown> = { title, visibility };
		if (typeId) payload.type_id = typeId;
		payload.items = itemNames.map((name, i) => ({ name, slug: itemSlugs[i] ?? '' }));
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
					values: { title, visibility, type_id: typeId }
				});
			}
			throw e;
		}
	}
};
