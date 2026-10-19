import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { CrsFactorsResponse, CrsDraw, CrsScoreHistoryItem } from '$lib/types';

/**
 * Express Entry CRS Calculator at /tools/crs-calculator (§107, ADR-118).
 * SSR loads the active ruleset factor tables, latest draws per category,
 * and draw history for the cut-off trend chart. Score history is auth-only.
 * The canonical score is computed client-side via POST /crs/calculate.
 *
 * API: GET /crs/factors — active ruleset + factor tables (public).
 *      GET /crs/draws/latest — latest draw per category (public).
 *      GET /crs/draws?limit= — draw history for the trend chart (public).
 *      POST /crs/calculate — canonical score (OptionalAuth → saves history).
 *      GET /me/crs-scores — saved score history (auth).
 *
 * Ref: japaconnect-engineering/sections/107-crs-calculator.md, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features['crs_calculator.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'CRS Calculator is not available' });
	}

	const ctx = apiContext(event);
	const authed = !!event.locals.user;

	const [factors, latestDraws, drawHistory, scoreHistory] = await Promise.all([
		apiGet<CrsFactorsResponse>(ctx, '/crs/factors').catch(() => null),
		apiGet<CrsDraw[]>(ctx, '/crs/draws/latest').catch(() => [] as CrsDraw[]),
		apiList<CrsDraw>(ctx, '/crs/draws?limit=40')
			.then((r) => r.items)
			.catch(() => [] as CrsDraw[]),
		authed
			? apiList<CrsScoreHistoryItem>(ctx, '/me/crs-scores')
					.then((r) => r.items)
					.catch(() => [] as CrsScoreHistoryItem[])
			: Promise.resolve([] as CrsScoreHistoryItem[])
	]);

	return { factors, latestDraws, drawHistory, scoreHistory, authed };
};
