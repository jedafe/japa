import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError, API_V1 } from '$lib/server/api';
import type { JCEComparisonDetail, MatrixItem, ResolvedItem } from '$lib/types/jce';
import type { PageServerLoad } from './$types';

function resolvedItem(cmp: JCEComparisonDetail): ResolvedItem[] {
	return (
		cmp.items?.map((it) => ({
			type: (it.item_ref?.type as string) ?? '',
			id: (it.item_ref?.id as string) ?? (it.id as string) ?? '',
			title: it.title,
			description: it.description,
			metadata: (it.item_ref as Record<string, unknown>) ?? undefined
		})) ?? []
	);
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const slug = event.params.slug;

	let comparison: JCEComparisonDetail | null = null;
	let matrix: MatrixItem[] = [];

	try {
		comparison = await apiGet<JCEComparisonDetail>(ctx, `/jce/comparisons/${slug}`);
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

	try {
		matrix = await apiGet<MatrixItem[]>(ctx, `/jce/comparisons/${slug}/matrix`);
	} catch {
		matrix = [];
	}

	return {
		comparison,
		matrix,
		resolvedItems: resolvedItem(comparison),
		user: event.locals.user,
		isOwner: !!event.locals.user && event.locals.user.id === comparison.content.author_id,
		apiBase: API_V1,
		mainOrigin: (event.locals.siteSettings?.['jce.main_origin'] as string) ?? 'http://localhost:3000',
		typeSlug: comparison.type_slug ?? ''
	};
};
