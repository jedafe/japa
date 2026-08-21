import { apiList } from './api';
import type { ApiContext } from './api';
import type { CatalogOption } from '$lib/types';

/**
 * Load a public option catalog by name (Rule 16).
 * Catalogs are canonical value/label lists returned by GET /catalogs/{name}.
 *
 * Ref: §78 (Tools Web Surface), conventions/api-routes-catalog.md.
 */
export async function loadCatalog(ctx: ApiContext, name: string): Promise<CatalogOption[]> {
	try {
		const { items } = await apiList<CatalogOption>(ctx, `/catalogs/${name}`);
		return items;
	} catch {
		return [];
	}
}
