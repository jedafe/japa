import type { PageServerLoad } from './$types';
import { listDocPages } from '$lib/server/api';

export const load: PageServerLoad = async ({ params }) => {
	const { audience } = params;
	try {
		const pages = await listDocPages(audience);
		return { pages };
	} catch {
		return { pages: [] };
	}
};
