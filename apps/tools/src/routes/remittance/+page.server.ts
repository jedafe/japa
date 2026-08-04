import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type {
	RemittanceCorridor,
	RemittanceProvider,
	RemittanceRate,
	RemittanceAlert
} from '$lib/types';

/**
 * Remittance Rate Tracker — §74, ADR-075.
 * SSR loads corridors, providers, and the default corridor's rates.
 * History (trend chart) and alerts are loaded based on user selection.
 *
 * API: GET /remittance/corridors — supported currency pairs.
 *      GET /remittance/providers — active providers.
 *      GET /remittance/rates?send=&receive= — current rates for a corridor.
 *      GET /remittance/alerts — user's alerts (auth only).
 *
 * Ref: §74 (Remittance Tracker), ROADMAP-TOOLS 02.06.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);

	let corridors: RemittanceCorridor[] = [];
	try {
		corridors = (await apiList<RemittanceCorridor>(ctx, '/remittance/corridors')).items;
	} catch {
		corridors = [];
	}

	let providers: RemittanceProvider[] = [];
	try {
		providers = (await apiList<RemittanceProvider>(ctx, '/remittance/providers')).items;
	} catch {
		providers = [];
	}

	// Default corridor: GBP → NGN (remittance.default_send_currency setting).
	// The first corridor from the API is used as fallback.
	const defaultCorridor = corridors[0] ?? {
		send_currency: 'GBP',
		receive_currency: 'NGN',
		label: 'GBP → NGN'
	};

	let rates: RemittanceRate[] = [];
	try {
		rates = (
			await apiList<RemittanceRate>(
				ctx,
				`/remittance/rates?send=${defaultCorridor.send_currency}&receive=${defaultCorridor.receive_currency}`
			)
		).items;
	} catch {
		rates = [];
	}

	// Load user's alerts if authenticated.
	let alerts: RemittanceAlert[] = [];
	if (event.locals.user) {
		try {
			alerts = (await apiList<RemittanceAlert>(ctx, '/remittance/alerts')).items;
		} catch {
			alerts = [];
		}
	}

	return {
		corridors,
		providers,
		rates,
		alerts,
		defaultCorridor,
		user: event.locals.user
	};
};
