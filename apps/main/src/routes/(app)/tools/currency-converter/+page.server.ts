import { error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';
import type { FxCurrency } from '$lib/types';

/**
 * FX Currency Converter at /tools/currency-converter (§112, ADR-125).
 * SSR-fetches the picker currency list (codes covered by the mid-market
 * matrix); conversion itself happens client-side via GET /fx/convert.
 *
 * API: GET /fx/currencies — active currencies with matrix coverage.
 *      GET /fx/convert?amount=&from=&to= — mid-market conversion.
 *
 * Ref: japaconnect-engineering/sections/112-fx-converter.md
 */
export const load: PageServerLoad = async (event) => {
	const { features, siteSettings } = await event.parent();
	if (!features['fx.enabled']) {
		throw error(404, { code: 'feature_disabled', message: 'Currency Converter is not available' });
	}

	const ctx = apiContext(event);

	// Picker list — degrades to [] on API failure; the page renders an
	// unavailable state rather than erroring the whole route.
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
		typeof siteSettings?.['fx.default_base_currency'] === 'string'
			? (siteSettings['fx.default_base_currency'] as string)
			: 'USD';

	return { currencies, defaultFrom };
};
