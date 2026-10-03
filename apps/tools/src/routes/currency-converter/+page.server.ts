import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import { getTool, isToolEnabled } from '$lib/tools/registry';
import type { FxCurrency } from '$lib/types';

/**
 * FX Currency Converter — §112, ADR-125.
 * SSR loads the picker currency list (codes covered by the mid-market
 * matrix); conversion itself happens client-side via GET /fx/convert.
 *
 * API: GET /fx/currencies — active currencies with matrix coverage.
 *      GET /fx/convert?amount=&from=&to= — mid-market conversion.
 *
 * Ref: japaconnect-engineering/sections/112-fx-converter.md
 */
export const load: PageServerLoad = async (event) => {
	// Gate: skip all backend calls when fx.enabled is false.
	// Ref: conventions/system-settings-catalog.md, Rule 3.
	const tool = getTool('currency-converter')!;
	const settings = event.locals.siteSettings;
	if (!isToolEnabled(tool, event.locals.features)) {
		return { enabled: false, tool, currencies: [], defaultFrom: 'USD', user: event.locals.user };
	}

	const ctx = apiContext(event);

	// Picker list — degrades to [] on API failure; the page renders an
	// unavailable state rather than erroring the whole route (§4.1).
	let currencies: FxCurrency[] = [];
	try {
		currencies = (await apiList<FxCurrency>(ctx, '/fx/currencies')).items;
	} catch {
		currencies = [];
	}

	// Default "from" currency: the matrix base from system_settings
	// (fx.default_base_currency, Rule 3 — never hardcode config values).
	// The picker normalizes to the first listed code when absent.
	const defaultFrom =
		typeof settings['fx.default_base_currency'] === 'string'
			? (settings['fx.default_base_currency'] as string)
			: 'USD';

	return { enabled: true, tool, currencies, defaultFrom, user: event.locals.user };
};
