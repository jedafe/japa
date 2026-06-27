import type { PageServerLoad } from './$types';
import { api } from '$lib/api';

export const load: PageServerLoad = async ({ params }) => {
	const { audience } = params;
	try {
		const pages = await api.listDocPages(audience);
		return { pages };
	} catch {
		return { pages: [] };
	}
};
