// Ref: japaconnect-engineering/sections/80-destination-wiki.md (Product Domain)
// and docs/superpowers/specs/2026-11-06-wiki-dynamic-entity-kinds.md
import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';
import { hasPerm } from '#lib/server/auth.js';
import type { PageServerLoad } from './$types';

/**
 * /products — the product directory surface. Products are comparison
 * entities (like mentors), not wiki kinds: the listing rides the dedicated
 * product endpoints — GET /products/types supplies the enabled product
 * types (the ?type= filter chips) and GET /products pages the rows, where
 * ?type= filters by product type slug. Cards link to the /product/{slug}
 * detail surface. /deals folds in as the sibling tab at /products/deals.
 */

export interface ProductTypeChip {
	slug: string;
	label: string;
	icon?: string;
	entity_count: number;
}

export interface ProductListItem {
	id: string;
	slug: string;
	name: string;
	tagline?: string;
	discriminator?: string;
	type_label?: string;
	logo_url?: string;
	price_from?: number;
	rating_avg?: number;
	review_count: number;
	provider_count: number;
	href: string;
}

interface EntitiesResponse {
	items: ProductListItem[];
	total: number;
	kind: string;
	label: string;
}

const PAGE_SIZE = 24;

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.jce']) {
		throw error(404, { code: 'feature_disabled', message: 'Products are not available' });
	}

	const ctx = apiContext(event);
	const type = event.url.searchParams.get('type')?.trim() ?? '';
	const q = event.url.searchParams.get('q')?.trim() ?? '';
	const page = Math.max(1, parseInt(event.url.searchParams.get('page') ?? '1', 10) || 1);
	const offset = (page - 1) * PAGE_SIZE;

	const params = new URLSearchParams({ limit: String(PAGE_SIZE), offset: String(offset) });
	if (type) params.set('type', type);
	if (q) params.set('q', q);

	const [typesRes, res] = await Promise.all([
		apiGet<{ items: ProductTypeChip[] }>(ctx, '/products/types').catch(
			() => null as { items: ProductTypeChip[] } | null
		),
		apiGet<EntitiesResponse>(ctx, `/products?${params}`).catch((err) => {
			// An unknown ?type= slug 404s upstream — surface it rather than
			// silently listing every product.
			if (err instanceof ApiError && err.status === 404) throw error(404, { code: 'not_found', message: 'Not found' });
			return null;
		})
	]);

	return {
		items: res?.items ?? [],
		total: res?.total ?? 0,
		label: res?.label ?? 'Products',
		types: typesRes?.items ?? [],
		type,
		q,
		page,
		pageSize: PAGE_SIZE,
		loadError: res === null,
		canCreate: hasPerm(event.locals.user?.permissions, 'products.create'),
		dealsEnabled: features?.['feature.products.deals'] === true
	};
};
