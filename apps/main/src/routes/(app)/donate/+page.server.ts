// Donate page load — public platform donation config (§125).
// Ref: japaconnect-engineering/sections/125-donations-tips.md
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

export interface DonateConfig {
	enabled: boolean;
	methods: string[];
	min_tip_amount?: number;
	thank_you_message?: string | null;
	oc_collective_slug?: string | null;
	external_links?: { kind: 'paypal' | 'kofi' | 'buymeacoffee' | 'opencollective' | 'other'; label: string; url: string }[];
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const config = await apiGet<DonateConfig>(ctx, '/donate').catch(() => null);
	return { config: config ?? { enabled: false, methods: [] } };
};
