import type { PageServerLoad } from './$types';
import { apiFetch } from '$lib/server/api';

interface PlanFeature {
	text: string;
	included: boolean;
}

interface PricingPlan {
	id: string;
	name: string;
	description: string;
	features: PlanFeature[];
	monthly: { amount: number; currency: string };
	yearly: { amount: number; currency: string };
	highlighted: boolean;
}

export const load: PageServerLoad = async ({ cookies }) => {
	const token = cookies.get('jc_token');
	const headers: Record<string, string> = token ? { Authorization: `Bearer ${token}` } : {};

	try {
		const result = await apiFetch<{ data: PricingPlan[] }>('/pricing/plans', { headers });
		if (result.ok && Array.isArray(result.data?.data)) {
			return { plans: result.data.data };
		}
		return { plans: [] };
	} catch {
		return { plans: [] };
	}
};
