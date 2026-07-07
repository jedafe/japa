import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface PaymentProvider {
	id: string;
	name: string;
	description: string;
	enabled: boolean;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	try {
		const result = await apiFetch<PaymentProvider[]>('/payment-providers', { headers });
		if (result.ok && result.data) {
			return { providers: result.data };
		}
		return { providers: [] };
	} catch {
		return { providers: [] };
	}
};
