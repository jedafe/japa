import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import type { JCEComparisonDetail, MatrixItem, ResolvedItem } from '$lib/types/jce';
import type { ContentAnalytics } from '$lib/types/content';
import type { PageServerLoad } from './$types';

function resolvedItems(cmp: JCEComparisonDetail): ResolvedItem[] {
	return (
		cmp.items?.map((it) => ({
			type: (it.item_ref?.type as string) ?? '',
			id: (it.item_ref?.id as string) ?? (it.id as string) ?? '',
			title: it.title,
			description: it.description,
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

	// Owner analytics (Section 40 — GET /content/{id}/analytics is owner or
	// analytics.admin only). Fetch only for the author to avoid guaranteed
	// 403s for other viewers; a failure degrades to no stats card.
	let analytics: ContentAnalytics | null = null;
	if (event.locals.user?.id && comparison.content.author_id === event.locals.user.id) {
		try {
			analytics = await apiGet<ContentAnalytics>(ctx, `/content/${comparison.content.id}/analytics`);
		} catch {
			analytics = null;
		}
	}

	return {
		comparison,
		matrix,
		resolvedItems: resolvedItems(comparison),
		analytics
	};
};
