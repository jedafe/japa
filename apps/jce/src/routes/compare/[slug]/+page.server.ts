import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError, API_V1 } from '$lib/server/api';
import type {
	ComparisonProfile,
	ComparisonRecommendation,
	JCEComparisonDetail,
	MatrixItem,
	ResolvedItem,
	ScoreResult
} from '$lib/types/jce';
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

	let scoreResult: ScoreResult | null = null;
	let recommendations: ComparisonRecommendation[] = [];
	let profiles: ComparisonProfile[] = [];

	if (event.locals.user && comparison.metadata.smart_mode && comparison.metadata.smart_mode !== 'none') {
		const profileID =
			(comparison.metadata.config as Record<string, unknown> | undefined)?.profile_id as string | undefined;

		try {
			scoreResult = await apiGet<ScoreResult>(
				ctx,
				`/jce/comparisons/${slug}/score?profile_id=${encodeURIComponent(profileID ?? '')}`
			);
			recommendations = await apiGet<ComparisonRecommendation[]>(
				ctx,
				`/jce/comparisons/${slug}/recommendations?profile_id=${encodeURIComponent(profileID ?? '')}`
			);
		} catch {
			// Allow the page to render without scores.
		}
	}

	if (event.locals.user) {
		try {
			profiles = await apiGet<ComparisonProfile[]>(ctx, '/jce/profiles');
		} catch {
			profiles = [];
		}
	}

	const defaultProfile = profiles.find((p) => p.is_default) ?? profiles[0] ?? null;

	return {
		comparison,
		matrix,
		resolvedItems: resolvedItem(comparison),
		user: event.locals.user,
		isOwner: !!event.locals.user && event.locals.user.id === comparison.content.author_id,
		apiBase: API_V1,
		mainOrigin: (event.locals.siteSettings?.['jce.main_origin'] as string) ?? 'http://localhost:3000',
		typeSlug: comparison.type_slug ?? '',
		scoreResult,
		recommendations,
		profiles,
		defaultProfile
	};
};
