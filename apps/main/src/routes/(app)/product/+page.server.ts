import { redirect } from '@sveltejs/kit';
import type { PageServerLoad } from './$types';

/** /product → 301 /products (canonical products index). */
export const load: PageServerLoad = () => {
	redirect(301, '/products');
};
