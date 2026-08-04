// Ref: japaconnect-engineering/sections/58-credit-currency-system.md
// Server load for the credit purchase page. Fetches active currencies and
// credit packs from the backend so the page can render pricing without
// client-side waterfalls.
import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiList } from '$lib/server/api';

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

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}

	const ctx = apiContext(event);

	let currencies: Currency[] = [];
	let creditPacks: CreditPack[] = [];

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

	return {
		currencies,
		creditPacks
	};
};
