import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { loadCatalog } from '$lib/server/catalog';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type {
	RemittanceCorridor,
	RemittanceProvider,
	RemittanceRate,
	RemittanceAlert,
	CatalogOption
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
	// Gate: skip all backend calls when remittance.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('remittance')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return {
			enabled: false,
			tool,
			corridors: [],
			providers: [],
			rates: [],
			alerts: [],
			directionOptions: [],
			defaultCorridor: { send_currency: 'GBP', receive_currency: 'NGN' },
			manualStaleHours: 24,
			user: event.locals.user
		};
	}

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

	// Default corridor: use the remittance.default_send_currency setting
	// (Rule 3 / Rule 16 — never hardcode config values). The first corridor
	// from the API is used as fallback when the setting doesn't match any
	// corridor or is absent.
	// Ref: §74, conventions/system-settings-catalog.md (remittance.default_send_currency).
	const defaultSendCurrency =
		(typeof settings['remittance.default_send_currency'] === 'string'
			? (settings['remittance.default_send_currency'] as string)
			: 'GBP');
	const defaultCorridor =
		corridors.find((c) => c.send_currency === defaultSendCurrency) ?? corridors[0] ?? {
			send_currency: defaultSendCurrency,
			receive_currency: 'NGN'
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

	// Rate alert direction options from the public catalog (Rule 16).
	let directionOptions: CatalogOption[] = [];
	try {
		directionOptions = await loadCatalog(ctx, 'remittance.alert_direction');
	} catch {
		directionOptions = [];
	}

	// Manual-rate staleness threshold from system_settings (Rule 3 — never
	// hardcode config values). Falls back to the spec default of 24h.
	// Ref: conventions/system-settings-catalog.md (remittance.manual_stale_hours).
	const manualStaleHours =
		typeof settings['remittance.manual_stale_hours'] === 'number'
			? (settings['remittance.manual_stale_hours'] as number)
			: 24;

	return {
		enabled: true,
		tool,
		corridors,
		providers,
		rates,
		alerts,
		directionOptions,
		defaultCorridor,
		manualStaleHours,
		user: event.locals.user
	};
};
