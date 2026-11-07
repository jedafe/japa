// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Product Domain)
import type { PageServerLoad } from './$types';
import { error, redirect } from '@sveltejs/kit';
import { apiContext, apiGet, apiList } from '$lib/server/api';
import { hasPerm } from '$lib/server/auth';
import type { DealPartnerRef } from '$lib/types';

/**
 * /products/new — product creation gated by the products.create RBAC node.
 * Loads product types (/products/types), providers (/deals/partners) and
 * active deals (/products/deals) for the form's pickers; the type's custom field
 * schema is fetched client-side on selection.
 */
export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.jce']) {
		throw error(404, { code: 'feature_disabled', message: 'Products are not available' });
	}
	if (!event.locals.user) {
		throw redirect(307, `/login?redirect=${encodeURIComponent(event.url.pathname)}`);
	}
	const perms = event.locals.user.permissions ?? [];
	if (!hasPerm(perms, 'products.create')) {
		throw error(403, { code: 'forbidden', message: 'You cannot create products' });
	}
	const ctx = apiContext(event);
	const [types, providers, deals] = await Promise.all([
		apiGet<{ items: { slug: string; label: string }[] }>(ctx, '/products/types').catch(() => null),
		apiList<DealPartnerRef>(ctx, '/deals/partners?page_size=100').catch(() => ({ items: [] as DealPartnerRef[] })),
		apiList<{ id: string; title: string; provider_id: string | null; provider_name?: string | null }>(
			ctx,
			'/products/deals'
		).catch(() => ({ items: [] as { id: string; title: string; provider_id: string | null; provider_name?: string | null }[] }))
	]);
	return {
		types: types?.items ?? [],
		providers: providers.items,
		deals: deals.items
	};
};
