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

	let factors: CrsFactorsResponse | null = null;
	try {
		factors = await apiGet<CrsFactorsResponse>(ctx, '/crs/factors');
	} catch {
		factors = null;
	}

	let latestDraws: CrsDraw[] = [];
	try {
		latestDraws = await apiGet<CrsDraw[]>(ctx, '/crs/draws/latest');
	} catch {
		latestDraws = [];
	}

	let drawHistory: CrsDraw[] = [];
	try {
		drawHistory = (await apiList<CrsDraw>(ctx, '/crs/draws?limit=40')).items;
	} catch {
		drawHistory = [];
	}

	let scoreHistory: CrsScoreHistoryItem[] = [];
	if (authed) {
		try {
			scoreHistory = (await apiList<CrsScoreHistoryItem>(ctx, '/me/crs-scores')).items;
		} catch {
			scoreHistory = [];
		}
	}

	return { factors, latestDraws, drawHistory, scoreHistory, authed };
};
