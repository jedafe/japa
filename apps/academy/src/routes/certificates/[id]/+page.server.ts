import type { PageServerLoad } from './$types';
import { apiContext, apiGet, ApiError } from '$lib/server/api';
import { error } from '@sveltejs/kit';
import type { Certificate } from '$lib/types';

/**
 * Public certificate share page — SSR (public content, ADR-060 §4).
 * Anyone with the URL can verify a certificate. No session required.
 */
export const load: PageServerLoad = async (event) => {
	const ctx = apiContext(event);
	const id = event.params.id;

	try {
		const cert = await apiGet<Certificate>(ctx, `/certificates/${id}`);
		return { cert };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) {
			throw error(404, { code: 'certificate_not_found', message: 'Certificate not found' });
		}
		throw err;
	}
};
