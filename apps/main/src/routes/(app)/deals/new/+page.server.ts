import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/**
 * /deals/new → 308 /products/deals/new — the create form moved with the
 * deals listing under the products surface.
 */
export const load: PageServerLoad = () => {
	redirect(308, '/products/deals/new');
};
