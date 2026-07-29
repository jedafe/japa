import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ComparisonDetail, ComparisonMatrixItem } from '$lib/types';

/**
 * Comparison detail at /compare/[slug] — auth-gated. The `slug` param is the
 * comparison ID (UUID); it is passed directly to GET /comparisons/{id}.
 * Also fetches the matrix (GET /comparisons/{id}/matrix) for type-aware cell
 * rendering — stars for rating, ✓/✗ for boolean, plain text otherwise.
 *
 * Ref: japaconnect-engineering/sections/16-comparisons.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw error(403, { code: 'forbidden', message: 'Authentication required' });
	}

	const ctx = apiContext(event);
	const id = event.params.slug;

	let comparison: ComparisonDetail;
	try {
		comparison = await apiGet<ComparisonDetail>(ctx, `/comparisons/${id}`);
	} catch (e) {
		const status = e instanceof ApiError ? e.status : 500;
		throw error(status === 404 ? 404 : 500, {
			code: status === 404 ? 'not_found' : 'server_error',
			message: status === 404 ? 'Comparison not found' : 'Failed to load comparison'
		});
	}

	// Best-effort matrix fetch — degrades to empty if the comparison has no
	// values yet (e.g. a freshly created comparison with items+dimensions but
	// no cell data populated).
	let matrix: ComparisonMatrixItem[] = [];
	try {
		matrix = (await apiGet<ComparisonMatrixItem[]>(ctx, `/comparisons/${id}/matrix`)) ?? [];
	} catch {
		matrix = [];
	}

	return { comparison, matrix };
};
