import type { PageServerLoad } from './$types';
import { listDocPages } from '$lib/server/api';
import { error } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ params, locals }) => {
	const { audience } = params;
	const token = locals.token as string | undefined;
	try {
		const pages = await listDocPages(audience, token);
		return { pages };
	} catch (e: unknown) {
		const status = (e as { status?: number }).status;
		if (status === 403) {
			throw error(403, { code: 'FORBIDDEN', message: 'Access denied' });
		}
		return { pages: [] };
	}
};
