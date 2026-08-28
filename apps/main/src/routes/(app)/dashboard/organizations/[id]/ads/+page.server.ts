// Ref: japaconnect-engineering/sections/61-self-serve-ads.md (ADR-079)
// Loads the org row + available ad slot types + the org's campaigns for
// the self-serve ads dashboard. Auth-gated; the backend self-403s non-members
// on /org/{org_id}/ads/* routes.
import { error, redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import type { OrganizationAdmin } from '$lib/types';

export const load: PageServerLoad = async (event) => {
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname + event.url.search)}`);
	}
	const ctx = apiContext(event);
	const id = event.params.id;

	let org: OrganizationAdmin | null = null;
	try {
		org = await apiGet<OrganizationAdmin>(ctx, `/organizations/${id}`);
	} catch (err) {
		const status = err instanceof Error && 'status' in err ? (err as { status: number }).status : 500;
		const code = err instanceof Error && 'code' in err ? (err as { code: string }).code : 'api_error';
		throw error(status, { code, message: 'Organization not found' });
	}

	// Load ad slots and campaigns best-effort. The self-serve ads endpoints
	// are /org/{org_id}/ads/* — they require the org_id UUID.
	const PAGE_SIZE = 20;
	const [slotsRes, campaignsRes] = await Promise.all([
		apiList<AdSlotType>(ctx, `/org/${id}/ads/slots`).catch(() => ({ items: [] as AdSlotType[] })),
		apiList<AdCampaign>(ctx, `/org/${id}/ads/campaigns?page=1&limit=${PAGE_SIZE}`).catch(() => ({
			items: [] as AdCampaign[],
			meta: { total: 0, page: 1, limit: PAGE_SIZE }
		}))
	]);

	return {
		org,
		slots: slotsRes.items,
		campaigns: campaignsRes.items,
		total: campaignsRes.meta?.total ?? campaignsRes.items.length,
		page: campaignsRes.meta?.page ?? 1,
		limit: campaignsRes.meta?.limit ?? PAGE_SIZE
	};
};

export interface AdSlotType {
	id: string;
	slot_name: string;
	description?: string;
	flat_rate: number;
	max_display_per_page?: number;
}

export interface AdCampaign {
	id: string;
	name: string;
	status: string;
	content: string;
	target_url?: string;
	budget_credits: number;
	flat_rate: number;
	start_date?: string;
	end_date?: string;
	created_at: string;
}
