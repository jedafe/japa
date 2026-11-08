import { error } from '@sveltejs/kit';
import { apiContext, apiGet, ApiError } from '#lib/server/api.js';
import type { PageServerLoad } from './$types';

/**
 * /product/{slug} — the product detail surface. Products are comparison
 * entities (like mentors), not wiki kinds: GET /products/{slug} returns
 * the product core, its provider offers (the within-product provider
 * matrix), and the type's cfv field groups. 404s on unknown slugs.
 */

export interface ProductField {
	key: string;
	label: string;
	field_type: string;
	value?: unknown;
	section?: string;
}

export interface ProductFieldGroup {
	id: string;
	slug: string;
	name: string;
	fields: ProductField[];
}

export interface ProductProvider {
	provider_id: string;
	provider_slug?: string;
	name: string;
	price_from?: number | null;
	offer_url?: string;
	has_deal?: boolean;
}

export interface ProductDetail {
	id: string;
	slug: string;
	name: string;
	tagline?: string;
	website_url?: string;
	logo_url?: string;
	pricing_model?: string;
	price_from?: number | null;
	rating_avg?: number | null;
	review_count: number;
	type_slug: string;
	type_label: string;
	providers: ProductProvider[];
	field_groups?: ProductFieldGroup[];
}

export const load: PageServerLoad = async (event) => {
	const { features } = await event.parent();
	if (!features?.['feature.jce']) {
		throw error(404, { code: 'feature_disabled', message: 'Products are not available' });
	}

	const ctx = apiContext(event);
	try {
		const product = await apiGet<ProductDetail>(ctx, `/products/${encodeURIComponent(event.params.slug)}`);
		if (!product) {
			throw error(404, { code: 'product_not_found', message: 'Product not found' });
		}
		return { product };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'product_not_found', message: 'Product not found' });
		}
		throw err;
	}
};
