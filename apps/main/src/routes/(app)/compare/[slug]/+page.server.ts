import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { ComparisonDetail, ComparisonMatrixItem } from '$lib/types';

/**
 * Comparison detail at /compare/[slug] — auth-gated. The `slug` param is the
 * comparison slug (human-readable); the backend GET /comparisons/{id} accepts
 * either a UUID or a slug (Section 30 — GET /api/v1/comparisons/:slug). Also
 * fetches the matrix (GET /comparisons/{id}/matrix) for type-aware cell
 * rendering — stars for rating, ✓/✗ for boolean, plain text otherwise.
 *
 * Ref: japaconnect-engineering/sections/30-generic-comparison-system.md
 */
export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);
	const slug = event.params.slug;

	let comparison: ComparisonDetail;
	try {
		comparison = await apiGet<ComparisonDetail>(ctx, `/comparisons/${slug}`);
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
		matrix = (await apiGet<ComparisonMatrixItem[]>(ctx, `/comparisons/${slug}/matrix`)) ?? [];
	} catch {
		matrix = [];
	}

	return { comparison, matrix };
};
