// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079)
// Loads a single campaign + its stats for the self-serve ads stats page.
// Auth-gated; the backend self-403s non-members on /org/{org_id}/ads/* routes.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet } from '$lib/server/api';
import type { AdCampaign } from '../../+page.server';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const orgId = event.params.id;
	const campaignId = event.params.campaignId;

	let campaign: AdCampaign | null = null;
	try {
		campaign = await apiGet<AdCampaign>(ctx, `/org/${orgId}/ads/campaigns/${campaignId}`);
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		throw error(status, { code, message: 'Campaign not found' });
	}

	let stats: CampaignStats | null = null;
	try {
		stats = await apiGet<CampaignStats>(ctx, `/org/${orgId}/ads/campaigns/${campaignId}/stats`);
	} catch {
		stats = null;
	}

	return {
		orgId,
		campaign,
		stats
	};
};

export interface CampaignStats {
	impressions: number;
	clicks: number;
	ctr: number;
	spent: number;
	budget_credits: number;
}
