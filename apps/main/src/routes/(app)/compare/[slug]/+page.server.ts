import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError, API_BASE } from '$lib/server/api';
import type {
	JCEComparisonDetail,
	MatrixItem,
	ResolvedItem
} from '$lib/types/jce';
import type { PageServerLoad } from './$types';

function resolvedItem(cmp: JCEComparisonDetail): ResolvedItem[] {
	return (
		cmp.items?.map((it) => ({
			type: (it.item_ref?.type as string) ?? '',
			id: (it.item_ref?.id as string) ?? (it.id as string) ?? '',
			title: it.title,
			description: it.description,
			url: (it.item_ref?.url as string) ?? undefined,
			position: it.position,
			metadata: (it.item_ref as Record<string, unknown>) ?? undefined
		})) ?? []
	);
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let comparison: JCEComparisonDetail | null = null;
	let matrix: MatrixItem[] = [];

	// Batch endpoint — detail + matrix in one request (§30). The matrix
	// degrades to [] on a partial-failure envelope the same way the old
	// separate /matrix call did.
	try {
		const page = await apiGet<{ comparison: JCEComparisonDetail; matrix: MatrixItem[] }>(
			ctx,
			`/jce/comparisons/${slug}/page`
		);
		comparison = page.comparison;
		matrix = page.matrix ?? [];
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			if (err.code === 'feature_disabled') {
				throw error(503, { code: 'feature_disabled', message: 'jce.error.feature_disabled' });
			}
			error(404, { code: 'not_found', message: 'jce.error.not_found' });
		}
		throw err;
	}

	if (!comparison) {
		error(404, { code: 'not_found', message: 'jce.error.not_found' });
	}

	return {
		comparison,
		matrix,
		resolvedItems: resolvedItem(comparison),
		user: event.locals.user,
		isOwner: !!event.locals.user && event.locals.user.id === comparison.content.author_id,
		apiBase: API_BASE,
		mainOrigin: event.url.origin,
		typeSlug: comparison.type_slug ?? ''
	};
};
