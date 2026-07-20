import type { LayoutServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import { normalizeConfig, type CardStyleConfig } from '$lib/feed/cardStyle';

/**
 * Shell-scoped data for every `(app)` route: the per-type card-style config so
 * the Home feed can resolve a card style per content type. The endpoint is
 * public, but the fetch degrades to the reddit default if it is unavailable —
 * a missing setting must never blank the shell.
 */
export const load: LayoutServerLoad = async (event) => {
	const ctx = apiContext(event);

	let cardStyleConfig: CardStyleConfig;
	try {
		cardStyleConfig = normalizeConfig(await apiGet<unknown>(ctx, '/feed/card-style'));
	} catch {
		cardStyleConfig = { default: 'reddit', byType: {} };
	}

	return { cardStyleConfig };
};
