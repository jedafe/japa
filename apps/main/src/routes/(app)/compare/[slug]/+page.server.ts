import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { JCEComparison, ComparisonDetail, ComparisonMatrixItem } from '$lib/types';

/**
 * Comparison detail at /compare/[slug] — auth-gated. The `slug` param is the
 * comparison slug (human-readable); the JCE backend GET /jce/comparisons/{id|slug}
 * accepts either a UUID or a slug. Also fetches the matrix
 * (GET /jce/comparisons/{id|slug}/matrix) for type-aware cell rendering.
 *
 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let jce: JCEComparison;
	try {
		jce = await apiGet<JCEComparison>(ctx, `/jce/comparisons/${slug}`);
	} catch (e) {
		const status = e instanceof ApiError ? e.status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Comparison not found' : 'Failed to load comparison'
		});
	}

	const comparison: ComparisonDetail = {
		id: jce.content.id,
		tenant_id: jce.content.tenant_id,
		author_id: jce.content.author_id,
		title: jce.content.title,
		slug: jce.content.slug,
		body: jce.content.body,
		visibility: jce.content.visibility,
		status: jce.content.status,
		space_id: jce.content.space_id,
		items: jce.items,
		dimensions: jce.dimensions
	};

	// Best-effort matrix fetch — degrades to empty if the comparison has no
	// values yet (e.g. a freshly created comparison with items+dimensions but
	// no cell data populated).
	let matrix: ComparisonMatrixItem[] = [];
	try {
		matrix = (await apiGet<ComparisonMatrixItem[]>(ctx, `/jce/comparisons/${slug}/matrix`)) ?? [];
	} catch {
		matrix = [];
	}

	return { comparison, matrix };
};
