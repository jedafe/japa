import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { CrsFactorsResponse, CrsDraw, CrsScoreHistoryItem } from '$lib/types';

/**
 * CRS Calculator — §107, ADR-118.
 * SSR loads the active ruleset factor tables (for the public factors
 * preview), the latest draw per category, and recent draw history for the
 * cut-off trend chart. Score history is auth-only (loaded when the user is
 * signed in). The canonical score is computed client-side via
 * POST /crs/calculate.
 *
 * API: GET /crs/factors — active ruleset + factor tables (public).
 *      GET /crs/draws/latest — latest draw per category (public).
 *      GET /crs/draws?limit= — draw history for the trend chart (public).
 *      POST /crs/calculate — canonical score (OptionalAuth → saves history).
 *      GET /me/crs-scores — saved score history (auth).
 *
 * Ref: §107 (CRS Calculator), ROADMAP-TOOLS 06.01, ADR-042 (flag → 404).
 */
export const load: PageServerLoad = async (event) => {
	const tool = getTool('crs-calculator')!;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			factors: null,
			latestDraws: [],
			drawHistory: [],
			scoreHistory: [],
			user: event.locals.user
		};
	}

	const ctx = apiContext(event);

	// All datasets are independent — one wave, each degrading to its empty
	// fallback so a broken endpoint never blanks the page.
	// factors: active ruleset + factor tables (public factor preview).
	// latestDraws: headline cut-off cards. drawHistory: trend chart.
	// scoreHistory: auth-only; silently empty when signed out.
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

	return {
		enabled: true,
		tool,
		factors,
		latestDraws,
		drawHistory,
		scoreHistory,
		user: event.locals.user
	};
};
