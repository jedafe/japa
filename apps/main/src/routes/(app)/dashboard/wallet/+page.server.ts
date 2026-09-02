// Ref: japaconnect-engineering/sections/50-user-dashboard-settings.md (Wallet)
// Dashboard wallet overview: balances, points/credits history, and conversion.
import { redirect, error } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { WalletSummary } from '$lib/types';

export interface WalletLedger {
	balance: number;
	entries: WalletEntry[];
}

export interface WalletEntry {
	id: string;
	ledger_type: 'points' | 'credits';
	amount: number;
	description?: string | null;
	created_at: string;
}

export interface WalletSettings {
	settings: {
		points_to_credits_rate?: number | null;
	};
	min_convert_points: number;
}

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);

	const [summary, pointsLedger, creditsLedger, settings] = await Promise.all([
		apiGet<WalletSummary>(ctx, '/me/wallet').catch(() => null),
		apiGet<WalletLedger>(ctx, '/me/wallet/points?page_size=10').catch(() => ({ balance: 0, entries: [] as WalletEntry[] })),
		apiGet<WalletLedger>(ctx, '/me/wallet/credits?page_size=10').catch(() => ({ balance: 0, entries: [] as WalletEntry[] })),
		apiGet<WalletSettings>(ctx, '/me/wallet/settings').catch(() => null)
	]);

	if (!summary) {
		throw error(500, { code: 'wallet_load_failed', message: 'Could not load wallet' });
	}

	return {
		pointsBalance: summary.points_balance,
		creditsBalance: summary.credits_balance,
		lifetimePoints: summary.lifetime_points,
		pointsEntries: pointsLedger?.entries ?? [],
		creditsEntries: creditsLedger?.entries ?? [],
		settings,
		minConvertPoints: settings?.min_convert_points ?? 100
	};
};
