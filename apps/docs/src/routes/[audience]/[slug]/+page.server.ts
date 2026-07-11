import type { PageServerLoad } from './$types';
import { getDocPage } from '$lib/server/api';
import { error } from '@sveltejs/kit';

export const load: PageServerLoad = async ({ params, locals }) => {
	const { audience, slug } = params;
	const token = locals.token as string | undefined;
	try {
		const doc = await getDocPage(audience, slug, token);
		return { doc };
	} catch (e: unknown) {
		const status = (e as { status?: number }).status;
		if (status === 403) {
			throw error(403, { code: 'FORBIDDEN', message: 'Access denied' });
		}
		return { doc: null };
	}
};
