// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
// Server load for the credit purchase page. Fetches active currencies, credit
// packs, and the minimum purchase amount from the backend so the page can
// render pricing without client-side waterfalls (Rule 16 — no hardcoded
// values).
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList, apiGet } from '$lib/server/api';

type Currency = {
	code: string;
	name: string;
	symbol: string;
	credits_per_unit: number;
	position: number;
};

type CreditPack = {
	id: string;
	name: string;
	description: string | null;
	credit_amount: number;
	bonus_credits: number;
	min_user_level: string | null;
	position: number;
};

type MemberSettings = {
	settings: unknown;
	min_convert_points: number;
	min_purchase_credits: number;
};

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);

	let currencies: Currency[] = [];
	let creditPacks: CreditPack[] = [];
	let minPurchaseCredits = 100;

	try {
		const res = await apiList<Currency>(ctx, '/wallet/currencies');
		currencies = res.items;
	} catch {
		// Degrade gracefully — the page will show an error state.
	}

	try {
		const res = await apiList<CreditPack>(ctx, '/wallet/credit-packs');
		creditPacks = res.items;
	} catch {
		// Degrade gracefully.
	}

	// Fetch the minimum purchase credits from member settings (BUG-011 —
	// previously hardcoded as 100 in the frontend).
	try {
		const res = await apiGet<MemberSettings>(ctx, '/me/wallet/settings');
		if (res.min_purchase_credits && res.min_purchase_credits > 0) {
			minPurchaseCredits = res.min_purchase_credits;
		}
	} catch {
		// Degrade gracefully — fall back to default of 100.
	}

	return {
		currencies,
		creditPacks,
		minPurchaseCredits
	};
};
