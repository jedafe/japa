import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';

export interface PlanFeature {
	text: string;
	included: boolean;
}

export interface PlanPricing {
	amount: number;
	currency: string;
}

export interface PricingPlan {
	id: string;
	name: string;
	description: string;
	features: PlanFeature[];
	monthly: PlanPricing;
	yearly: PlanPricing;
	highlighted: boolean;
}

export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	try {
		const plans = await apiGet<PricingPlan[]>(ctx, '/pricing/plans');
		return { plans };
	} catch {
		return { plans: [] as PricingPlan[] };
	}
};
