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

	// Active ruleset + factor tables — used for the public factor preview
	// and the "how points work" explainer.
	let factors: CrsFactorsResponse | null = null;
	try {
		factors = await apiGet<CrsFactorsResponse>(ctx, '/crs/factors');
	} catch {
		factors = null;
	}

	// Latest draw per category — headline cut-off cards.
	let latestDraws: CrsDraw[] = [];
	try {
		latestDraws = await apiGet<CrsDraw[]>(ctx, '/crs/draws/latest');
	} catch {
		latestDraws = [];
	}

	// Recent draw history for the cut-off trend chart (general stream).
	let drawHistory: CrsDraw[] = [];
	try {
		drawHistory = (await apiList<CrsDraw>(ctx, '/crs/draws?limit=40')).items;
	} catch {
		drawHistory = [];
	}

	// Saved score history — auth-only; silently empty when signed out.
	let scoreHistory: CrsScoreHistoryItem[] = [];
	if (event.locals.user) {
		try {
			scoreHistory = (
				await apiList<CrsScoreHistoryItem>(ctx, '/me/crs-scores')
			).items;
		} catch {
			scoreHistory = [];
		}
	}

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
