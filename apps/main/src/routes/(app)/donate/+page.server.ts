// Donate page load — public platform donation config (§125).
// Ref: japaconnect-engineering/sections/125-donations-tips.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';

export interface DonateConfig {
	enabled: boolean;
	methods: string[];
	min_tip_amount?: number;
	fiat_currency?: string;
	fiat_min_minor?: number;
	fiat_max_minor?: number;
	fiat_presets?: number[];
	thank_you_message?: string | null;
	oc_collective_slug?: string | null;
	external_links?: { kind: 'paypal' | 'kofi' | 'buymeacoffee' | 'opencollective' | 'other' | 'product'; label: string; url: string }[];
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	try {
		const config = await apiGet<DonateConfig>(ctx, '/donate');
		return { config: config ?? { enabled: false, methods: [] }, loadError: false };
	} catch (e) {
		// 404 means the tips.enabled flag is off (feature unavailable copy);
		// anything else is a transport/API failure — surface it as a
		// retryable load error rather than misreporting donations as off.
		const disabled = e instanceof ApiError && e.status === 404;
		return { config: { enabled: false, methods: [] }, loadError: !disabled };
	}
};
