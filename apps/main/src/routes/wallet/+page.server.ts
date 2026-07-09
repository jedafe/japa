import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

export const load: PageServerLoad = async ({ parent }) => {
	const { token } = await parent();

	const [balanceRes, ledgerRes] = await Promise.allSettled([
		apiFetch<{ points: number; credits: number }>('/me/wallet', {}, token ?? undefined),
		apiFetch<Array<{
			id: string;
			amount: number;
			balance_after: number;
			type: string;
			description: string;
			created_at: string;
		}>>('/me/wallet/ledger?limit=30', {}, token ?? undefined)
	]);

	const balance =
		balanceRes.status === 'fulfilled' && balanceRes.value.ok && balanceRes.value.data
			? balanceRes.value.data
			: { points: 0, credits: 0 };

	const ledger =
		ledgerRes.status === 'fulfilled' && ledgerRes.value.ok && ledgerRes.value.data
			? ledgerRes.value.data
			: [];

	return { balance, ledger };
};
